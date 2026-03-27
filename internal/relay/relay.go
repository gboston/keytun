// ABOUTME: WebSocket relay server that brokers connections between hosts and clients.
// ABOUTME: Maintains an in-memory session map and bridges messages between paired connections.
package relay

import (
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

// session holds the state for a single keytun session.
type session struct {
	host   *websocket.Conn
	client *websocket.Conn
	mu     sync.Mutex
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
		r.limitersMu.Lock()
		for ip, entry := range r.limiters {
			if time.Since(entry.lastSeen) > 5*time.Minute {
				delete(r.limiters, ip)
			}
		}
		r.limitersMu.Unlock()
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
	sess := &session{host: conn}
	r.sessions[code] = sess
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		delete(r.sessions, code)
		r.mu.Unlock()
		conn.Close()
		sess.mu.Lock()
		if sess.client != nil {
			sess.client.Close()
		}
		sess.mu.Unlock()
	}()

	// Read messages from host and forward to client
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}

		var msg protocol.Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		sess.mu.Lock()
		client := sess.client
		sess.mu.Unlock()

		if client == nil {
			continue
		}

		// Forward output, key exchange, and resize messages to client
		if msg.Type == protocol.MsgOutput || msg.Type == protocol.MsgKeyExchange || msg.Type == protocol.MsgResize {
			if err := client.WriteMessage(websocket.TextMessage, data); err != nil {
				continue
			}
		}
	}
}

func (r *Relay) handleClient(conn *websocket.Conn, code string, ip string) {
	if !r.getLimiter(ip).Allow() {
		sendError(conn, "too many requests")
		conn.Close()
		return
	}

	r.mu.Lock()
	sess, exists := r.sessions[code]
	r.mu.Unlock()

	if !exists {
		sendError(conn, "session not found")
		conn.Close()
		return
	}

	sess.mu.Lock()
	sess.client = conn
	sess.mu.Unlock()

	// Acknowledge to client that they joined successfully
	sendJSON(conn, protocol.Message{
		Type:    protocol.MsgSessionJoined,
		Session: code,
	})

	// Notify host that client joined
	sendJSON(sess.host, protocol.Message{
		Type:  protocol.MsgPeerEvent,
		Event: "joined",
	})

	defer func() {
		conn.Close()
		sess.mu.Lock()
		sess.client = nil
		sess.mu.Unlock()
		// Notify host that client left
		sendJSON(sess.host, protocol.Message{
			Type:  protocol.MsgPeerEvent,
			Event: "left",
		})
	}()

	// Read messages from client and forward to host
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}

		var msg protocol.Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		// Forward input and key exchange messages to host
		if msg.Type == protocol.MsgInput || msg.Type == protocol.MsgKeyExchange {
			if err := sess.host.WriteMessage(websocket.TextMessage, data); err != nil {
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
