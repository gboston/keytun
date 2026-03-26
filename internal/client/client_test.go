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
	host := dialWS(t, server)
	defer host.Close()
	sendMsg(t, host, protocol.Message{
		Type:    protocol.MsgHostRegister,
		Session: "test-rat-10",
	})

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

	host := dialWS(t, server)
	defer host.Close()
	sendMsg(t, host, protocol.Message{
		Type:    protocol.MsgHostRegister,
		Session: "test-rat-11",
	})

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

func TestClientSendsControlSequences(t *testing.T) {
	server := setupRelay(t)
	relayURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	host := dialWS(t, server)
	defer host.Close()
	sendMsg(t, host, protocol.Message{
		Type:    protocol.MsgHostRegister,
		Session: "test-rat-12",
	})

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
