// ABOUTME: Tests for the WebSocket relay server.
// ABOUTME: Validates session registration, client joining, message bridging, and cleanup.
package relay

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gboston/keytun/internal/protocol"
	"github.com/gorilla/websocket"
)

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
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
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

func newTestServer(t *testing.T) (*httptest.Server, *Relay) {
	t.Helper()
	r := New()
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", r.HandleWebSocket)
	server := httptest.NewServer(mux)
	t.Cleanup(func() { server.Close() })
	return server, r
}

func TestHostRegisterAndClientJoin(t *testing.T) {
	server, _ := newTestServer(t)

	// Host registers
	host := dialWS(t, server)
	defer host.Close()
	sendMsg(t, host, protocol.Message{
		Type:    protocol.MsgHostRegister,
		Session: "test-fox-42",
	})

	// Client joins
	client := dialWS(t, server)
	defer client.Close()
	sendMsg(t, client, protocol.Message{
		Type:    protocol.MsgClientJoin,
		Session: "test-fox-42",
	})

	// Host should get peer_event "joined"
	msg := readMsg(t, host)
	if msg.Type != protocol.MsgPeerEvent || msg.Event != "joined" {
		t.Errorf("expected peer_event joined, got %+v", msg)
	}
}

func TestInputFlowsFromClientToHost(t *testing.T) {
	server, _ := newTestServer(t)

	host := dialWS(t, server)
	defer host.Close()
	sendMsg(t, host, protocol.Message{
		Type:    protocol.MsgHostRegister,
		Session: "test-fox-43",
	})

	client := dialWS(t, server)
	defer client.Close()
	sendMsg(t, client, protocol.Message{
		Type:    protocol.MsgClientJoin,
		Session: "test-fox-43",
	})

	// Read the peer_event on host side
	readMsg(t, host)

	// Client sends input
	sendMsg(t, client, protocol.Message{
		Type: protocol.MsgInput,
		Data: "aGVsbG8=",
	})

	// Host should receive input
	msg := readMsg(t, host)
	if msg.Type != protocol.MsgInput {
		t.Errorf("expected input, got %+v", msg)
	}
	if msg.Data != "aGVsbG8=" {
		t.Errorf("data = %v, want aGVsbG8=", msg.Data)
	}
}

func TestOutputFlowsFromHostToClient(t *testing.T) {
	server, _ := newTestServer(t)

	host := dialWS(t, server)
	defer host.Close()
	sendMsg(t, host, protocol.Message{
		Type:    protocol.MsgHostRegister,
		Session: "test-fox-44",
	})

	client := dialWS(t, server)
	defer client.Close()
	sendMsg(t, client, protocol.Message{
		Type:    protocol.MsgClientJoin,
		Session: "test-fox-44",
	})

	// Read peer_event on host side
	readMsg(t, host)

	// Read session_joined ack on client side
	ack := readMsg(t, client)
	if ack.Type != protocol.MsgSessionJoined {
		t.Errorf("expected session_joined ack, got %+v", ack)
	}

	// Host sends output
	sendMsg(t, host, protocol.Message{
		Type: protocol.MsgOutput,
		Data: "d29ybGQ=",
	})

	// Client should receive output
	msg := readMsg(t, client)
	if msg.Type != protocol.MsgOutput {
		t.Errorf("expected output, got %+v", msg)
	}
	if msg.Data != "d29ybGQ=" {
		t.Errorf("data = %v, want d29ybGQ=", msg.Data)
	}
}

func TestJoinUnknownSession(t *testing.T) {
	server, _ := newTestServer(t)

	client := dialWS(t, server)
	defer client.Close()
	sendMsg(t, client, protocol.Message{
		Type:    protocol.MsgClientJoin,
		Session: "no-such-99",
	})

	msg := readMsg(t, client)
	if msg.Type != protocol.MsgError {
		t.Errorf("expected error, got %+v", msg)
	}
	if msg.ErrMessage != "session not found" {
		t.Errorf("message = %v, want 'session not found'", msg.ErrMessage)
	}
}

func waitFor(t *testing.T, condition func() bool, timeout time.Duration, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal(msg)
}

func TestHostDisconnectCleansUpSession(t *testing.T) {
	server, r := newTestServer(t)

	host := dialWS(t, server)
	sendMsg(t, host, protocol.Message{
		Type:    protocol.MsgHostRegister,
		Session: "test-fox-45",
	})

	// Wait for session to be registered server-side
	waitFor(t, func() bool { return r.HasSession("test-fox-45") }, 2*time.Second,
		"session should exist after host registers")

	// Host disconnects
	host.Close()

	// Wait for cleanup
	waitFor(t, func() bool { return !r.HasSession("test-fox-45") }, 2*time.Second,
		"session should be cleaned up after host disconnects")
}

func TestClientDisconnectSendsPeerEvent(t *testing.T) {
	server, _ := newTestServer(t)

	host := dialWS(t, server)
	defer host.Close()
	sendMsg(t, host, protocol.Message{
		Type:    protocol.MsgHostRegister,
		Session: "test-fox-46",
	})

	client := dialWS(t, server)
	sendMsg(t, client, protocol.Message{
		Type:    protocol.MsgClientJoin,
		Session: "test-fox-46",
	})

	// Read peer_event "joined"
	readMsg(t, host)

	// Client disconnects
	client.Close()

	// Host should get peer_event "left"
	msg := readMsg(t, host)
	if msg.Type != protocol.MsgPeerEvent || msg.Event != "left" {
		t.Errorf("expected peer_event left, got %+v", msg)
	}
}

func TestKeyExchangeFlowsBidirectionally(t *testing.T) {
	server, _ := newTestServer(t)

	host := dialWS(t, server)
	defer host.Close()
	sendMsg(t, host, protocol.Message{
		Type:    protocol.MsgHostRegister,
		Session: "test-fox-kx",
	})

	client := dialWS(t, server)
	defer client.Close()
	sendMsg(t, client, protocol.Message{
		Type:    protocol.MsgClientJoin,
		Session: "test-fox-kx",
	})

	// Read peer_event on host, session_joined on client
	readMsg(t, host)
	readMsg(t, client)

	// Host sends key_exchange → client should receive it
	sendMsg(t, host, protocol.Message{
		Type: protocol.MsgKeyExchange,
		Data: "aG9zdC1wdWJrZXk=",
	})
	msg := readMsg(t, client)
	if msg.Type != protocol.MsgKeyExchange {
		t.Errorf("client expected key_exchange, got %+v", msg)
	}
	if msg.Data != "aG9zdC1wdWJrZXk=" {
		t.Errorf("client data = %v, want aG9zdC1wdWJrZXk=", msg.Data)
	}

	// Client sends key_exchange → host should receive it
	sendMsg(t, client, protocol.Message{
		Type: protocol.MsgKeyExchange,
		Data: "Y2xpZW50LXB1YmtleQ==",
	})
	msg = readMsg(t, host)
	if msg.Type != protocol.MsgKeyExchange {
		t.Errorf("host expected key_exchange, got %+v", msg)
	}
	if msg.Data != "Y2xpZW50LXB1YmtleQ==" {
		t.Errorf("host data = %v, want Y2xpZW50LXB1YmtleQ==", msg.Data)
	}
}

func TestDuplicateSessionCode(t *testing.T) {
	server, _ := newTestServer(t)

	host1 := dialWS(t, server)
	defer host1.Close()
	sendMsg(t, host1, protocol.Message{
		Type:    protocol.MsgHostRegister,
		Session: "test-fox-47",
	})

	// Second host tries same code
	host2 := dialWS(t, server)
	defer host2.Close()
	sendMsg(t, host2, protocol.Message{
		Type:    protocol.MsgHostRegister,
		Session: "test-fox-47",
	})

	msg := readMsg(t, host2)
	if msg.Type != protocol.MsgError {
		t.Errorf("expected error for duplicate session, got %+v", msg)
	}
}
