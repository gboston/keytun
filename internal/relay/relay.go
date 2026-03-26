// ABOUTME: WebSocket relay server that brokers connections between hosts and clients.
// ABOUTME: Maintains an in-memory session map and bridges messages between paired connections.
package relay

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gboston/keytun/internal/protocol"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// session holds the state for a single keytun session.
type session struct {
	host   *websocket.Conn
	client *websocket.Conn
	mu     sync.Mutex
}

// Relay is the WebSocket broker that manages sessions.
type Relay struct {
	sessions map[string]*session
	mu       sync.Mutex
}

// New creates a new Relay.
func New() *Relay {
	return &Relay{
		sessions: make(map[string]*session),
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
		r.handleClient(conn, msg.Session)
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

func (r *Relay) handleClient(conn *websocket.Conn, code string) {
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
