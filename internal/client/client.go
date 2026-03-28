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

// Client manages a keytun client session.
type Client struct {
	conn          *websocket.Conn
	connMu        sync.Mutex
	cryptoSession *crypto.Session
	onOutput      func([]byte)
	done          chan struct{}
}

// SetOnOutput registers a callback that is invoked with decrypted terminal
// output from the host. Must be called before the read loop processes messages.
func (c *Client) SetOnOutput(fn func([]byte)) {
	c.onOutput = fn
}

// New creates a new Client that connects to the relay and joins a session.
// Returns an error if the session doesn't exist.
func New(relayURL string, sessionCode string) (*Client, error) {
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
	if err := sess.Complete(peerPub); err != nil {
		conn.Close()
		return nil, fmt.Errorf("key exchange: %w", err)
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
			select {
			case <-c.done:
				// Already closed
			default:
				close(c.done)
			}
			return
		}

		var msg protocol.Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case protocol.MsgOutput:
			if c.onOutput == nil {
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
			c.onOutput(plaintext)
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
	select {
	case <-c.done:
		return
	default:
		close(c.done)
	}
	c.conn.Close()
}
