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
	host := registerHost(t, server, "test-fox-42")
	defer host.Close()

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

	host := registerHost(t, server, "test-fox-43")
	defer host.Close()

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

	host := registerHost(t, server, "test-fox-44")
	defer host.Close()

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

	host := registerHost(t, server, "test-fox-45")

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

	host := registerHost(t, server, "test-fox-46")
	defer host.Close()

	client := dialWS(t, server)
	sendMsg(t, client, protocol.Message{
		Type:    protocol.MsgClientJoin,
		Session: "test-fox-46",
	})

	// Read peer_event "joined"
	joinMsg := readMsg(t, host)
	if joinMsg.ClientID == "" {
		t.Error("peer_event joined should include a ClientID")
	}

	// Client disconnects
	client.Close()

	// Host should get peer_event "left" with the same ClientID
	msg := readMsg(t, host)
	if msg.Type != protocol.MsgPeerEvent || msg.Event != "left" {
		t.Errorf("expected peer_event left, got %+v", msg)
	}
	if msg.ClientID != joinMsg.ClientID {
		t.Errorf("left ClientID = %q, want %q", msg.ClientID, joinMsg.ClientID)
	}
}

func TestKeyExchangeFlowsBidirectionally(t *testing.T) {
	server, _ := newTestServer(t)

	host := registerHost(t, server, "test-fox-kx")
	defer host.Close()

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

	host := registerHost(t, server, "test-fox-resize")
	defer host.Close()

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
	host := registerHost(t, server, "rate-test-01")
	defer host.Close()

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

	host := registerHost(t, server, "cf-rate-test")
	defer host.Close()

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

func TestHandleWebSocketInvalidJSON(t *testing.T) {
	server, _ := newTestServer(t)

	conn := dialWS(t, server)
	defer conn.Close()

	// Send invalid JSON as the first message
	conn.WriteMessage(websocket.TextMessage, []byte("not json"))

	msg := readMsg(t, conn)
	if msg.Type != protocol.MsgError {
		t.Errorf("expected error for invalid JSON, got %+v", msg)
	}
	if msg.ErrMessage != "invalid message" {
		t.Errorf("message = %q, want %q", msg.ErrMessage, "invalid message")
	}
}

func TestHandleWebSocketUnknownMessageType(t *testing.T) {
	server, _ := newTestServer(t)

	conn := dialWS(t, server)
	defer conn.Close()

	sendMsg(t, conn, protocol.Message{
		Type: "unknown_type",
	})

	msg := readMsg(t, conn)
	if msg.Type != protocol.MsgError {
		t.Errorf("expected error for unknown type, got %+v", msg)
	}
	if msg.ErrMessage != "expected host_register or client_join" {
		t.Errorf("message = %q, want %q", msg.ErrMessage, "expected host_register or client_join")
	}
}

func TestHandleWebSocketDisconnectBeforeFirstMessage(t *testing.T) {
	server, _ := newTestServer(t)

	conn := dialWS(t, server)
	// Close immediately without sending anything — should not panic
	conn.Close()

	// Give the handler a moment to process
	time.Sleep(50 * time.Millisecond)
}

func TestMultipleClientsJoinSameSession(t *testing.T) {
	server, _ := newTestServer(t)

	host := registerHost(t, server, "test-multi-01")
	defer host.Close()

	// Client A joins
	clientA := dialWS(t, server)
	defer clientA.Close()
	sendMsg(t, clientA, protocol.Message{
		Type:    protocol.MsgClientJoin,
		Session: "test-multi-01",
	})

	// Host should get peer_event with a ClientID
	msgA := readMsg(t, host)
	if msgA.Type != protocol.MsgPeerEvent || msgA.Event != "joined" {
		t.Fatalf("expected peer_event joined for client A, got %+v", msgA)
	}
	if msgA.ClientID == "" {
		t.Fatal("peer_event for client A should have a ClientID")
	}

	// Client B joins
	clientB := dialWS(t, server)
	defer clientB.Close()
	sendMsg(t, clientB, protocol.Message{
		Type:    protocol.MsgClientJoin,
		Session: "test-multi-01",
	})

	// Host should get peer_event with a different ClientID
	msgB := readMsg(t, host)
	if msgB.Type != protocol.MsgPeerEvent || msgB.Event != "joined" {
		t.Fatalf("expected peer_event joined for client B, got %+v", msgB)
	}
	if msgB.ClientID == "" {
		t.Fatal("peer_event for client B should have a ClientID")
	}
	if msgA.ClientID == msgB.ClientID {
		t.Errorf("client IDs should be different: both are %q", msgA.ClientID)
	}
}

func TestInputRoutedWithClientID(t *testing.T) {
	server, _ := newTestServer(t)

	host := registerHost(t, server, "test-multi-02")
	defer host.Close()

	// Two clients join
	clientA := dialWS(t, server)
	defer clientA.Close()
	sendMsg(t, clientA, protocol.Message{
		Type:    protocol.MsgClientJoin,
		Session: "test-multi-02",
	})
	peerA := readMsg(t, host)
	idA := peerA.ClientID

	clientB := dialWS(t, server)
	defer clientB.Close()
	sendMsg(t, clientB, protocol.Message{
		Type:    protocol.MsgClientJoin,
		Session: "test-multi-02",
	})
	peerB := readMsg(t, host)
	idB := peerB.ClientID

	// Client A sends input
	sendMsg(t, clientA, protocol.Message{
		Type: protocol.MsgInput,
		Data: "ZnJvbUE=",
	})
	msgFromA := readMsg(t, host)
	if msgFromA.ClientID != idA {
		t.Errorf("input from A: ClientID = %q, want %q", msgFromA.ClientID, idA)
	}

	// Client B sends input
	sendMsg(t, clientB, protocol.Message{
		Type: protocol.MsgInput,
		Data: "ZnJvbUI=",
	})
	msgFromB := readMsg(t, host)
	if msgFromB.ClientID != idB {
		t.Errorf("input from B: ClientID = %q, want %q", msgFromB.ClientID, idB)
	}
}

func TestTargetedOutputRoutesToSpecificClient(t *testing.T) {
	server, _ := newTestServer(t)

	host := registerHost(t, server, "test-multi-03")
	defer host.Close()

	clientA := dialWS(t, server)
	defer clientA.Close()
	sendMsg(t, clientA, protocol.Message{
		Type:    protocol.MsgClientJoin,
		Session: "test-multi-03",
	})
	peerA := readMsg(t, host)
	idA := peerA.ClientID
	readMsg(t, clientA) // session_joined

	clientB := dialWS(t, server)
	defer clientB.Close()
	sendMsg(t, clientB, protocol.Message{
		Type:    protocol.MsgClientJoin,
		Session: "test-multi-03",
	})
	readMsg(t, host) // peer_event for B
	readMsg(t, clientB) // session_joined

	// Host sends output targeted to client A only
	sendMsg(t, host, protocol.Message{
		Type:     protocol.MsgOutput,
		Data:     "Zm9yQQ==",
		ClientID: idA,
	})

	// Client A should receive it
	msgA := readMsg(t, clientA)
	if msgA.Type != protocol.MsgOutput || msgA.Data != "Zm9yQQ==" {
		t.Errorf("client A expected targeted output, got %+v", msgA)
	}

	// Client B should NOT receive it (set a short deadline)
	clientB.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, _, err := clientB.ReadMessage()
	if err == nil {
		t.Error("client B should NOT have received the targeted output")
	}
}

func TestBroadcastOutputRoutesToAllClients(t *testing.T) {
	server, _ := newTestServer(t)

	host := registerHost(t, server, "test-multi-04")
	defer host.Close()

	clientA := dialWS(t, server)
	defer clientA.Close()
	sendMsg(t, clientA, protocol.Message{
		Type:    protocol.MsgClientJoin,
		Session: "test-multi-04",
	})
	readMsg(t, host)    // peer_event
	readMsg(t, clientA) // session_joined

	clientB := dialWS(t, server)
	defer clientB.Close()
	sendMsg(t, clientB, protocol.Message{
		Type:    protocol.MsgClientJoin,
		Session: "test-multi-04",
	})
	readMsg(t, host)    // peer_event
	readMsg(t, clientB) // session_joined

	// Host sends resize (no ClientID = broadcast)
	sendMsg(t, host, protocol.Message{
		Type: protocol.MsgResize,
		Cols: 120,
		Rows: 40,
	})

	// Both clients should receive it
	msgA := readMsg(t, clientA)
	if msgA.Type != protocol.MsgResize || msgA.Cols != 120 {
		t.Errorf("client A expected resize, got %+v", msgA)
	}
	msgB := readMsg(t, clientB)
	if msgB.Type != protocol.MsgResize || msgB.Cols != 120 {
		t.Errorf("client B expected resize, got %+v", msgB)
	}
}

func TestClientDisconnectSendsClientIDInPeerEvent(t *testing.T) {
	server, _ := newTestServer(t)

	host := registerHost(t, server, "test-multi-05")
	defer host.Close()

	client := dialWS(t, server)
	sendMsg(t, client, protocol.Message{
		Type:    protocol.MsgClientJoin,
		Session: "test-multi-05",
	})
	joinMsg := readMsg(t, host)
	clientID := joinMsg.ClientID

	client.Close()

	leftMsg := readMsg(t, host)
	if leftMsg.Type != protocol.MsgPeerEvent || leftMsg.Event != "left" {
		t.Fatalf("expected peer_event left, got %+v", leftMsg)
	}
	if leftMsg.ClientID != clientID {
		t.Errorf("left ClientID = %q, want %q", leftMsg.ClientID, clientID)
	}
}

func TestHostDisconnectClosesAllClients(t *testing.T) {
	server, _ := newTestServer(t)

	host := registerHost(t, server, "test-multi-06")

	clientA := dialWS(t, server)
	defer clientA.Close()
	sendMsg(t, clientA, protocol.Message{
		Type:    protocol.MsgClientJoin,
		Session: "test-multi-06",
	})
	readMsg(t, host)    // peer_event
	readMsg(t, clientA) // session_joined

	clientB := dialWS(t, server)
	defer clientB.Close()
	sendMsg(t, clientB, protocol.Message{
		Type:    protocol.MsgClientJoin,
		Session: "test-multi-06",
	})
	readMsg(t, host)    // peer_event
	readMsg(t, clientB) // session_joined

	// Host disconnects
	host.Close()

	// Both clients should have their connections closed
	clientA.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, errA := clientA.ReadMessage()
	if errA == nil {
		t.Error("client A connection should be closed after host disconnect")
	}
	clientB.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, errB := clientB.ReadMessage()
	if errB == nil {
		t.Error("client B connection should be closed after host disconnect")
	}
}

func TestDuplicateSessionCode(t *testing.T) {
	server, _ := newTestServer(t)

	host1 := registerHost(t, server, "test-fox-47")
	defer host1.Close()

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

func TestSecondClientDoesNotDisplaceFirst(t *testing.T) {
	server, _ := newTestServer(t)

	host := registerHost(t, server, "test-fox-coexist")
	defer host.Close()

	// First client joins
	client1 := dialWS(t, server)
	defer client1.Close()
	sendMsg(t, client1, protocol.Message{
		Type:    protocol.MsgClientJoin,
		Session: "test-fox-coexist",
	})
	readMsg(t, host)    // peer_event
	readMsg(t, client1) // session_joined

	// Second client joins — should NOT displace the first
	client2 := dialWS(t, server)
	defer client2.Close()
	sendMsg(t, client2, protocol.Message{
		Type:    protocol.MsgClientJoin,
		Session: "test-fox-coexist",
	})
	readMsg(t, host)    // peer_event
	readMsg(t, client2) // session_joined

	// First client should still be able to send input
	sendMsg(t, client1, protocol.Message{
		Type: protocol.MsgInput,
		Data: "c3RpbGxoZXJl",
	})
	msg := readMsg(t, host)
	if msg.Type != protocol.MsgInput || msg.Data != "c3RpbGxoZXJl" {
		t.Errorf("expected input from client1, got %+v", msg)
	}
}

func TestCloseAllSessionsClosesConnections(t *testing.T) {
	server, r := newTestServer(t)

	// Register a host
	host := registerHost(t, server, "test-close-all-01")
	defer host.Close()

	// Client joins
	client := dialWS(t, server)
	defer client.Close()
	sendMsg(t, client, protocol.Message{
		Type:    protocol.MsgClientJoin,
		Session: "test-close-all-01",
	})

	// Read peer_event on host, session_joined on client
	readMsg(t, host)
	readMsg(t, client)

	// Close all sessions
	r.CloseAllSessions()

	// Both connections should be closed
	host.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err := host.ReadMessage()
	if err == nil {
		t.Error("host connection should be closed after CloseAllSessions")
	}
	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err = client.ReadMessage()
	if err == nil {
		t.Error("client connection should be closed after CloseAllSessions")
	}
}

func TestCloseAllSessionsMultipleSessions(t *testing.T) {
	server, r := newTestServer(t)

	// Register two hosts with different sessions
	host1 := registerHost(t, server, "test-close-all-02a")
	defer host1.Close()

	host2 := registerHost(t, server, "test-close-all-02b")
	defer host2.Close()

	waitFor(t, func() bool {
		return r.HasSession("test-close-all-02a") && r.HasSession("test-close-all-02b")
	}, 2*time.Second, "both sessions should exist")

	r.CloseAllSessions()

	// Both hosts should have their connections closed
	host1.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err := host1.ReadMessage()
	if err == nil {
		t.Error("host1 connection should be closed")
	}
	host2.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err = host2.ReadMessage()
	if err == nil {
		t.Error("host2 connection should be closed")
	}
}

func TestRealIPFallsBackToRawRemoteAddr(t *testing.T) {
	// When RemoteAddr has no port (no colon), SplitHostPort returns empty ip
	req, _ := http.NewRequest("GET", "/ws", nil)
	req.RemoteAddr = "bad-addr"
	if got := realIP(req); got != "bad-addr" {
		t.Errorf("realIP = %q, want %q", got, "bad-addr")
	}
}

func TestHostSendsInvalidJSONToRelay(t *testing.T) {
	server, _ := newTestServer(t)

	host := registerHost(t, server, "test-badjson-host")
	defer host.Close()

	client := dialWS(t, server)
	defer client.Close()
	sendMsg(t, client, protocol.Message{
		Type:    protocol.MsgClientJoin,
		Session: "test-badjson-host",
	})

	// Read peer_event on host, session_joined on client
	readMsg(t, host)
	readMsg(t, client)

	// Host sends invalid JSON — relay should silently skip it, not crash
	host.WriteMessage(websocket.TextMessage, []byte("not valid json"))

	// Host can still send valid messages after that
	sendMsg(t, host, protocol.Message{
		Type: protocol.MsgOutput,
		Data: "YWZ0ZXI=",
	})

	msg := readMsg(t, client)
	if msg.Type != protocol.MsgOutput || msg.Data != "YWZ0ZXI=" {
		t.Errorf("expected output after invalid JSON, got %+v", msg)
	}
}

func TestClientSendsInvalidJSONToRelay(t *testing.T) {
	server, _ := newTestServer(t)

	host := registerHost(t, server, "test-badjson-client")
	defer host.Close()

	client := dialWS(t, server)
	defer client.Close()
	sendMsg(t, client, protocol.Message{
		Type:    protocol.MsgClientJoin,
		Session: "test-badjson-client",
	})

	// Read peer_event on host, session_joined on client
	readMsg(t, host)
	readMsg(t, client)

	// Client sends invalid JSON — relay should skip it
	client.WriteMessage(websocket.TextMessage, []byte("{bad"))

	// Client can still send valid input after that
	sendMsg(t, client, protocol.Message{
		Type: protocol.MsgInput,
		Data: "cmVjb3Zlcg==",
	})

	msg := readMsg(t, host)
	if msg.Type != protocol.MsgInput || msg.Data != "cmVjb3Zlcg==" {
		t.Errorf("expected input after invalid JSON, got %+v", msg)
	}
}

func TestOversizedMessageRejected(t *testing.T) {
	server, _ := newTestServer(t)

	host := registerHost(t, server, "test-fox-oversize")
	defer host.Close()

	client := dialWS(t, server)
	defer client.Close()
	sendMsg(t, client, protocol.Message{
		Type:    protocol.MsgClientJoin,
		Session: "test-fox-oversize",
	})

	// Read peer_event on host, session_joined on client
	readMsg(t, host)
	readMsg(t, client)

	// Send a message that exceeds the relay's read limit
	oversized := make([]byte, maxMessageSize+1)
	for i := range oversized {
		oversized[i] = 'A'
	}
	err := client.WriteMessage(websocket.TextMessage, oversized)
	if err != nil {
		// Write might fail if connection already closed, which is fine
		return
	}

	// The relay should close the connection after the oversized read
	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err = client.ReadMessage()
	if err == nil {
		t.Error("expected connection to be closed after oversized message")
	}
}
