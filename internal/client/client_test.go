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

	"github.com/gbostoen/keytun/internal/protocol"
	"github.com/gbostoen/keytun/internal/relay"
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

	c, err := New(relayURL, "test-rat-10")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	// Host should get peer_event "joined"
	msg := readMsg(t, host)
	if msg.Type != protocol.MsgPeerEvent || msg.Event != "joined" {
		t.Errorf("expected peer_event joined, got %+v", msg)
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

	c, err := New(relayURL, "test-rat-11")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	// Read the peer_event on host side
	readMsg(t, host)

	// Send input through the client
	if err := c.SendInput([]byte("hello")); err != nil {
		t.Fatalf("SendInput: %v", err)
	}

	// Host should receive the input message
	msg := readMsg(t, host)
	if msg.Type != protocol.MsgInput {
		t.Fatalf("expected input, got %+v", msg)
	}
	decoded, err := base64.StdEncoding.DecodeString(msg.Data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(decoded) != "hello" {
		t.Errorf("data = %q, want %q", string(decoded), "hello")
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

	c, err := New(relayURL, "test-rat-12")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	// Read the peer_event
	readMsg(t, host)

	// Send Ctrl+C (0x03)
	if err := c.SendInput([]byte{0x03}); err != nil {
		t.Fatalf("SendInput: %v", err)
	}

	msg := readMsg(t, host)
	decoded, _ := base64.StdEncoding.DecodeString(msg.Data)
	if len(decoded) != 1 || decoded[0] != 0x03 {
		t.Errorf("expected Ctrl+C byte (0x03), got %v", decoded)
	}
}
