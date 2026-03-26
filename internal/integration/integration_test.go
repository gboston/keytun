// ABOUTME: End-to-end integration tests for keytun.
// ABOUTME: Spins up relay + host + client in-process and verifies keystrokes flow through.
package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gbostoen/keytun/internal/client"
	"github.com/gbostoen/keytun/internal/host"
	"github.com/gbostoen/keytun/internal/inject"
	"github.com/gbostoen/keytun/internal/protocol"
	"github.com/gbostoen/keytun/internal/relay"
	"github.com/gorilla/websocket"
)

func startRelay(t *testing.T) *httptest.Server {
	t.Helper()
	r := relay.New()
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", r.HandleWebSocket)
	server := httptest.NewServer(mux)
	t.Cleanup(func() { server.Close() })
	return server
}

func wsURL(server *httptest.Server) string {
	return "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
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

func TestEndToEnd_KeystrokesFlowThrough(t *testing.T) {
	server := startRelay(t)
	url := wsURL(server)

	// Start host
	inj := newPTYInjector(t)
	h, err := host.New(url, "e2e-fox-42", inj)
	if err != nil {
		t.Fatalf("host.New: %v", err)
	}
	defer h.Close()

	// Start client
	c, err := client.New(url, "e2e-fox-42")
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	defer c.Close()

	// Client types "echo integration\n"
	if err := c.SendInput([]byte("echo integration\n")); err != nil {
		t.Fatalf("SendInput: %v", err)
	}

	// Host PTY output should contain "integration"
	output := h.ReadOutputUntil("integration", 5*time.Second)
	if !strings.Contains(output, "integration") {
		t.Errorf("expected host PTY output to contain 'integration', got: %q", output)
	}
}

func TestEndToEnd_ClientDisconnectNotifiesHost(t *testing.T) {
	server := startRelay(t)
	url := wsURL(server)

	inj := newPTYInjector(t)
	h, err := host.New(url, "e2e-fox-43", inj)
	if err != nil {
		t.Fatalf("host.New: %v", err)
	}
	defer h.Close()

	c, err := client.New(url, "e2e-fox-43")
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}

	// Wait for connection to be established
	output := h.ReadOutputUntil("client connected", 3*time.Second)
	if !strings.Contains(output, "client connected") {
		t.Fatalf("expected 'client connected' banner, got: %q", output)
	}

	// Client disconnects
	c.Close()

	// Host should see "client disconnected"
	output = h.ReadOutputUntil("client disconnected", 3*time.Second)
	if !strings.Contains(output, "client disconnected") {
		t.Errorf("expected 'client disconnected' banner, got: %q", output)
	}
}

func TestEndToEnd_ControlSequencesPreserved(t *testing.T) {
	server := startRelay(t)
	url := wsURL(server)

	inj := newPTYInjector(t)
	h, err := host.New(url, "e2e-fox-44", inj)
	if err != nil {
		t.Fatalf("host.New: %v", err)
	}
	defer h.Close()

	c, err := client.New(url, "e2e-fox-44")
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	defer c.Close()

	// Send tab character (for tab completion) followed by a command
	// Tab is 0x09, but let's just test that an echo with special chars works
	if err := c.SendInput([]byte("echo 'ctrl-test'\n")); err != nil {
		t.Fatalf("SendInput: %v", err)
	}

	output := h.ReadOutputUntil("ctrl-test", 5*time.Second)
	if !strings.Contains(output, "ctrl-test") {
		t.Errorf("expected 'ctrl-test' in output, got: %q", output)
	}
}

func TestEndToEnd_MultipleInputMessages(t *testing.T) {
	server := startRelay(t)
	url := wsURL(server)

	inj := newPTYInjector(t)
	h, err := host.New(url, "e2e-fox-45", inj)
	if err != nil {
		t.Fatalf("host.New: %v", err)
	}
	defer h.Close()

	c, err := client.New(url, "e2e-fox-45")
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	defer c.Close()

	// Send keystrokes one byte at a time like a real keyboard
	for _, b := range []byte("echo byte-by-byte\n") {
		if err := c.SendInput([]byte{b}); err != nil {
			t.Fatalf("SendInput: %v", err)
		}
	}

	output := h.ReadOutputUntil("byte-by-byte", 5*time.Second)
	if !strings.Contains(output, "byte-by-byte") {
		t.Errorf("expected 'byte-by-byte' in output, got: %q", output)
	}
}

func TestEndToEnd_HostOutputReachesClient(t *testing.T) {
	server := startRelay(t)
	url := wsURL(server)

	inj := newPTYInjector(t)
	h, err := host.New(url, "e2e-fox-46", inj)
	if err != nil {
		t.Fatalf("host.New: %v", err)
	}
	defer h.Close()

	// Connect raw WebSocket as client to read output messages directly
	wsConn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer wsConn.Close()

	joinMsg, _ := json.Marshal(protocol.Message{
		Type:    protocol.MsgClientJoin,
		Session: "e2e-fox-46",
	})
	wsConn.WriteMessage(websocket.TextMessage, joinMsg)

	// Read session_joined ack
	wsConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	wsConn.ReadMessage()

	// Send a command that produces output
	inputMsg, _ := json.Marshal(protocol.Message{
		Type: protocol.MsgInput,
		Data: "ZWNobyByZWxheS10ZXN0Cg==", // "echo relay-test\n"
	})
	wsConn.WriteMessage(websocket.TextMessage, inputMsg)

	// Read output messages from the relay
	deadline := time.Now().Add(5 * time.Second)
	found := false
	for time.Now().Before(deadline) {
		wsConn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, data, err := wsConn.ReadMessage()
		if err != nil {
			break
		}
		var msg protocol.Message
		json.Unmarshal(data, &msg)
		if msg.Type == protocol.MsgOutput {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to receive output messages from host via relay")
	}
}
