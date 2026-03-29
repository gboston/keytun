// ABOUTME: Tests for the keytun client functionality.
// ABOUTME: Validates WebSocket connection, key sending, and disconnect behavior.
package client

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gboston/keytun/internal/crypto"
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

// registerHost dials the relay, sends host_register, and reads the host_registered ack.
func registerHost(t *testing.T, server *httptest.Server, session string) *websocket.Conn {
	t.Helper()
	conn := dialWS(t, server)
	sendMsg(t, conn, protocol.Message{
		Type:    protocol.MsgHostRegister,
		Session: session,
	})
	ack := readMsg(t, conn)
	if ack.Type != protocol.MsgHostRegistered {
		t.Fatalf("expected host_registered, got %+v", ack)
	}
	return conn
}

// hostKeyExchangeResult holds the result of a background host key exchange.
type hostKeyExchangeResult struct {
	session *crypto.Session
	err     error
}

// startHostKeyExchange runs the host-side key exchange in a goroutine. This is
// needed because client.New() blocks until key exchange completes, so the raw
// WS host must handle it concurrently. Call this BEFORE client.New().
func startHostKeyExchange(t *testing.T, conn *websocket.Conn) <-chan hostKeyExchangeResult {
	t.Helper()
	ch := make(chan hostKeyExchangeResult, 1)
	go func() {
		// Read peer_event{joined} from relay
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, data, err := conn.ReadMessage()
		if err != nil {
			ch <- hostKeyExchangeResult{err: err}
			return
		}
		var msg protocol.Message
		json.Unmarshal(data, &msg)

		// Read key_exchange from client (forwarded by relay)
		_, data, err = conn.ReadMessage()
		if err != nil {
			ch <- hostKeyExchangeResult{err: err}
			return
		}
		var kxMsg protocol.Message
		json.Unmarshal(data, &kxMsg)

		// Create crypto session, complete with client's key, send ours
		sess, _ := crypto.NewSession()
		peerPub, _ := base64.StdEncoding.DecodeString(kxMsg.Data)
		if err := sess.Complete(peerPub); err != nil {
			ch <- hostKeyExchangeResult{err: err}
			return
		}
		pubEncoded := base64.StdEncoding.EncodeToString(sess.PublicKey())
		respData, _ := json.Marshal(protocol.Message{
			Type: protocol.MsgKeyExchange,
			Data: pubEncoded,
		})
		conn.WriteMessage(websocket.TextMessage, respData)
		conn.SetReadDeadline(time.Time{})

		ch <- hostKeyExchangeResult{session: sess}
	}()
	return ch
}

func TestClientConnectsToSession(t *testing.T) {
	server := setupRelay(t)
	relayURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	// Register a host first
	host := registerHost(t, server, "test-rat-10")
	defer host.Close()

	// Start host-side key exchange in background before client.New() blocks
	kxCh := startHostKeyExchange(t, host)

	c, err := New(relayURL, "test-rat-10")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	result := <-kxCh
	if result.err != nil {
		t.Fatalf("host key exchange: %v", result.err)
	}
}

func TestClientSendsInput(t *testing.T) {
	server := setupRelay(t)
	relayURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	host := registerHost(t, server, "test-rat-11")
	defer host.Close()

	kxCh := startHostKeyExchange(t, host)

	c, err := New(relayURL, "test-rat-11")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	result := <-kxCh
	if result.err != nil {
		t.Fatalf("host key exchange: %v", result.err)
	}

	// Send input through the client
	if err := c.SendInput([]byte("hello")); err != nil {
		t.Fatalf("SendInput: %v", err)
	}

	// Host should receive the encrypted input message
	msg := readMsg(t, host)
	if msg.Type != protocol.MsgInput {
		t.Fatalf("expected input, got %+v", msg)
	}
	// Data is encrypted — decrypt it with the host's crypto session
	ciphertext, err := base64.StdEncoding.DecodeString(msg.Data)
	if err != nil {
		t.Fatalf("decode base64: %v", err)
	}
	plaintext, err := result.session.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(plaintext) != "hello" {
		t.Errorf("data = %q, want %q", string(plaintext), "hello")
	}
}

func TestClientJoinNonexistentSession(t *testing.T) {
	server := setupRelay(t)
	relayURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	_, err := New(relayURL, "no-such-99")
	if err == nil {
		t.Fatal("expected error joining nonexistent session")
	}
}

func TestClientDoneClosesAfterClose(t *testing.T) {
	server := setupRelay(t)
	relayURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	host := registerHost(t, server, "test-rat-done")
	defer host.Close()

	kxCh := startHostKeyExchange(t, host)

	c, err := New(relayURL, "test-rat-done")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	result := <-kxCh
	if result.err != nil {
		t.Fatalf("host key exchange: %v", result.err)
	}

	// Done channel should not be closed yet
	select {
	case <-c.Done():
		t.Fatal("Done channel closed before Close()")
	default:
	}

	c.Close()

	// Done channel should now be closed
	select {
	case <-c.Done():
		// expected
	case <-time.After(2 * time.Second):
		t.Fatal("Done channel not closed after Close()")
	}
}

func TestClientNewUnexpectedResponseType(t *testing.T) {
	// Server sends an unexpected message type (not session_joined, not error)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		// Read the client_join message
		conn.ReadMessage()
		// Reply with an unexpected message type
		resp, _ := json.Marshal(protocol.Message{Type: "unexpected"})
		conn.WriteMessage(websocket.TextMessage, resp)
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	relayURL := "ws" + strings.TrimPrefix(server.URL, "http")

	_, err := New(relayURL, "test-code")
	if err == nil {
		t.Fatal("expected error for unexpected response type")
	}
	if !strings.Contains(err.Error(), "unexpected relay response") {
		t.Errorf("error = %q, want it to contain 'unexpected relay response'", err)
	}
}

func TestClientNewInvalidJSONResponse(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		conn.ReadMessage()
		// Reply with invalid JSON
		conn.WriteMessage(websocket.TextMessage, []byte("not json"))
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	relayURL := "ws" + strings.TrimPrefix(server.URL, "http")

	_, err := New(relayURL, "test-code")
	if err == nil {
		t.Fatal("expected error for invalid JSON response")
	}
	if !strings.Contains(err.Error(), "invalid relay response") {
		t.Errorf("error = %q, want it to contain 'invalid relay response'", err)
	}
}

func TestClientNewRelayError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		conn.ReadMessage()
		// Reply with an error message
		resp, _ := json.Marshal(protocol.Message{
			Type:       protocol.MsgError,
			ErrMessage: "session not found",
		})
		conn.WriteMessage(websocket.TextMessage, resp)
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	relayURL := "ws" + strings.TrimPrefix(server.URL, "http")

	_, err := New(relayURL, "test-code")
	if err == nil {
		t.Fatal("expected error from relay error response")
	}
	if !strings.Contains(err.Error(), "session not found") {
		t.Errorf("error = %q, want it to contain 'session not found'", err)
	}
}

func TestClientNewBadKeyExchangeResponse(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		conn.ReadMessage()
		// Send session_joined ack
		ack, _ := json.Marshal(protocol.Message{Type: protocol.MsgSessionJoined, Session: "test-code"})
		conn.WriteMessage(websocket.TextMessage, ack)
		// Read client's key_exchange
		conn.ReadMessage()
		// Send wrong message type instead of key_exchange
		resp, _ := json.Marshal(protocol.Message{Type: "output", Data: "junk"})
		conn.WriteMessage(websocket.TextMessage, resp)
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	relayURL := "ws" + strings.TrimPrefix(server.URL, "http")

	_, err := New(relayURL, "test-code")
	if err == nil {
		t.Fatal("expected error for bad key exchange response type")
	}
	if !strings.Contains(err.Error(), "expected key_exchange") {
		t.Errorf("error = %q, want it to contain 'expected key_exchange'", err)
	}
}

func TestClientNewInvalidPeerPublicKey(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		conn.ReadMessage()
		// Send session_joined ack
		ack, _ := json.Marshal(protocol.Message{Type: protocol.MsgSessionJoined, Session: "test-code"})
		conn.WriteMessage(websocket.TextMessage, ack)
		// Read client's key_exchange
		conn.ReadMessage()
		// Send key_exchange with invalid public key (wrong length)
		badKey := base64.StdEncoding.EncodeToString([]byte("too short"))
		resp, _ := json.Marshal(protocol.Message{Type: protocol.MsgKeyExchange, Data: badKey})
		conn.WriteMessage(websocket.TextMessage, resp)
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	relayURL := "ws" + strings.TrimPrefix(server.URL, "http")

	_, err := New(relayURL, "test-code")
	if err == nil {
		t.Fatal("expected error for invalid peer public key")
	}
	if !strings.Contains(err.Error(), "key exchange") {
		t.Errorf("error = %q, want it to contain 'key exchange'", err)
	}
}

func TestClientSetOnOutputReceivesOutput(t *testing.T) {
	server := setupRelay(t)
	relayURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	host := registerHost(t, server, "test-rat-output")
	defer host.Close()

	kxCh := startHostKeyExchange(t, host)

	c, err := New(relayURL, "test-rat-output")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	result := <-kxCh
	if result.err != nil {
		t.Fatalf("host key exchange: %v", result.err)
	}

	// Register output callback
	outputCh := make(chan []byte, 10)
	c.SetOnOutput(func(data []byte) {
		outputCh <- append([]byte(nil), data...)
	})

	// Host sends encrypted output to the client
	plaintext := []byte("hello from host")
	encrypted, err := result.session.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	encoded := base64.StdEncoding.EncodeToString(encrypted)
	data, _ := json.Marshal(protocol.Message{
		Type: protocol.MsgOutput,
		Data: encoded,
	})
	if err := host.WriteMessage(websocket.TextMessage, data); err != nil {
		t.Fatalf("host.WriteMessage: %v", err)
	}

	// Client's readLoop should invoke the callback
	select {
	case got := <-outputCh:
		if string(got) != "hello from host" {
			t.Errorf("output = %q, want %q", string(got), "hello from host")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for output callback")
	}
}

func TestClientReadLoopCloseDoneOnDisconnect(t *testing.T) {
	server := setupRelay(t)
	relayURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	host := registerHost(t, server, "test-rat-readloop-dc")

	kxCh := startHostKeyExchange(t, host)

	c, err := New(relayURL, "test-rat-readloop-dc")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	result := <-kxCh
	if result.err != nil {
		t.Fatalf("host key exchange: %v", result.err)
	}

	// Host disconnects — readLoop should detect and close done channel
	host.Close()

	select {
	case <-c.Done():
		// expected
	case <-time.After(5 * time.Second):
		t.Fatal("Done channel not closed after host disconnect")
	}
}

func TestClientSendsControlSequences(t *testing.T) {
	server := setupRelay(t)
	relayURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	host := registerHost(t, server, "test-rat-12")
	defer host.Close()

	kxCh := startHostKeyExchange(t, host)

	c, err := New(relayURL, "test-rat-12")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	result := <-kxCh
	if result.err != nil {
		t.Fatalf("host key exchange: %v", result.err)
	}

	// Send Ctrl+C (0x03)
	if err := c.SendInput([]byte{0x03}); err != nil {
		t.Fatalf("SendInput: %v", err)
	}

	msg := readMsg(t, host)
	ciphertext, _ := base64.StdEncoding.DecodeString(msg.Data)
	plaintext, err := result.session.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if len(plaintext) != 1 || plaintext[0] != 0x03 {
		t.Errorf("expected Ctrl+C byte (0x03), got %v", plaintext)
	}
}
