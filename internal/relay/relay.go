// ABOUTME: WebSocket relay server that brokers connections between hosts and clients.
// ABOUTME: Maintains an in-memory session map and bridges messages between paired connections.
package relay

import (
	crypto_rand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gboston/keytun/internal/protocol"
	"github.com/gorilla/websocket"
	"golang.org/x/time/rate"
)

// generateClientID returns an 8-character hex string from crypto/rand.
func generateClientID() string {
	b := make([]byte, 4)
	if _, err := crypto_rand.Read(b); err != nil {
		log.Fatalf("crypto/rand: %v", err)
	}
	return hex.EncodeToString(b)
}

// realIP returns the client's IP address. It prefers the CF-Connecting-IP
// header set by Cloudflare, then falls back to the TCP remote address.
func realIP(r *http.Request) string {
	if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
		return ip
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip == "" {
		return r.RemoteAddr
	}
	return ip
}

// checkOrigin allows CLI clients (no Origin header) and browser connections
// from keytun.com or localhost. Rejects all other browser origins.
func checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	if strings.HasPrefix(origin, "http://localhost") || strings.HasPrefix(origin, "http://127.0.0.1") {
		return true
	}
	allowed := []string{"https://keytun.com", "https://www.keytun.com"}
	for _, o := range allowed {
		if origin == o {
			return true
		}
	}
	return false
}

var upgrader = websocket.Upgrader{CheckOrigin: checkOrigin}

// maxMessageSize limits the size of a single WebSocket message on the relay
// to prevent OOM from malicious peers. 64KB is generous for keystroke and
// terminal output messages.
const maxMessageSize = 64 * 1024

// pingInterval is how often the relay sends WebSocket pings to detect dead connections.
const pingInterval = 30 * time.Second

// pongTimeout is how long the relay waits for a pong before considering the connection dead.
// Must be greater than pingInterval.
const pongTimeout = 40 * time.Second

// session holds the state for a single keytun session.
type session struct {
	host     *websocket.Conn
	hostMu   sync.Mutex      // serializes writes to the host connection
	clients  map[string]*websocket.Conn // clientID -> connection
	clientMu sync.RWMutex    // protects the clients map
}

// ipEntry tracks a per-IP rate limiter and when it was last used.
type ipEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// Relay is the WebSocket broker that manages sessions.
type Relay struct {
	sessions   map[string]*session
	mu         sync.Mutex
	limiters   map[string]*ipEntry
	limitersMu sync.Mutex
	// joinBurst controls how many join attempts an IP may make before being
	// rate limited. Defaults to 10 (one per 6 seconds steady-state).
	joinBurst int
}

// New creates a new Relay and starts background cleanup of stale rate limiters.
func New() *Relay {
	r := &Relay{
		sessions:  make(map[string]*session),
		limiters:  make(map[string]*ipEntry),
		joinBurst: 10,
	}
	go r.cleanupLimiters()
	return r
}

// getLimiter returns the rate limiter for the given IP, creating one if needed.
// Each IP may make joinBurst join attempts upfront, then one every 6 seconds.
func (r *Relay) getLimiter(ip string) *rate.Limiter {
	r.limitersMu.Lock()
	defer r.limitersMu.Unlock()
	entry, ok := r.limiters[ip]
	if !ok {
		entry = &ipEntry{
			limiter: rate.NewLimiter(rate.Every(6*time.Second), r.joinBurst),
		}
		r.limiters[ip] = entry
	}
	entry.lastSeen = time.Now()
	return entry.limiter
}

// cleanupLimiters removes per-IP entries that have not been used in 5 minutes.
func (r *Relay) cleanupLimiters() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		r.sweepStaleLimiters()
	}
}

// sweepStaleLimiters removes per-IP entries that have not been used in 5 minutes.
func (r *Relay) sweepStaleLimiters() {
	r.limitersMu.Lock()
	for ip, entry := range r.limiters {
		if time.Since(entry.lastSeen) > 5*time.Minute {
			delete(r.limiters, ip)
		}
	}
	r.limitersMu.Unlock()
}

// CloseAllSessions closes all active host and client connections, allowing
// read loops to exit and defer-based cleanup to run.
func (r *Relay) CloseAllSessions() {
	r.mu.Lock()
	snapshot := make([]*session, 0, len(r.sessions))
	for _, sess := range r.sessions {
		snapshot = append(snapshot, sess)
	}
	r.mu.Unlock()
	for _, sess := range snapshot {
		sess.host.Close()
		sess.clientMu.RLock()
		for _, c := range sess.clients {
			c.Close()
		}
		sess.clientMu.RUnlock()
	}
}

// HasSession returns true if a session with the given code exists.
func (r *Relay) HasSession(code string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.sessions[code]
	return ok
}

// HandleWebSocket is the HTTP handler for the /ws endpoint.
func (r *Relay) HandleWebSocket(w http.ResponseWriter, req *http.Request) {
	// Capture the real client IP before the HTTP request is consumed by upgrade.
	ip := realIP(req)

	conn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		log.Printf("upgrade: %v", err)
		return
	}
	conn.SetReadLimit(maxMessageSize)

	// Read the first message to determine role (host or client)
	_, data, err := conn.ReadMessage()
	if err != nil {
		conn.Close()
		return
	}

	var msg protocol.Message
	if err := json.Unmarshal(data, &msg); err != nil {
		sendError(conn, "invalid message")
		conn.Close()
		return
	}

	switch msg.Type {
	case protocol.MsgHostRegister:
		r.handleHost(conn, msg.Session)
	case protocol.MsgClientJoin:
		r.handleClient(conn, msg.Session, ip)
	default:
		sendError(conn, "expected host_register or client_join")
		conn.Close()
	}
}

func (r *Relay) handleHost(conn *websocket.Conn, code string) {
	r.mu.Lock()
	if _, exists := r.sessions[code]; exists {
		r.mu.Unlock()
		sendError(conn, "session code already in use")
		return
	}
	sess := &session{
		host:    conn,
		clients: make(map[string]*websocket.Conn),
	}
	r.sessions[code] = sess
	r.mu.Unlock()

	configureKeepalive(conn)
	done := make(chan struct{})
	go startPinger(conn, &sess.hostMu, done)

	defer func() {
		close(done)
		r.mu.Lock()
		delete(r.sessions, code)
		r.mu.Unlock()
		conn.Close()
		sess.clientMu.RLock()
		for _, c := range sess.clients {
			c.Close()
		}
		sess.clientMu.RUnlock()
	}()

	// Read messages from host and forward to clients.
	// If ClientID is set, route to that specific client; otherwise broadcast.
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}

		var msg protocol.Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		if msg.Type == protocol.MsgOutput || msg.Type == protocol.MsgKeyExchange || msg.Type == protocol.MsgResize {
			sess.clientMu.RLock()
			if msg.ClientID != "" {
				// Targeted: send to a specific client
				if c, ok := sess.clients[msg.ClientID]; ok {
					if err := c.WriteMessage(websocket.TextMessage, data); err != nil {
						log.Printf("write to client %s: %v", msg.ClientID, err)
					}
				}
			} else {
				// Broadcast: send to all clients
				for _, c := range sess.clients {
					if err := c.WriteMessage(websocket.TextMessage, data); err != nil {
						log.Printf("write to client: %v", err)
					}
				}
			}
			sess.clientMu.RUnlock()
		}
	}
}

func (r *Relay) handleClient(conn *websocket.Conn, code string, ip string) {
	if !r.getLimiter(ip).Allow() {
		sendError(conn, "too many requests")
		conn.Close()
		return
	}

	clientID := generateClientID()

	// Hold r.mu while looking up the session and adding the client so that
	// a concurrent host disconnect cannot remove the session in between.
	r.mu.Lock()
	sess, exists := r.sessions[code]
	if !exists {
		r.mu.Unlock()
		sendError(conn, "session not found")
		conn.Close()
		return
	}
	r.mu.Unlock()

	configureKeepalive(conn)
	var clientWriteMu sync.Mutex
	done := make(chan struct{})
	go startPinger(conn, &clientWriteMu, done)

	// Acknowledge to client that they joined successfully
	sendJSON(conn, protocol.Message{
		Type:     protocol.MsgSessionJoined,
		Session:  code,
		ClientID: clientID,
	})

	// Add to session's client map
	sess.clientMu.Lock()
	sess.clients[clientID] = conn
	sess.clientMu.Unlock()

	// Notify host that client joined (with ClientID)
	if err := sess.sendToHost(protocol.Message{
		Type:     protocol.MsgPeerEvent,
		Event:    "joined",
		ClientID: clientID,
	}); err != nil {
		log.Printf("notify host of client join: %v", err)
	}

	defer func() {
		close(done)
		conn.Close()
		sess.clientMu.Lock()
		delete(sess.clients, clientID)
		sess.clientMu.Unlock()
		// Notify host that client left (with ClientID)
		if err := sess.sendToHost(protocol.Message{
			Type:     protocol.MsgPeerEvent,
			Event:    "left",
			ClientID: clientID,
		}); err != nil {
			log.Printf("notify host of client leave: %v", err)
		}
	}()

	// Read messages from client and forward to host with ClientID injected
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}

		var msg protocol.Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		// Forward input and key exchange messages to host, tagging with this client's ID
		if msg.Type == protocol.MsgInput || msg.Type == protocol.MsgKeyExchange {
			msg.ClientID = clientID
			tagged, err := json.Marshal(msg)
			if err != nil {
				continue
			}
			sess.hostMu.Lock()
			err = sess.host.WriteMessage(websocket.TextMessage, tagged)
			sess.hostMu.Unlock()
			if err != nil {
				return
			}
		}
	}
}

// sendToHost serializes writes to the host connection.
func (s *session) sendToHost(msg protocol.Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	s.hostMu.Lock()
	defer s.hostMu.Unlock()
	return s.host.WriteMessage(websocket.TextMessage, data)
}

// configureKeepalive sets up pong handling and an initial read deadline on a
// connection. Each pong resets the deadline so idle connections are reaped.
func configureKeepalive(conn *websocket.Conn) {
	conn.SetReadDeadline(time.Now().Add(pongTimeout))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongTimeout))
		return nil
	})
}

// startPinger sends periodic WebSocket pings until done is closed.
// The write mutex mu must serialize all writes to conn.
func startPinger(conn *websocket.Conn, mu *sync.Mutex, done <-chan struct{}) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			mu.Lock()
			err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second))
			mu.Unlock()
			if err != nil {
				return
			}
		}
	}
}

func sendError(conn *websocket.Conn, message string) {
	sendJSON(conn, protocol.Message{
		Type:       protocol.MsgError,
		ErrMessage: message,
	})
}

func sendJSON(conn *websocket.Conn, msg protocol.Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}
