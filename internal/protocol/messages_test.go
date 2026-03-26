// ABOUTME: Tests for the keytun WebSocket protocol message types.
// ABOUTME: Validates JSON serialization/deserialization with type discriminator.
package protocol

import (
	"encoding/json"
	"testing"
)

func TestMarshalHostRegister(t *testing.T) {
	msg := Message{
		Type:    MsgHostRegister,
		Session: "keen-fox-42",
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]interface{}
	json.Unmarshal(data, &raw)
	if raw["type"] != "host_register" {
		t.Errorf("type = %v, want host_register", raw["type"])
	}
	if raw["session"] != "keen-fox-42" {
		t.Errorf("session = %v, want keen-fox-42", raw["session"])
	}
}

func TestMarshalClientJoin(t *testing.T) {
	msg := Message{
		Type:    MsgClientJoin,
		Session: "bold-cat-7",
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]interface{}
	json.Unmarshal(data, &raw)
	if raw["type"] != "client_join" {
		t.Errorf("type = %v, want client_join", raw["type"])
	}
}

func TestMarshalInput(t *testing.T) {
	msg := Message{
		Type: MsgInput,
		Data: "aGVsbG8=", // base64 "hello"
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]interface{}
	json.Unmarshal(data, &raw)
	if raw["type"] != "input" {
		t.Errorf("type = %v, want input", raw["type"])
	}
	if raw["data"] != "aGVsbG8=" {
		t.Errorf("data = %v, want aGVsbG8=", raw["data"])
	}
}

func TestMarshalOutput(t *testing.T) {
	msg := Message{
		Type: MsgOutput,
		Data: "d29ybGQ=",
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]interface{}
	json.Unmarshal(data, &raw)
	if raw["type"] != "output" {
		t.Errorf("type = %v, want output", raw["type"])
	}
}

func TestMarshalError(t *testing.T) {
	msg := Message{
		Type:       MsgError,
		ErrMessage: "session not found",
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]interface{}
	json.Unmarshal(data, &raw)
	if raw["type"] != "error" {
		t.Errorf("type = %v, want error", raw["type"])
	}
	if raw["message"] != "session not found" {
		t.Errorf("message = %v, want 'session not found'", raw["message"])
	}
}

func TestMarshalPeerEvent(t *testing.T) {
	msg := Message{
		Type:  MsgPeerEvent,
		Event: "joined",
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]interface{}
	json.Unmarshal(data, &raw)
	if raw["type"] != "peer_event" {
		t.Errorf("type = %v, want peer_event", raw["type"])
	}
	if raw["event"] != "joined" {
		t.Errorf("event = %v, want joined", raw["event"])
	}
}

func TestUnmarshalRoundTrip(t *testing.T) {
	cases := []Message{
		{Type: MsgHostRegister, Session: "keen-fox-42"},
		{Type: MsgClientJoin, Session: "bold-cat-7"},
		{Type: MsgInput, Data: "aGVsbG8="},
		{Type: MsgOutput, Data: "d29ybGQ="},
		{Type: MsgError, ErrMessage: "session not found"},
		{Type: MsgPeerEvent, Event: "joined"},
		{Type: MsgPeerEvent, Event: "left"},
	}
	for _, tc := range cases {
		data, err := json.Marshal(tc)
		if err != nil {
			t.Fatalf("marshal %v: %v", tc.Type, err)
		}
		var got Message
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("unmarshal %v: %v", tc.Type, err)
		}
		if got.Type != tc.Type {
			t.Errorf("type = %v, want %v", got.Type, tc.Type)
		}
		if got.Session != tc.Session {
			t.Errorf("session = %v, want %v", got.Session, tc.Session)
		}
		if got.Data != tc.Data {
			t.Errorf("data = %v, want %v", got.Data, tc.Data)
		}
		if got.ErrMessage != tc.ErrMessage {
			t.Errorf("message = %v, want %v", got.ErrMessage, tc.ErrMessage)
		}
		if got.Event != tc.Event {
			t.Errorf("event = %v, want %v", got.Event, tc.Event)
		}
	}
}

func TestOmitEmptyFields(t *testing.T) {
	msg := Message{
		Type:    MsgHostRegister,
		Session: "keen-fox-42",
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]interface{}
	json.Unmarshal(data, &raw)
	// Fields that aren't set should be omitted
	if _, ok := raw["data"]; ok {
		t.Error("data field should be omitted for host_register")
	}
	if _, ok := raw["message"]; ok {
		t.Error("message field should be omitted for host_register")
	}
	if _, ok := raw["event"]; ok {
		t.Error("event field should be omitted for host_register")
	}
}
