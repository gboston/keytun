// ABOUTME: Host-side logic for keytun: connects to the relay and delivers remote keystrokes.
// ABOUTME: Uses an Injector to deliver input — either to a PTY (terminal mode) or the OS (system mode).
package host

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/gboston/keytun/internal/crypto"
	"github.com/gboston/keytun/internal/inject"
	"github.com/gboston/keytun/internal/protocol"
	"github.com/gorilla/websocket"
)

const (
	pingInterval = 30 * time.Second
	pongTimeout  = 40 * time.Second
)

// Host manages a keytun hosting session.
type Host struct {
	sessionCode      string
	conn             *websocket.Conn
	connMu           sync.Mutex
	injector         inject.Injector
	localOut         io.Writer
	localOutMu       sync.Mutex
	outputBuf        strings.Builder
	outputMu         sync.Mutex
	cryptoSessions   map[string]*crypto.Session // clientID -> crypto session
	cryptoMu         sync.RWMutex
	keyReady         chan struct{}
	done             chan struct{}
	closeOnce        sync.Once
	wg               sync.WaitGroup
	clientJoined     chan struct{}
	clientJoinedOnce sync.Once
	termCols         uint16
	termRows         uint16
	termMu           sync.Mutex
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
		sessionCode:    sessionCode,
		conn:           conn,
		injector:       injector,
		localOut:       out,
		cryptoSessions: make(map[string]*crypto.Session),
		keyReady:       make(chan struct{}),
		done:           make(chan struct{}),
		clientJoined:   make(chan struct{}),
	}

	// Configure WebSocket keepalive so dead connections are detected quickly
	conn.SetReadDeadline(time.Now().Add(pongTimeout))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongTimeout))
		return nil
	})
	h.wg.Add(1)
	go h.pingLoop()

	// Set initial terminal title showing the session is waiting for a client
	h.setTerminalTitle("waiting")

	// Only read output if the injector produces it (e.g. PTY mode)
	if injector.HasOutput() {
		if or, ok := injector.(inject.OutputReader); ok {
			h.wg.Add(1)
			go h.readOutput(or)
		}
	}

	// Read messages from relay and deliver via injector
	h.wg.Add(1)
	go h.readRelayMessages()

	return h, nil
}

// SessionCode returns the session code for this host.
func (h *Host) SessionCode() string {
	return h.sessionCode
}

// UpdateTermSize stores the current terminal dimensions and sends them to the client.
func (h *Host) UpdateTermSize(cols, rows uint16) {
	h.termMu.Lock()
	h.termCols = cols
	h.termRows = rows
	h.termMu.Unlock()
	h.SendResize(cols, rows)
}

// SendResize sends the host's terminal dimensions to all ready clients via the relay.
// Dimensions are encrypted so the relay only sees ciphertext.
func (h *Host) SendResize(cols, rows uint16) {
	plain := []byte{byte(cols >> 8), byte(cols), byte(rows >> 8), byte(rows)}
	h.cryptoMu.RLock()
	defer h.cryptoMu.RUnlock()
	for clientID, sess := range h.cryptoSessions {
		if !sess.IsReady() {
			continue
		}
		encrypted, err := sess.Encrypt(plain)
		if err != nil {
			continue
		}
		encoded := base64.StdEncoding.EncodeToString(encrypted)
		msg := protocol.Message{
			Type:     protocol.MsgResize,
			Data:     encoded,
			ClientID: clientID,
		}
		data, _ := json.Marshal(msg)
		h.writeMessage(websocket.TextMessage, data)
	}
}

// sendResizeTo sends terminal dimensions to a specific client.
// Falls back to cleartext if encryption is not yet available.
func (h *Host) sendResizeTo(clientID string, cols, rows uint16) {
	plain := []byte{byte(cols >> 8), byte(cols), byte(rows >> 8), byte(rows)}
	h.cryptoMu.RLock()
	sess, ok := h.cryptoSessions[clientID]
	h.cryptoMu.RUnlock()
	if ok && sess.IsReady() {
		encrypted, err := sess.Encrypt(plain)
		if err == nil {
			encoded := base64.StdEncoding.EncodeToString(encrypted)
			msg := protocol.Message{
				Type:     protocol.MsgResize,
				Data:     encoded,
				ClientID: clientID,
			}
			data, _ := json.Marshal(msg)
			h.writeMessage(websocket.TextMessage, data)
			return
		}
	}
	// Fallback to cleartext (e.g. sending initial dimensions before key exchange)
	msg := protocol.Message{
		Type:     protocol.MsgResize,
		Cols:     cols,
		Rows:     rows,
		ClientID: clientID,
	}
	data, _ := json.Marshal(msg)
	h.writeMessage(websocket.TextMessage, data)
}

// writeMessage serializes writes to the WebSocket connection, which does not
// allow concurrent writers.
func (h *Host) writeMessage(msgType int, data []byte) error {
	h.connMu.Lock()
	defer h.connMu.Unlock()
	return h.conn.WriteMessage(msgType, data)
}

// pingLoop sends periodic WebSocket pings to keep the connection alive.
func (h *Host) pingLoop() {
	defer h.wg.Done()
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-h.done:
			return
		case <-ticker.C:
			h.connMu.Lock()
			err := h.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second))
			h.connMu.Unlock()
			if err != nil {
				return
			}
		}
	}
}

// setTerminalTitle sets the terminal window/tab title via OSC escape sequence.
func (h *Host) setTerminalTitle(status string) {
	if h.localOut == nil {
		return
	}
	title := fmt.Sprintf("\x1b]0;keytun: %s (%s)\x07", h.sessionCode, status)
	h.localOutMu.Lock()
	io.WriteString(h.localOut, title)
	h.localOutMu.Unlock()
}

// ClearTerminalTitle resets the terminal title to show the session code without a status.
func (h *Host) ClearTerminalTitle() {
	if h.localOut == nil {
		return
	}
	title := fmt.Sprintf("\x1b]0;keytun: %s\x07", h.sessionCode)
	h.localOutMu.Lock()
	io.WriteString(h.localOut, title)
	h.localOutMu.Unlock()
}

// Close shuts down the host session and waits for all goroutines to finish.
func (h *Host) Close() {
	h.closeOnce.Do(func() {
		close(h.done)
		h.conn.Close()
		h.injector.Close()
		h.wg.Wait()
	})
}

// Done returns a channel that is closed when the host session ends.
func (h *Host) Done() <-chan struct{} {
	return h.done
}

// ClientJoined returns a channel that is closed when a client connects.
func (h *Host) ClientJoined() <-chan struct{} {
	return h.clientJoined
}

// KeyReady returns a channel that is closed when the first client key exchange completes.
func (h *Host) KeyReady() <-chan struct{} {
	return h.keyReady
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

// broadcastEncryptedOutput encrypts data and sends it to all clients
// that have completed key exchange.
func (h *Host) broadcastEncryptedOutput(data []byte) {
	h.cryptoMu.RLock()
	defer h.cryptoMu.RUnlock()
	for clientID, sess := range h.cryptoSessions {
		if !sess.IsReady() {
			continue
		}
		encrypted, err := sess.Encrypt(data)
		if err != nil {
			continue
		}
		encoded := base64.StdEncoding.EncodeToString(encrypted)
		msg := protocol.Message{
			Type:     protocol.MsgOutput,
			Data:     encoded,
			ClientID: clientID,
		}
		msgData, _ := json.Marshal(msg)
		h.writeMessage(websocket.TextMessage, msgData)
	}
}

// sendEncryptedOutputTo encrypts data and sends it to a specific client.
func (h *Host) sendEncryptedOutputTo(clientID string, data []byte) {
	h.cryptoMu.RLock()
	sess, ok := h.cryptoSessions[clientID]
	h.cryptoMu.RUnlock()
	if !ok || !sess.IsReady() {
		return
	}
	encrypted, err := sess.Encrypt(data)
	if err != nil {
		return
	}
	encoded := base64.StdEncoding.EncodeToString(encrypted)
	msg := protocol.Message{
		Type:     protocol.MsgOutput,
		Data:     encoded,
		ClientID: clientID,
	}
	msgData, _ := json.Marshal(msg)
	h.writeMessage(websocket.TextMessage, msgData)
}

// readOutput reads from an OutputReader and forwards output to the relay and buffer.
func (h *Host) readOutput(or inject.OutputReader) {
	defer h.wg.Done()

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
			h.localOutMu.Lock()
			h.localOut.Write(buf[:n])
			h.localOutMu.Unlock()
		}

		// Buffer for test readback
		h.outputMu.Lock()
		h.outputBuf.Write(buf[:n])
		h.outputMu.Unlock()

		// Encrypt and forward to relay as output message (to all ready clients)
		h.broadcastEncryptedOutput(buf[:n])
	}
}

// clientCountStatus returns a terminal title status string like "1 client" or "3 clients".
func clientCountStatus(n int) string {
	if n == 1 {
		return "1 client"
	}
	return fmt.Sprintf("%d clients", n)
}

// clientCountLabel returns a count with a suffix, e.g. "2 total" or "1 remaining".
func clientCountLabel(n int, suffix string) string {
	return fmt.Sprintf("%d %s", n, suffix)
}

// readRelayMessages reads messages from the relay WebSocket and handles them.
func (h *Host) readRelayMessages() {
	defer h.wg.Done()
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
			// Decode base64, decrypt with the sender's crypto session, and deliver via injector
			decoded, err := base64.StdEncoding.DecodeString(msg.Data)
			if err != nil {
				continue
			}
			h.cryptoMu.RLock()
			sess, ok := h.cryptoSessions[msg.ClientID]
			if !ok {
				h.cryptoMu.RUnlock()
				continue
			}
			plaintext, err := sess.Decrypt(decoded)
			h.cryptoMu.RUnlock()
			if err != nil {
				continue
			}
			h.injector.Inject(plaintext)

			// In system mode there is no output stream, so echo input
			// back to all clients so everyone can see what was typed.
			if !h.injector.HasOutput() {
				h.broadcastEncryptedOutput(plaintext)
			}
		case protocol.MsgKeyExchange:
			// Peer's public key — complete the ECDH key exchange for this client
			peerPub, err := base64.StdEncoding.DecodeString(msg.Data)
			if err != nil {
				continue
			}
			h.cryptoMu.Lock()
			sess, ok := h.cryptoSessions[msg.ClientID]
			if !ok {
				h.cryptoMu.Unlock()
				continue
			}
			err = sess.Complete(peerPub)
			h.cryptoMu.Unlock()
			if err != nil {
				continue
			}
			select {
			case <-h.keyReady:
				// already closed from a previous key exchange
			default:
				close(h.keyReady)
			}

			// In system mode, send a banner to this specific client so they know
			// they will only see echoed keystrokes, not application output.
			if !h.injector.HasOutput() {
				banner := "\x1b[90m[system mode — keystrokes are echoed, no application output]\x1b[0m\r\n"
				h.sendEncryptedOutputTo(msg.ClientID, []byte(banner))
			}
		case protocol.MsgPeerEvent:
			var banner string
			if msg.Event == "joined" {
				h.clientJoinedOnce.Do(func() { close(h.clientJoined) })
				// Start key exchange: create a crypto session for this client
				sess, err := crypto.NewSession()
				if err != nil {
					continue
				}
				h.cryptoMu.Lock()
				h.cryptoSessions[msg.ClientID] = sess
				count := len(h.cryptoSessions)
				h.cryptoMu.Unlock()
				pubEncoded := base64.StdEncoding.EncodeToString(sess.PublicKey())
				kxMsg := protocol.Message{
					Type:     protocol.MsgKeyExchange,
					Data:     pubEncoded,
					ClientID: msg.ClientID,
				}
				kxData, _ := json.Marshal(kxMsg)
				h.writeMessage(websocket.TextMessage, kxData)

				// Send current terminal dimensions to this client
				h.termMu.Lock()
				cols, rows := h.termCols, h.termRows
				h.termMu.Unlock()
				if cols > 0 && rows > 0 {
					h.sendResizeTo(msg.ClientID, cols, rows)
				}

				h.setTerminalTitle(clientCountStatus(count))
				banner = fmt.Sprintf("\r\n[keytun] client connected (%s)\r\n", clientCountLabel(count, "total"))
			} else if msg.Event == "left" {
				h.cryptoMu.Lock()
				delete(h.cryptoSessions, msg.ClientID)
				count := len(h.cryptoSessions)
				h.cryptoMu.Unlock()
				if count == 0 {
					h.setTerminalTitle("no clients — waiting")
					banner = "\r\n[keytun] client disconnected — session still open, waiting for reconnect...\r\n"
				} else {
					h.setTerminalTitle(clientCountStatus(count))
					banner = fmt.Sprintf("\r\n[keytun] client disconnected (%s)\r\n", clientCountLabel(count, "remaining"))
				}
			}
			if banner != "" {
				if h.localOut != nil {
					h.localOutMu.Lock()
					io.WriteString(h.localOut, banner)
					h.localOutMu.Unlock()
				}
				h.outputMu.Lock()
				h.outputBuf.WriteString(banner)
				h.outputMu.Unlock()
			}
		}
	}
}
