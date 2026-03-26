// ABOUTME: Host-side logic for keytun: connects to the relay and delivers remote keystrokes.
// ABOUTME: Uses an Injector to deliver input — either to a PTY (terminal mode) or the OS (system mode).
package host

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/gboston/keytun/internal/crypto"
	"github.com/gboston/keytun/internal/inject"
	"github.com/gboston/keytun/internal/protocol"
	"github.com/gorilla/websocket"
)

// Host manages a keytun hosting session.
type Host struct {
	sessionCode   string
	conn          *websocket.Conn
	injector      inject.Injector
	localOut      io.Writer
	outputBuf     strings.Builder
	outputMu      sync.Mutex
	cryptoSession *crypto.Session
	keyReady      chan struct{}
	done          chan struct{}
}

// New creates a new Host that connects to the relay and uses the given injector.
// If localOut is non-nil, output and peer events are written to it.
func New(relayURL string, sessionCode string, injector inject.Injector, localOut ...io.Writer) (*Host, error) {
	conn, _, err := websocket.DefaultDialer.Dial(relayURL, nil)
	if err != nil {
		return nil, err
	}

	// Register with relay
	msg := protocol.Message{
		Type:    protocol.MsgHostRegister,
		Session: sessionCode,
	}
	data, _ := json.Marshal(msg)
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		conn.Close()
		return nil, err
	}

	var out io.Writer
	if len(localOut) > 0 && localOut[0] != nil {
		out = localOut[0]
	}

	h := &Host{
		sessionCode: sessionCode,
		conn:        conn,
		injector:    injector,
		localOut:    out,
		keyReady:    make(chan struct{}),
		done:        make(chan struct{}),
	}

	// Only read output if the injector produces it (e.g. PTY mode)
	if injector.HasOutput() {
		if or, ok := injector.(inject.OutputReader); ok {
			go h.readOutput(or)
		}
	}

	// Read messages from relay and deliver via injector
	go h.readRelayMessages()

	return h, nil
}

// SessionCode returns the session code for this host.
func (h *Host) SessionCode() string {
	return h.sessionCode
}

// Close shuts down the host session.
func (h *Host) Close() {
	select {
	case <-h.done:
		return
	default:
		close(h.done)
	}
	h.conn.Close()
	h.injector.Close()
}

// Done returns a channel that is closed when the host session ends.
func (h *Host) Done() <-chan struct{} {
	return h.done
}

// ReadOutputUntil reads buffered output until it contains the target
// string or the timeout expires.
func (h *Host) ReadOutputUntil(target string, timeout time.Duration) string {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		h.outputMu.Lock()
		out := h.outputBuf.String()
		h.outputMu.Unlock()
		if strings.Contains(out, target) {
			return out
		}
		time.Sleep(50 * time.Millisecond)
	}
	h.outputMu.Lock()
	defer h.outputMu.Unlock()
	return h.outputBuf.String()
}

// readOutput reads from an OutputReader and forwards output to the relay and buffer.
func (h *Host) readOutput(or inject.OutputReader) {
	// Wait for encryption key exchange before sending output to the relay.
	select {
	case <-h.keyReady:
	case <-h.done:
		return
	}

	fd := or.OutputFd()
	buf := make([]byte, 4096)
	for {
		select {
		case <-h.done:
			return
		default:
		}
		n, err := fd.Read(buf)
		if err != nil {
			return
		}
		if n == 0 {
			continue
		}

		// Write to local output (stdout) if configured
		if h.localOut != nil {
			h.localOut.Write(buf[:n])
		}

		// Buffer for test readback
		h.outputMu.Lock()
		h.outputBuf.Write(buf[:n])
		h.outputMu.Unlock()

		// Encrypt and forward to relay as output message
		encrypted, err := h.cryptoSession.Encrypt(buf[:n])
		if err != nil {
			continue
		}
		encoded := base64.StdEncoding.EncodeToString(encrypted)
		msg := protocol.Message{
			Type: protocol.MsgOutput,
			Data: encoded,
		}
		data, _ := json.Marshal(msg)
		h.conn.WriteMessage(websocket.TextMessage, data)
	}
}

// readRelayMessages reads messages from the relay WebSocket and handles them.
func (h *Host) readRelayMessages() {
	for {
		select {
		case <-h.done:
			return
		default:
		}
		_, data, err := h.conn.ReadMessage()
		if err != nil {
			return
		}

		var msg protocol.Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case protocol.MsgInput:
			// Decode base64, decrypt, and deliver via injector
			decoded, err := base64.StdEncoding.DecodeString(msg.Data)
			if err != nil {
				continue
			}
			plaintext, err := h.cryptoSession.Decrypt(decoded)
			if err != nil {
				continue
			}
			h.injector.Inject(plaintext)
		case protocol.MsgKeyExchange:
			// Peer's public key — complete the ECDH key exchange
			peerPub, err := base64.StdEncoding.DecodeString(msg.Data)
			if err != nil {
				continue
			}
			if err := h.cryptoSession.Complete(peerPub); err != nil {
				continue
			}
			close(h.keyReady)
		case protocol.MsgPeerEvent:
			var banner string
			if msg.Event == "joined" {
				banner = "\r\n[keytun] client connected\r\n"
				// Start key exchange: create session and send our public key
				sess, err := crypto.NewSession()
				if err != nil {
					continue
				}
				h.cryptoSession = sess
				pubEncoded := base64.StdEncoding.EncodeToString(sess.PublicKey())
				kxMsg := protocol.Message{
					Type: protocol.MsgKeyExchange,
					Data: pubEncoded,
				}
				kxData, _ := json.Marshal(kxMsg)
				h.conn.WriteMessage(websocket.TextMessage, kxData)
			} else if msg.Event == "left" {
				banner = "\r\n[keytun] client disconnected\r\n"
			}
			if banner != "" {
				if h.localOut != nil {
					io.WriteString(h.localOut, banner)
				}
				h.outputMu.Lock()
				h.outputBuf.WriteString(banner)
				h.outputMu.Unlock()
			}
		}
	}
}
