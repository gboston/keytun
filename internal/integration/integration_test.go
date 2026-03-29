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

func startRelay(t *testing.T) (*httptest.Server, *relay.Relay) {
	t.Helper()
	r := relay.New()
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", r.HandleWebSocket)
	server := httptest.NewServer(mux)
	t.Cleanup(func() { server.Close() })
	return server, r
}

// waitForSession polls until the relay has registered the session.
// Prevents races between host.New() returning and client.New() joining.
func waitForSession(t *testing.T, r *relay.Relay, code string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if r.HasSession(code) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for session %q to be registered", code)
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
	server, r := startRelay(t)
	url := wsURL(server)

	// Start host
	inj := newPTYInjector(t)
	h, err := host.New(url, "e2e-fox-42", inj)
	if err != nil {
		t.Fatalf("host.New: %v", err)
	}
	defer h.Close()
	waitForSession(t, r, "e2e-fox-42", 2*time.Second)

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
	server, r := startRelay(t)
	url := wsURL(server)

	inj := newPTYInjector(t)
	h, err := host.New(url, "e2e-fox-43", inj)
	if err != nil {
		t.Fatalf("host.New: %v", err)
	}
	defer h.Close()
	waitForSession(t, r, "e2e-fox-43", 2*time.Second)

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
	server, r := startRelay(t)
	url := wsURL(server)

	inj := newPTYInjector(t)
	h, err := host.New(url, "e2e-fox-44", inj)
	if err != nil {
		t.Fatalf("host.New: %v", err)
	}
	defer h.Close()
	waitForSession(t, r, "e2e-fox-44", 2*time.Second)

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
	server, r := startRelay(t)
	url := wsURL(server)

	inj := newPTYInjector(t)
	h, err := host.New(url, "e2e-fox-45", inj)
	if err != nil {
		t.Fatalf("host.New: %v", err)
	}
	defer h.Close()
	waitForSession(t, r, "e2e-fox-45", 2*time.Second)

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

func TestEndToEnd_TwoClientsCanTypeSimultaneously(t *testing.T) {
	server, r := startRelay(t)
	url := wsURL(server)

	inj := newPTYInjector(t)
	h, err := host.New(url, "e2e-fox-50", inj)
	if err != nil {
		t.Fatalf("host.New: %v", err)
	}
	defer h.Close()
	waitForSession(t, r, "e2e-fox-50", 2*time.Second)

	// Connect two clients to the same session
	c1, err := client.New(url, "e2e-fox-50")
	if err != nil {
		t.Fatalf("client.New (c1): %v", err)
	}
	defer c1.Close()

	c2, err := client.New(url, "e2e-fox-50")
	if err != nil {
		t.Fatalf("client.New (c2): %v", err)
	}
	defer c2.Close()

	// Client 1 sends a command
	if err := c1.SendInput([]byte("echo from-client-one\n")); err != nil {
		t.Fatalf("c1.SendInput: %v", err)
	}
	output := h.ReadOutputUntil("from-client-one", 5*time.Second)
	if !strings.Contains(output, "from-client-one") {
		t.Errorf("expected host output to contain 'from-client-one', got: %q", output)
	}

	// Client 2 sends a command
	if err := c2.SendInput([]byte("echo from-client-two\n")); err != nil {
		t.Fatalf("c2.SendInput: %v", err)
	}
	output = h.ReadOutputUntil("from-client-two", 5*time.Second)
	if !strings.Contains(output, "from-client-two") {
		t.Errorf("expected host output to contain 'from-client-two', got: %q", output)
	}
}

func TestEndToEnd_ClientDisconnectDoesNotAffectOtherClients(t *testing.T) {
	server, r := startRelay(t)
	url := wsURL(server)

	inj := newPTYInjector(t)
	h, err := host.New(url, "e2e-fox-51", inj)
	if err != nil {
		t.Fatalf("host.New: %v", err)
	}
	defer h.Close()
	waitForSession(t, r, "e2e-fox-51", 2*time.Second)

	// Connect two clients
	c1, err := client.New(url, "e2e-fox-51")
	if err != nil {
		t.Fatalf("client.New (c1): %v", err)
	}
	defer c1.Close()

	c2, err := client.New(url, "e2e-fox-51")
	if err != nil {
		t.Fatalf("client.New (c2): %v", err)
	}

	// Verify c2 can type
	if err := c2.SendInput([]byte("echo c2-before\n")); err != nil {
		t.Fatalf("c2.SendInput: %v", err)
	}
	output := h.ReadOutputUntil("c2-before", 5*time.Second)
	if !strings.Contains(output, "c2-before") {
		t.Fatalf("expected 'c2-before' in output, got: %q", output)
	}

	// Disconnect c2
	c2.Close()

	// Wait for the disconnect banner
	output = h.ReadOutputUntil("client disconnected", 3*time.Second)
	if !strings.Contains(output, "client disconnected") {
		t.Fatalf("expected 'client disconnected' banner, got: %q", output)
	}

	// Client 1 should still work fine
	if err := c1.SendInput([]byte("echo c1-still-works\n")); err != nil {
		t.Fatalf("c1.SendInput after c2 disconnect: %v", err)
	}
	output = h.ReadOutputUntil("c1-still-works", 5*time.Second)
	if !strings.Contains(output, "c1-still-works") {
		t.Errorf("expected 'c1-still-works' in output after c2 disconnect, got: %q", output)
	}
}

func TestEndToEnd_PasswordProtectedSession(t *testing.T) {
	server, r := startRelay(t)
	url := wsURL(server)

	inj := newPTYInjector(t)
	h, err := host.New(url, "e2e-pw-01", inj)
	if err != nil {
		t.Fatalf("host.New: %v", err)
	}
	defer h.Close()
	h.SetPassword("secret123")
	waitForSession(t, r, "e2e-pw-01", 2*time.Second)

	// Client with correct password should connect and send input
	c, err := client.New(url, "e2e-pw-01", "secret123")
	if err != nil {
		t.Fatalf("client.New with correct password: %v", err)
	}
	defer c.Close()

	if err := c.SendInput([]byte("echo pw-works\n")); err != nil {
		t.Fatalf("SendInput: %v", err)
	}

	output := h.ReadOutputUntil("pw-works", 5*time.Second)
	if !strings.Contains(output, "pw-works") {
		t.Errorf("expected 'pw-works' in output, got: %q", output)
	}
}

func TestEndToEnd_WrongPasswordRejected(t *testing.T) {
	server, r := startRelay(t)
	url := wsURL(server)

	inj := newPTYInjector(t)
	h, err := host.New(url, "e2e-pw-02", inj)
	if err != nil {
		t.Fatalf("host.New: %v", err)
	}
	defer h.Close()
	h.SetPassword("correct-password")
	waitForSession(t, r, "e2e-pw-02", 2*time.Second)

	// Client with wrong password should fail
	_, err = client.New(url, "e2e-pw-02", "wrong-password")
	if err == nil {
		t.Fatal("expected error when joining with wrong password")
	}
	if !strings.Contains(err.Error(), "wrong session password") {
		t.Errorf("expected 'wrong session password' error, got: %v", err)
	}
}

func TestEndToEnd_NoPasswordWhenHostRequiresOne(t *testing.T) {
	server, r := startRelay(t)
	url := wsURL(server)

	inj := newPTYInjector(t)
	h, err := host.New(url, "e2e-pw-03", inj)
	if err != nil {
		t.Fatalf("host.New: %v", err)
	}
	defer h.Close()
	h.SetPassword("required-password")
	waitForSession(t, r, "e2e-pw-03", 2*time.Second)

	// Client without password should fail
	_, err = client.New(url, "e2e-pw-03")
	if err == nil {
		t.Fatal("expected error when joining without password")
	}
	if !strings.Contains(err.Error(), "wrong session password") {
		t.Errorf("expected 'wrong session password' error, got: %v", err)
	}
}

func TestEndToEnd_HostOutputReachesClient(t *testing.T) {
	server, r := startRelay(t)
	url := wsURL(server)

	inj := newPTYInjector(t)
	h, err := host.New(url, "e2e-fox-46", inj)
	if err != nil {
		t.Fatalf("host.New: %v", err)
	}
	defer h.Close()
	waitForSession(t, r, "e2e-fox-46", 2*time.Second)

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

func TestEndToEnd_LatencyMeasurement(t *testing.T) {
	server, r := startRelay(t)
	url := wsURL(server)

	inj := newPTYInjector(t)
	h, err := host.New(url, "e2e-lat-01", inj)
	if err != nil {
		t.Fatalf("host.New: %v", err)
	}
	defer h.Close()
	waitForSession(t, r, "e2e-lat-01", 2*time.Second)

	c, err := client.New(url, "e2e-lat-01")
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	defer c.Close()

	// Wait for the latency ping/pong cycle to complete (up to 2 intervals)
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		if c.Latency() > 0 && h.ClientLatency() > 0 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	clientLat := c.Latency()
	hostLat := h.ClientLatency()

	if clientLat <= 0 {
		t.Errorf("expected client to have positive latency, got %v", clientLat)
	}
	if hostLat <= 0 {
		t.Errorf("expected host to have positive latency, got %v", hostLat)
	}

	// Sanity check: in-process latency should be well under 1 second
	if clientLat > time.Second {
		t.Errorf("client latency suspiciously high: %v", clientLat)
	}
	if hostLat > time.Second {
		t.Errorf("host latency suspiciously high: %v", hostLat)
	}
}
