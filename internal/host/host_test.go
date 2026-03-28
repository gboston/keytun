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
	clientSess := simulateClientJoinWithKeyExchange(t, clientConn, "test-owl-resize")

	// Host sends resize via UpdateTermSize (stores dims + sends)
	h.UpdateTermSize(120, 40)

	// Client should receive an encrypted resize message
	msg := readMsg(t, clientConn)
	if msg.Type != protocol.MsgResize {
		t.Errorf("expected resize, got %+v", msg)
	}
	// Dimensions should be encrypted in Data, not cleartext
	if msg.Cols != 0 || msg.Rows != 0 {
		t.Errorf("expected zero cols/rows (encrypted), got cols=%v rows=%v", msg.Cols, msg.Rows)
	}
	if msg.Data == "" {
		t.Fatal("expected encrypted Data field, got empty")
	}
	// Decrypt and verify dimensions
	ciphertext, err := base64.StdEncoding.DecodeString(msg.Data)
	if err != nil {
		t.Fatalf("decode resize data: %v", err)
	}
	plain, err := clientSess.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("decrypt resize: %v", err)
	}
	if len(plain) != 4 {
		t.Fatalf("expected 4 bytes, got %d", len(plain))
	}
	cols := uint16(plain[0])<<8 | uint16(plain[1])
	rows := uint16(plain[2])<<8 | uint16(plain[3])
	if cols != 120 {
		t.Errorf("cols = %v, want 120", cols)
	}
	if rows != 40 {
		t.Errorf("rows = %v, want 40", rows)
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

func TestHostClientJoinedChannel(t *testing.T) {
	server := setupRelay(t)
	relayURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	inj := newPTYInjector(t)
	h, err := New(relayURL, "test-joined-01", inj)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer h.Close()

	// Before client joins, channel should not be closed
	select {
	case <-h.ClientJoined():
		t.Fatal("ClientJoined channel closed before client joined")
	default:
		// expected: not yet closed
	}

	// Simulate client joining with key exchange
	clientConn := dialWS(t, server)
	defer clientConn.Close()
	simulateClientJoinWithKeyExchange(t, clientConn, "test-joined-01")

	// After client joins, channel should be closed
	select {
	case <-h.ClientJoined():
		// expected: closed
	case <-time.After(2 * time.Second):
		t.Fatal("ClientJoined channel not closed after client joined")
	}
}

func TestHostDoneClosesAfterClose(t *testing.T) {
	server := setupRelay(t)
	relayURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	inj := newPTYInjector(t)
	h, err := New(relayURL, "test-owl-done", inj)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Done channel should not be closed yet
	select {
	case <-h.Done():
		t.Fatal("Done channel closed before Close()")
	default:
	}

	h.Close()

	// Done channel should now be closed
	select {
	case <-h.Done():
		// expected
	case <-time.After(2 * time.Second):
		t.Fatal("Done channel not closed after Close()")
	}
}

func TestHostClientReconnectDoesNotPanic(t *testing.T) {
	server := setupRelay(t)
	relayURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	inj := &noOutputInjector{}
	h, err := New(relayURL, "test-reconnect-01", inj)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer h.Close()

	// First client connects and completes key exchange
	clientConn1 := dialWS(t, server)
	clientSess1 := simulateClientJoinWithKeyExchange(t, clientConn1, "test-reconnect-01")

	// Send some input to confirm the session works
	plaintext := []byte("first")
	encrypted, err := clientSess1.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	encoded := base64.StdEncoding.EncodeToString(encrypted)
	sendMsg(t, clientConn1, protocol.Message{
		Type: protocol.MsgInput,
		Data: encoded,
	})

	// First client disconnects
	clientConn1.Close()

	// Give the relay time to notice the disconnect and notify the host
	time.Sleep(200 * time.Millisecond)

	// Second client connects — this would panic before the fix because
	// the key exchange handler would close the already-closed keyReady channel.
	clientConn2 := dialWS(t, server)
	defer clientConn2.Close()
	clientSess2 := simulateClientJoinWithKeyExchange(t, clientConn2, "test-reconnect-01")

	// Verify the new session works by sending input
	plaintext2 := []byte("second")
	encrypted2, err := clientSess2.Encrypt(plaintext2)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	encoded2 := base64.StdEncoding.EncodeToString(encrypted2)
	sendMsg(t, clientConn2, protocol.Message{
		Type: protocol.MsgInput,
		Data: encoded2,
	})

	// Verify the injector received input from both sessions
	time.Sleep(200 * time.Millisecond)
	inj.mu.Lock()
	injected := string(inj.injected)
	inj.mu.Unlock()
	if !strings.Contains(injected, "first") {
		t.Errorf("expected injector to contain 'first', got: %q", injected)
	}
	if !strings.Contains(injected, "second") {
		t.Errorf("expected injector to contain 'second', got: %q", injected)
	}
}

func TestHostSetsTerminalTitleOnStateChanges(t *testing.T) {
	server := setupRelay(t)
	relayURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	var localBuf safeBuffer
	inj := &noOutputInjector{}
	h, err := New(relayURL, "test-title-01", inj, &localBuf)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer h.Close()

	// After creation, localOut should contain the "waiting" terminal title
	time.Sleep(100 * time.Millisecond)
	output := localBuf.String()
	wantWaiting := "\x1b]0;keytun: test-title-01 (waiting)\x07"
	if !strings.Contains(output, wantWaiting) {
		t.Errorf("expected waiting title %q in output, got: %q", wantWaiting, output)
	}

	// Client joins — title should update to "client connected"
	clientConn := dialWS(t, server)
	simulateClientJoinWithKeyExchange(t, clientConn, "test-title-01")

	time.Sleep(200 * time.Millisecond)
	output = localBuf.String()
	wantConnected := "\x1b]0;keytun: test-title-01 (client connected)\x07"
	if !strings.Contains(output, wantConnected) {
		t.Errorf("expected connected title %q in output, got: %q", wantConnected, output)
	}

	// Client disconnects — title should show session is still open
	clientConn.Close()
	time.Sleep(300 * time.Millisecond)
	output = localBuf.String()
	afterConnected := strings.LastIndex(output, wantConnected)
	if afterConnected < 0 {
		t.Fatal("connected title not found")
	}
	remainder := output[afterConnected+len(wantConnected):]
	wantDisconnected := "\x1b]0;keytun: test-title-01 (session open \xe2\x80\x94 waiting for client)\x07"
	if !strings.Contains(remainder, wantDisconnected) {
		t.Errorf("expected 'session open' title after disconnect in output, got remainder: %q", remainder)
	}
}

// safeBuffer is a concurrency-safe bytes buffer for capturing localOut in tests.
type safeBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
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
