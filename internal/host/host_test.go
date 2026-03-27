// ABOUTME: Tests for the keytun host functionality.
// ABOUTME: Validates WebSocket connection, input delivery via injector, and output forwarding.
package host

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gboston/keytun/internal/crypto"
	"github.com/gboston/keytun/internal/inject"
	"github.com/gboston/keytun/internal/protocol"
	"github.com/gboston/keytun/internal/relay"
	"github.com/gorilla/websocket"
)

func setupRelay(t *testing.T) *httptest.Server {
	t.Helper()
	r := relay.New()
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", r.HandleWebSocket)
	server := httptest.NewServer(mux)
	t.Cleanup(func() { server.Close() })
	return server
}

func dialWS(t *testing.T, server *httptest.Server) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return conn
}

func readMsg(t *testing.T, conn *websocket.Conn) protocol.Message {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var msg protocol.Message
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return msg
}

func sendMsg(t *testing.T, conn *websocket.Conn, msg protocol.Message) {
	t.Helper()
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func newPTYInjector(t *testing.T) *inject.PTYInjector {
	t.Helper()
	p, err := inject.NewPTY()
	if err != nil {
		t.Fatalf("NewPTY: %v", err)
	}
	t.Cleanup(func() { p.Close() })
	return p
}

// noOutputInjector is a test injector that has no output stream,
// simulating system mode behavior for echo testing.
type noOutputInjector struct {
	mu       sync.Mutex
	injected []byte
}

func (n *noOutputInjector) Inject(data []byte) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.injected = append(n.injected, data...)
	return nil
}

func (n *noOutputInjector) HasOutput() bool { return false }
func (n *noOutputInjector) Close() error    { return nil }

// simulateClientJoinWithKeyExchange joins a session as a raw WS client and
// performs the encryption key exchange with the host. Returns the crypto
// session for encrypting/decrypting messages.
func simulateClientJoinWithKeyExchange(t *testing.T, conn *websocket.Conn, session string) *crypto.Session {
	t.Helper()
	// Send client_join
	sendMsg(t, conn, protocol.Message{
		Type:    protocol.MsgClientJoin,
		Session: session,
	})
	// Read session_joined ack
	readMsg(t, conn)

	// Create crypto session and send our public key
	sess, err := crypto.NewSession()
	if err != nil {
		t.Fatalf("crypto.NewSession: %v", err)
	}
	pubEncoded := base64.StdEncoding.EncodeToString(sess.PublicKey())
	sendMsg(t, conn, protocol.Message{
		Type: protocol.MsgKeyExchange,
		Data: pubEncoded,
	})

	// Read host's key_exchange
	kxMsg := readMsg(t, conn)
	if kxMsg.Type != protocol.MsgKeyExchange {
		t.Fatalf("expected key_exchange, got %+v", kxMsg)
	}
	peerPub, err := base64.StdEncoding.DecodeString(kxMsg.Data)
	if err != nil {
		t.Fatalf("decode peer pub: %v", err)
	}
	if err := sess.Complete(peerPub); err != nil {
		t.Fatalf("complete key exchange: %v", err)
	}
	return sess
}

func TestHostConnectsAndRegisters(t *testing.T) {
	server := setupRelay(t)
	relayURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	inj := newPTYInjector(t)
	h, err := New(relayURL, "test-owl-11", inj)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer h.Close()

	if h.SessionCode() != "test-owl-11" {
		t.Errorf("session code = %v, want test-owl-11", h.SessionCode())
	}
}

func TestHostReceivesRemoteInput(t *testing.T) {
	server := setupRelay(t)
	relayURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	inj := newPTYInjector(t)
	h, err := New(relayURL, "test-owl-12", inj)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer h.Close()

	// Simulate a client joining with key exchange
	clientConn := dialWS(t, server)
	defer clientConn.Close()
	clientSess := simulateClientJoinWithKeyExchange(t, clientConn, "test-owl-12")

	// Send encrypted input from the client (echo command + newline)
	plaintext := []byte("echo hello\n")
	encrypted, err := clientSess.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	encoded := base64.StdEncoding.EncodeToString(encrypted)
	sendMsg(t, clientConn, protocol.Message{
		Type: protocol.MsgInput,
		Data: encoded,
	})

	// Read PTY output from the host — it should eventually contain "hello"
	output := h.ReadOutputUntil("hello", 5*time.Second)
	if !strings.Contains(output, "hello") {
		t.Errorf("expected PTY output to contain 'hello', got: %q", output)
	}
}

func TestHostSendsResizeToClient(t *testing.T) {
	server := setupRelay(t)
	relayURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	inj := newPTYInjector(t)
	h, err := New(relayURL, "test-owl-resize", inj)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer h.Close()

	// Simulate a client joining with key exchange
	clientConn := dialWS(t, server)
	defer clientConn.Close()
	simulateClientJoinWithKeyExchange(t, clientConn, "test-owl-resize")

	// Host sends resize
	h.SendResize(120, 40)

	// Client should receive unencrypted resize message
	msg := readMsg(t, clientConn)
	if msg.Type != protocol.MsgResize {
		t.Errorf("expected resize, got %+v", msg)
	}
	if msg.Cols != 120 {
		t.Errorf("cols = %v, want 120", msg.Cols)
	}
	if msg.Rows != 40 {
		t.Errorf("rows = %v, want 40", msg.Rows)
	}
}

func TestHostSendsOutputToClient(t *testing.T) {
	server := setupRelay(t)
	relayURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	inj := newPTYInjector(t)
	h, err := New(relayURL, "test-owl-13", inj)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer h.Close()

	// Simulate a client joining with key exchange
	clientConn := dialWS(t, server)
	defer clientConn.Close()
	clientSess := simulateClientJoinWithKeyExchange(t, clientConn, "test-owl-13")

	// Send encrypted input that produces output
	plaintext := []byte("echo world\n")
	encrypted, err := clientSess.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	encoded := base64.StdEncoding.EncodeToString(encrypted)
	sendMsg(t, clientConn, protocol.Message{
		Type: protocol.MsgInput,
		Data: encoded,
	})

	// Client should receive encrypted output messages
	deadline := time.Now().Add(5 * time.Second)
	found := false
	for time.Now().Before(deadline) {
		clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, data, err := clientConn.ReadMessage()
		if err != nil {
			break
		}
		var msg protocol.Message
		json.Unmarshal(data, &msg)
		if msg.Type == protocol.MsgOutput && msg.Data != "" {
			// Verify we can decrypt the output
			ciphertext, _ := base64.StdEncoding.DecodeString(msg.Data)
			_, decErr := clientSess.Decrypt(ciphertext)
			if decErr != nil {
				t.Fatalf("failed to decrypt output: %v", decErr)
			}
			found = true
			break
		}
	}
	if !found {
		t.Error("expected client to receive output messages from host PTY")
	}
}

func TestHostEchosInputWhenNoOutput(t *testing.T) {
	server := setupRelay(t)
	relayURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	inj := &noOutputInjector{}
	h, err := New(relayURL, "test-echo-01", inj)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer h.Close()

	// Simulate a client joining with key exchange
	clientConn := dialWS(t, server)
	defer clientConn.Close()
	clientSess := simulateClientJoinWithKeyExchange(t, clientConn, "test-echo-01")

	// Send encrypted input
	plaintext := []byte("hello")
	encrypted, err := clientSess.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	encoded := base64.StdEncoding.EncodeToString(encrypted)
	sendMsg(t, clientConn, protocol.Message{
		Type: protocol.MsgInput,
		Data: encoded,
	})

	// Client should receive the echoed input as output
	deadline := time.Now().Add(5 * time.Second)
	var received []byte
	for time.Now().Before(deadline) {
		clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, data, err := clientConn.ReadMessage()
		if err != nil {
			break
		}
		var msg protocol.Message
		json.Unmarshal(data, &msg)
		if msg.Type == protocol.MsgOutput && msg.Data != "" {
			ciphertext, _ := base64.StdEncoding.DecodeString(msg.Data)
			decrypted, decErr := clientSess.Decrypt(ciphertext)
			if decErr != nil {
				t.Fatalf("failed to decrypt echoed output: %v", decErr)
			}
			received = append(received, decrypted...)
			if strings.Contains(string(received), "hello") {
				break
			}
		}
	}
	if !strings.Contains(string(received), "hello") {
		t.Errorf("expected echoed input containing 'hello', got: %q", string(received))
	}

	// Verify the injector also received the input
	inj.mu.Lock()
	injected := string(inj.injected)
	inj.mu.Unlock()
	if !strings.Contains(injected, "hello") {
		t.Errorf("expected injector to receive 'hello', got: %q", injected)
	}
}

func TestHostSendsBannerWhenNoOutput(t *testing.T) {
	server := setupRelay(t)
	relayURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	inj := &noOutputInjector{}
	h, err := New(relayURL, "test-echo-banner", inj)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer h.Close()

	// Simulate a client joining with key exchange
	clientConn := dialWS(t, server)
	defer clientConn.Close()
	clientSess := simulateClientJoinWithKeyExchange(t, clientConn, "test-echo-banner")

	// Client should receive a banner message indicating system mode
	deadline := time.Now().Add(5 * time.Second)
	found := false
	for time.Now().Before(deadline) {
		clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, data, err := clientConn.ReadMessage()
		if err != nil {
			break
		}
		var msg protocol.Message
		json.Unmarshal(data, &msg)
		if msg.Type == protocol.MsgOutput && msg.Data != "" {
			ciphertext, _ := base64.StdEncoding.DecodeString(msg.Data)
			decrypted, decErr := clientSess.Decrypt(ciphertext)
			if decErr != nil {
				t.Fatalf("failed to decrypt banner: %v", decErr)
			}
			if strings.Contains(string(decrypted), "system mode") {
				found = true
				break
			}
		}
	}
	if !found {
		t.Error("expected client to receive a system mode banner after key exchange")
	}
}
