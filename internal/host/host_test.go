// ABOUTME: Tests for the keytun host functionality.
// ABOUTME: Validates WebSocket connection, input delivery via injector, and output forwarding.
package host

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gbostoen/keytun/internal/inject"
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

	// Simulate a client joining and sending input
	client := dialWS(t, server)
	defer client.Close()
	sendMsg(t, client, protocol.Message{
		Type:    protocol.MsgClientJoin,
		Session: "test-owl-12",
	})

	// Wait a moment for the connection to be established
	time.Sleep(100 * time.Millisecond)

	// Send some input from the client (echo command + newline)
	sendMsg(t, client, protocol.Message{
		Type: protocol.MsgInput,
		Data: "ZWNobyBoZWxsbwo=", // "echo hello\n"
	})

	// Read PTY output from the host — it should eventually contain "hello"
	output := h.ReadOutputUntil("hello", 5*time.Second)
	if !strings.Contains(output, "hello") {
		t.Errorf("expected PTY output to contain 'hello', got: %q", output)
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

	// Simulate a client joining
	client := dialWS(t, server)
	defer client.Close()
	sendMsg(t, client, protocol.Message{
		Type:    protocol.MsgClientJoin,
		Session: "test-owl-13",
	})

	time.Sleep(100 * time.Millisecond)

	// Send input that produces output
	sendMsg(t, client, protocol.Message{
		Type: protocol.MsgInput,
		Data: "ZWNobyB3b3JsZAo=", // "echo world\n"
	})

	// Client should receive output messages
	deadline := time.Now().Add(5 * time.Second)
	found := false
	for time.Now().Before(deadline) {
		client.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, data, err := client.ReadMessage()
		if err != nil {
			break
		}
		var msg protocol.Message
		json.Unmarshal(data, &msg)
		if msg.Type == protocol.MsgOutput && msg.Data != "" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected client to receive output messages from host PTY")
	}
}
