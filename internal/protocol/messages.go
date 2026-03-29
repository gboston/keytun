// ABOUTME: Defines the keytun WebSocket protocol message types.
// ABOUTME: All messages are JSON with a "type" discriminator field.
package protocol

// MessageType identifies the kind of protocol message.
type MessageType string

const (
	MsgHostRegister   MessageType = "host_register"
	MsgHostRegistered MessageType = "host_registered"
	MsgClientJoin     MessageType = "client_join"
	MsgInput        MessageType = "input"
	MsgOutput       MessageType = "output"
	MsgError        MessageType = "error"
	MsgPeerEvent      MessageType = "peer_event"
	MsgSessionJoined  MessageType = "session_joined"
	MsgKeyExchange    MessageType = "key_exchange"
	MsgVerify         MessageType = "verify"
	MsgResize         MessageType = "resize"
	MsgPing           MessageType = "ping"
	MsgPong           MessageType = "pong"
)

// Message is the envelope for all keytun protocol messages.
// Fields are omitted from JSON when empty, so each message type
// only serializes the fields it uses.
type Message struct {
	Type       MessageType `json:"type"`
	Session    string      `json:"session,omitempty"`
	ClientID   string      `json:"client_id,omitempty"`
	Data       string      `json:"data,omitempty"`
	ErrMessage string      `json:"message,omitempty"`
	Event      string      `json:"event,omitempty"`
	Cols       uint16      `json:"cols,omitempty"`
	Rows       uint16      `json:"rows,omitempty"`
}
