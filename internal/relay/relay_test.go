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

func TestResizeFlowsFromHostToClient(t *testing.T) {
	server, _ := newTestServer(t)

	host := dialWS(t, server)
	defer host.Close()
	sendMsg(t, host, protocol.Message{
		Type:    protocol.MsgHostRegister,
		Session: "test-fox-resize",
	})

	client := dialWS(t, server)
	defer client.Close()
	sendMsg(t, client, protocol.Message{
		Type:    protocol.MsgClientJoin,
		Session: "test-fox-resize",
	})

	// Read peer_event on host, session_joined on client
	readMsg(t, host)
	readMsg(t, client)

	// Host sends resize
	sendMsg(t, host, protocol.Message{
		Type: protocol.MsgResize,
		Cols: 120,
		Rows: 40,
	})

	// Client should receive resize
	msg := readMsg(t, client)
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

func TestCheckOrigin(t *testing.T) {
	cases := []struct {
		origin  string
		allowed bool
	}{
		{"", true},
		{"https://keytun.com", true},
		{"https://www.keytun.com", true},
		{"http://localhost", true},
		{"http://localhost:3000", true},
		{"http://127.0.0.1:8080", true},
		{"https://evil.com", false},
		{"https://notkeytun.com", false},
		{"https://fakeytun.com", false},
	}

	for _, tc := range cases {
		req, _ := http.NewRequest("GET", "/ws", nil)
		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		}
		got := checkOrigin(req)
		if got != tc.allowed {
			t.Errorf("checkOrigin(%q) = %v, want %v", tc.origin, got, tc.allowed)
		}
	}
}

func TestRealIPPrefersCFConnectingIP(t *testing.T) {
	req, _ := http.NewRequest("GET", "/ws", nil)
	req.RemoteAddr = "1.2.3.4:5678"
	req.Header.Set("CF-Connecting-IP", "9.9.9.9")
	if got := realIP(req); got != "9.9.9.9" {
		t.Errorf("realIP = %q, want %q", got, "9.9.9.9")
	}
}

func TestRealIPFallsBackToRemoteAddr(t *testing.T) {
	req, _ := http.NewRequest("GET", "/ws", nil)
	req.RemoteAddr = "1.2.3.4:5678"
	if got := realIP(req); got != "1.2.3.4" {
		t.Errorf("realIP = %q, want %q", got, "1.2.3.4")
	}
}

func TestRateLimitBlocksExcessiveJoinAttempts(t *testing.T) {
	r := New()
	r.joinBurst = 2 // low burst to test without 10+ connections
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", r.HandleWebSocket)
	server := httptest.NewServer(mux)
	t.Cleanup(func() { server.Close() })

	// Register a host so joins can succeed (avoids "session not found" for first attempts)
	host := dialWS(t, server)
	defer host.Close()
	sendMsg(t, host, protocol.Message{Type: protocol.MsgHostRegister, Session: "rate-test-01"})

	// Exhaust the burst allowance
	for i := 0; i < r.joinBurst; i++ {
		conn := dialWS(t, server)
		sendMsg(t, conn, protocol.Message{Type: protocol.MsgClientJoin, Session: "rate-test-01"})
		conn.Close()
	}

	// Next attempt should be rate limited
	conn := dialWS(t, server)
	defer conn.Close()
	sendMsg(t, conn, protocol.Message{Type: protocol.MsgClientJoin, Session: "rate-test-01"})
	msg := readMsg(t, conn)
	if msg.Type != protocol.MsgError || msg.ErrMessage != "too many requests" {
		t.Errorf("expected rate limit error, got %+v", msg)
	}
}

func dialWSWithHeader(t *testing.T, server *httptest.Server, header http.Header) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(url, header)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return conn
}

func TestRateLimitUsesCFConnectingIP(t *testing.T) {
	r := New()
	r.joinBurst = 2
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", r.HandleWebSocket)
	server := httptest.NewServer(mux)
	t.Cleanup(func() { server.Close() })

	host := dialWS(t, server)
	defer host.Close()
	sendMsg(t, host, protocol.Message{Type: protocol.MsgHostRegister, Session: "cf-rate-test"})

	// Exhaust burst for a specific CF IP
	cfHeader := http.Header{"CF-Connecting-IP": []string{"5.6.7.8"}}
	for i := 0; i < r.joinBurst; i++ {
		conn := dialWSWithHeader(t, server, cfHeader)
		sendMsg(t, conn, protocol.Message{Type: protocol.MsgClientJoin, Session: "cf-rate-test"})
		conn.Close()
	}

	// Same CF IP should now be rate limited
	conn := dialWSWithHeader(t, server, cfHeader)
	defer conn.Close()
	sendMsg(t, conn, protocol.Message{Type: protocol.MsgClientJoin, Session: "cf-rate-test"})
	msg := readMsg(t, conn)
	if msg.Type != protocol.MsgError || msg.ErrMessage != "too many requests" {
		t.Errorf("expected rate limit error for CF IP, got %+v", msg)
	}

	// A different CF IP should still be allowed
	otherHeader := http.Header{"CF-Connecting-IP": []string{"9.9.9.9"}}
	conn2 := dialWSWithHeader(t, server, otherHeader)
	defer conn2.Close()
	sendMsg(t, conn2, protocol.Message{Type: protocol.MsgClientJoin, Session: "cf-rate-test"})
	msg2 := readMsg(t, conn2)
	if msg2.Type == protocol.MsgError && msg2.ErrMessage == "too many requests" {
		t.Error("different CF IP should not be rate limited")
	}
}

func TestRateLimitGetLimiterSameIPReturnsSameLimiter(t *testing.T) {
	r := New()
	l1 := r.getLimiter("10.0.0.1")
	l2 := r.getLimiter("10.0.0.1")
	if l1 != l2 {
		t.Error("expected same limiter for same IP")
	}
}

func TestRateLimitGetLimiterDifferentIPsReturnDifferentLimiters(t *testing.T) {
	r := New()
	l1 := r.getLimiter("10.0.0.1")
	l2 := r.getLimiter("10.0.0.2")
	if l1 == l2 {
		t.Error("expected different limiters for different IPs")
	}
}

func TestCheckOriginRejectedCannotDial(t *testing.T) {
	server, _ := newTestServer(t)
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	header := http.Header{"Origin": []string{"https://evil.com"}}
	_, resp, err := websocket.DefaultDialer.Dial(url, header)
	if err == nil {
		t.Fatal("expected dial to fail for disallowed origin, but it succeeded")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden, got: %v", resp)
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
