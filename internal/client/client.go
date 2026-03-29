// ABOUTME: Client-side logic for keytun: connects to a session and sends keystrokes.
// ABOUTME: Captures raw key bytes and forwards them over WebSocket to the relay.
package client

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gboston/keytun/internal/crypto"
	"github.com/gboston/keytun/internal/protocol"
	"github.com/gorilla/websocket"
)

const (
	pingInterval = 30 * time.Second
	pongTimeout  = 40 * time.Second
)

// latencyPingInterval controls how often the client sends app-level pings to the host.
const latencyPingInterval = 5 * time.Second

// Client manages a keytun client session.
type Client struct {
	conn          *websocket.Conn
	connMu        sync.Mutex
	cryptoSession *crypto.Session
	onOutput      func([]byte)
	onOutputMu    sync.RWMutex
	done          chan struct{}
	closeOnce     sync.Once

	// Latency tracking
	pendingPing   time.Time
	latency       time.Duration
	latencyMu     sync.Mutex
}

// SetOnOutput registers a callback that is invoked with decrypted terminal
// output from the host. Must be called before the read loop processes messages.
func (c *Client) SetOnOutput(fn func([]byte)) {
	c.onOutputMu.Lock()
	c.onOutput = fn
	c.onOutputMu.Unlock()
}

// New creates a new Client that connects to the relay and joins a session.
// An optional password can be provided for password-protected sessions.
// Returns an error if the session doesn't exist or the password is wrong.
func New(relayURL string, sessionCode string, password ...string) (*Client, error) {
	conn, _, err := websocket.DefaultDialer.Dial(relayURL, nil)
	if err != nil {
		return nil, err
	}

	// Send client_join
	msg := protocol.Message{
		Type:    protocol.MsgClientJoin,
		Session: sessionCode,
	}
	data, _ := json.Marshal(msg)
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		conn.Close()
		return nil, err
	}

	// Wait for either a session_joined ack or an error from the relay.
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, peek, err := conn.ReadMessage()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to read relay response: %w", err)
	}

	var resp protocol.Message
	if err := json.Unmarshal(peek, &resp); err != nil {
		conn.Close()
		return nil, fmt.Errorf("invalid relay response")
	}
	if resp.Type == protocol.MsgError {
		conn.Close()
		return nil, fmt.Errorf("relay: %s", resp.ErrMessage)
	}
	if resp.Type != protocol.MsgSessionJoined {
		conn.Close()
		return nil, fmt.Errorf("unexpected relay response: %s", resp.Type)
	}

	// Perform key exchange: create session, send our public key, read peer's
	sess, err := crypto.NewSession()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("crypto session: %w", err)
	}
	pubEncoded := base64.StdEncoding.EncodeToString(sess.PublicKey())
	kxMsg := protocol.Message{
		Type: protocol.MsgKeyExchange,
		Data: pubEncoded,
	}
	kxData, _ := json.Marshal(kxMsg)
	if err := conn.WriteMessage(websocket.TextMessage, kxData); err != nil {
		conn.Close()
		return nil, fmt.Errorf("send key exchange: %w", err)
	}

	// Read the host's key_exchange message
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, kxPeek, err := conn.ReadMessage()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("read key exchange: %w", err)
	}
	var kxResp protocol.Message
	if err := json.Unmarshal(kxPeek, &kxResp); err != nil {
		conn.Close()
		return nil, fmt.Errorf("invalid key exchange response")
	}
	if kxResp.Type != protocol.MsgKeyExchange {
		conn.Close()
		return nil, fmt.Errorf("expected key_exchange, got %s", kxResp.Type)
	}
	peerPub, err := base64.StdEncoding.DecodeString(kxResp.Data)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("decode peer public key: %w", err)
	}
	var pw string
	if len(password) > 0 {
		pw = password[0]
	}
	if err := sess.Complete(peerPub, pw); err != nil {
		conn.Close()
		return nil, fmt.Errorf("key exchange: %w", err)
	}

	// Send our verify token so the host can confirm key agreement (password match)
	verifyToken, err := sess.VerifyToken()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("verify token: %w", err)
	}
	verifyMsg := protocol.Message{
		Type: protocol.MsgVerify,
		Data: base64.StdEncoding.EncodeToString(verifyToken),
	}
	verifyData, _ := json.Marshal(verifyMsg)
	if err := conn.WriteMessage(websocket.TextMessage, verifyData); err != nil {
		conn.Close()
		return nil, fmt.Errorf("send verify: %w", err)
	}

	// Read the host's verify token and confirm key agreement
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, verifyPeek, err := conn.ReadMessage()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("read verify: %w", err)
	}
	var verifyResp protocol.Message
	if err := json.Unmarshal(verifyPeek, &verifyResp); err != nil {
		conn.Close()
		return nil, fmt.Errorf("invalid verify response")
	}
	if verifyResp.Type != protocol.MsgVerify {
		conn.Close()
		return nil, fmt.Errorf("expected verify, got %s", verifyResp.Type)
	}
	hostToken, err := base64.StdEncoding.DecodeString(verifyResp.Data)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("decode verify token: %w", err)
	}
	if err := sess.CheckVerify(hostToken); err != nil {
		conn.Close()
		return nil, fmt.Errorf("wrong session password")
	}

	conn.SetReadDeadline(time.Now().Add(pongTimeout))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongTimeout))
		return nil
	})

	c := &Client{
		conn:          conn,
		cryptoSession: sess,
		done:          make(chan struct{}),
	}

	// Read incoming messages in the background to detect connection loss.
	go c.readLoop()
	go c.pingLoop()
	go c.latencyPingLoop()

	return c, nil
}

// Done returns a channel that is closed when the connection is lost.
func (c *Client) Done() <-chan struct{} {
	return c.done
}

// readLoop reads incoming messages, delivers output to the callback, and
// detects connection loss.
func (c *Client) readLoop() {
	for {
		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			c.Close()
			return
		}

		var msg protocol.Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case protocol.MsgOutput:
			c.onOutputMu.RLock()
			fn := c.onOutput
			c.onOutputMu.RUnlock()
			if fn == nil {
				continue
			}
			decoded, err := base64.StdEncoding.DecodeString(msg.Data)
			if err != nil {
				continue
			}
			plaintext, err := c.cryptoSession.Decrypt(decoded)
			if err != nil {
				continue
			}
			fn(plaintext)
		case protocol.MsgPing:
			// Host sent a ping — respond with pong echoing the same data
			pong := protocol.Message{
				Type: protocol.MsgPong,
				Data: msg.Data,
			}
			data, _ := json.Marshal(pong)
			c.connMu.Lock()
			c.conn.WriteMessage(websocket.TextMessage, data)
			c.connMu.Unlock()
		case protocol.MsgPong:
			// Host responded to our ping — compute RTT
			c.latencyMu.Lock()
			if !c.pendingPing.IsZero() {
				c.latency = time.Since(c.pendingPing)
				c.pendingPing = time.Time{}
			}
			c.latencyMu.Unlock()
		}
	}
}

// pingLoop sends periodic WebSocket pings to keep the connection alive.
func (c *Client) pingLoop() {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			c.connMu.Lock()
			err := c.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second))
			c.connMu.Unlock()
			if err != nil {
				return
			}
		}
	}
}

// latencyPingLoop sends periodic app-level pings to the host to measure RTT.
func (c *Client) latencyPingLoop() {
	ticker := time.NewTicker(latencyPingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			now := time.Now()
			token := fmt.Sprintf("%d", now.UnixMilli())
			msg := protocol.Message{
				Type: protocol.MsgPing,
				Data: token,
			}
			data, _ := json.Marshal(msg)
			c.latencyMu.Lock()
			c.pendingPing = now
			c.latencyMu.Unlock()
			c.connMu.Lock()
			c.conn.WriteMessage(websocket.TextMessage, data)
			c.connMu.Unlock()
		}
	}
}

// Latency returns the last measured round-trip time to the host.
// Returns 0 if no measurement is available yet.
func (c *Client) Latency() time.Duration {
	c.latencyMu.Lock()
	defer c.latencyMu.Unlock()
	return c.latency
}

// SendInput encrypts and sends raw bytes as an input message to the relay.
func (c *Client) SendInput(input []byte) error {
	encrypted, err := c.cryptoSession.Encrypt(input)
	if err != nil {
		return err
	}
	encoded := base64.StdEncoding.EncodeToString(encrypted)
	msg := protocol.Message{
		Type: protocol.MsgInput,
		Data: encoded,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	c.connMu.Lock()
	defer c.connMu.Unlock()
	return c.conn.WriteMessage(websocket.TextMessage, data)
}

// Close shuts down the client connection.
func (c *Client) Close() {
	c.closeOnce.Do(func() {
		close(c.done)
		c.conn.Close()
	})
}
