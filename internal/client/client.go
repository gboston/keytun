// ABOUTME: Client-side logic for keytun: connects to a session and sends keystrokes.
// ABOUTME: Captures raw key bytes and forwards them over WebSocket to the relay.
package client

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gbostoen/keytun/internal/protocol"
	"github.com/gorilla/websocket"
)

// Client manages a keytun client session.
type Client struct {
	conn *websocket.Conn
	done chan struct{}
}

// New creates a new Client that connects to the relay and joins a session.
// Returns an error if the session doesn't exist.
func New(relayURL string, sessionCode string) (*Client, error) {
	conn, _, err := websocket.DefaultDialer.Dial(relayURL, nil)
	if err != nil {
		return nil, err
	}

	// Send client_join
	msg := protocol.Message{
		Type:    protocol.MsgClientJoin,
		Session: sessionCode,
	}
	data, _ := json.Marshal(msg)
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		conn.Close()
		return nil, err
	}

	// Wait for either a session_joined ack or an error from the relay.
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, peek, err := conn.ReadMessage()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to read relay response: %w", err)
	}

	var resp protocol.Message
	if err := json.Unmarshal(peek, &resp); err != nil {
		conn.Close()
		return nil, fmt.Errorf("invalid relay response")
	}
	if resp.Type == protocol.MsgError {
		conn.Close()
		return nil, fmt.Errorf("relay: %s", resp.ErrMessage)
	}
	if resp.Type != protocol.MsgSessionJoined {
		conn.Close()
		return nil, fmt.Errorf("unexpected relay response: %s", resp.Type)
	}

	conn.SetReadDeadline(time.Time{})

	c := &Client{
		conn: conn,
		done: make(chan struct{}),
	}

	return c, nil
}

// SendInput sends raw bytes as an input message to the relay.
func (c *Client) SendInput(input []byte) error {
	encoded := base64.StdEncoding.EncodeToString(input)
	msg := protocol.Message{
		Type: protocol.MsgInput,
		Data: encoded,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return c.conn.WriteMessage(websocket.TextMessage, data)
}

// Close shuts down the client connection.
func (c *Client) Close() {
	select {
	case <-c.done:
		return
	default:
		close(c.done)
	}
	c.conn.Close()
}
