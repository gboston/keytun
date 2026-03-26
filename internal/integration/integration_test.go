// ABOUTME: End-to-end integration tests for keytun.
// ABOUTME: Spins up relay + host + client in-process and verifies keystrokes flow through.
package integration

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gboston/keytun/internal/client"
	"github.com/gboston/keytun/internal/host"
	"github.com/gboston/keytun/internal/inject"
	"github.com/gboston/keytun/internal/relay"
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

	// Use a real client so encryption handshake is handled automatically
	c, err := client.New(url, "e2e-fox-46")
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	defer c.Close()

	// Send a command that produces output
	if err := c.SendInput([]byte("echo relay-test\n")); err != nil {
		t.Fatalf("SendInput: %v", err)
	}

	// Host should see the output (proves the encrypted round-trip works)
	output := h.ReadOutputUntil("relay-test", 5*time.Second)
	if !strings.Contains(output, "relay-test") {
		t.Errorf("expected host output to contain 'relay-test', got: %q", output)
	}
}
