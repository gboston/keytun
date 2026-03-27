//go:build darwin

// ABOUTME: Tests for the macOS byte-to-CGEvent keymap logic.
// ABOUTME: Validates mapping of printable chars, control chars, and escape sequences.
package inject

import (
	"testing"
)

func TestParseKeyEvents_PrintableASCII(t *testing.T) {
	events := parseKeyEvents([]byte("Hello"))
	if len(events) != 5 {
		t.Fatalf("expected 5 events, got %d", len(events))
	}

	// Each printable char should produce a unicode key event
	expected := []rune{'H', 'e', 'l', 'l', 'o'}
	for i, ev := range events {
		if ev.eventType != keyEventUnicode {
			t.Errorf("event[%d]: expected unicode event, got %v", i, ev.eventType)
		}
		if ev.char != expected[i] {
			t.Errorf("event[%d]: expected char %q, got %q", i, expected[i], ev.char)
		}
	}
}

func TestParseKeyEvents_Space(t *testing.T) {
	events := parseKeyEvents([]byte(" "))
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].eventType != keyEventUnicode {
		t.Errorf("expected unicode event for space")
	}
	if events[0].char != ' ' {
		t.Errorf("expected space char, got %q", events[0].char)
	}
}

func TestParseKeyEvents_Return(t *testing.T) {
	events := parseKeyEvents([]byte{0x0A}) // newline
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].eventType != keyEventKeycode {
		t.Errorf("expected keycode event for return, got %v", events[0].eventType)
	}
	if events[0].keycode != kVK_Return {
		t.Errorf("expected keycode %d (Return), got %d", kVK_Return, events[0].keycode)
	}
}

func TestParseKeyEvents_CarriageReturn(t *testing.T) {
	events := parseKeyEvents([]byte{0x0D}) // \r
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].keycode != kVK_Return {
		t.Errorf("expected Return keycode, got %d", events[0].keycode)
	}
}

func TestParseKeyEvents_Tab(t *testing.T) {
	events := parseKeyEvents([]byte{0x09})
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].eventType != keyEventKeycode {
		t.Errorf("expected keycode event for tab")
	}
	if events[0].keycode != kVK_Tab {
		t.Errorf("expected keycode %d (Tab), got %d", kVK_Tab, events[0].keycode)
	}
}

func TestParseKeyEvents_Backspace(t *testing.T) {
	events := parseKeyEvents([]byte{0x7F})
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].keycode != kVK_Delete {
		t.Errorf("expected keycode %d (Delete/Backspace), got %d", kVK_Delete, events[0].keycode)
	}
}

func TestParseKeyEvents_Escape(t *testing.T) {
	// Bare escape (not followed by '[')
	events := parseKeyEvents([]byte{0x1B})
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].keycode != kVK_Escape {
		t.Errorf("expected keycode %d (Escape), got %d", kVK_Escape, events[0].keycode)
	}
}

func TestParseKeyEvents_ControlChars(t *testing.T) {
	tests := []struct {
		input byte
		name  string
	}{
		{0x01, "Ctrl+A"},
		{0x03, "Ctrl+C"},
		{0x04, "Ctrl+D"},
		{0x1A, "Ctrl+Z"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := parseKeyEvents([]byte{tt.input})
			if len(events) != 1 {
				t.Fatalf("expected 1 event, got %d", len(events))
			}
			if events[0].eventType != keyEventControl {
				t.Errorf("expected control event, got %v", events[0].eventType)
			}
			// Control char 0x01 = Ctrl+A, letter 'a' = 0x61
			expectedChar := rune(tt.input + 0x60)
			if events[0].char != expectedChar {
				t.Errorf("expected char %q, got %q", expectedChar, events[0].char)
			}
		})
	}
}

func TestParseKeyEvents_ArrowKeys(t *testing.T) {
	tests := []struct {
		input   []byte
		name    string
		keycode uint16
	}{
		{[]byte{0x1B, '[', 'A'}, "Up", kVK_UpArrow},
		{[]byte{0x1B, '[', 'B'}, "Down", kVK_DownArrow},
		{[]byte{0x1B, '[', 'C'}, "Right", kVK_RightArrow},
		{[]byte{0x1B, '[', 'D'}, "Left", kVK_LeftArrow},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := parseKeyEvents(tt.input)
			if len(events) != 1 {
				t.Fatalf("expected 1 event, got %d", len(events))
			}
			if events[0].eventType != keyEventKeycode {
				t.Errorf("expected keycode event, got %v", events[0].eventType)
			}
			if events[0].keycode != tt.keycode {
				t.Errorf("expected keycode %d, got %d", tt.keycode, events[0].keycode)
			}
		})
	}
}

func TestParseKeyEvents_HomeEnd(t *testing.T) {
	tests := []struct {
		input   []byte
		name    string
		keycode uint16
	}{
		{[]byte{0x1B, '[', 'H'}, "Home", kVK_Home},
		{[]byte{0x1B, '[', 'F'}, "End", kVK_End},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := parseKeyEvents(tt.input)
			if len(events) != 1 {
				t.Fatalf("expected 1 event, got %d", len(events))
			}
			if events[0].keycode != tt.keycode {
				t.Errorf("expected keycode %d, got %d", tt.keycode, events[0].keycode)
			}
		})
	}
}

func TestParseKeyEvents_MixedInput(t *testing.T) {
	// "ab\n" = 'a', 'b', Return
	events := parseKeyEvents([]byte("ab\n"))
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[0].eventType != keyEventUnicode || events[0].char != 'a' {
		t.Errorf("event[0]: expected 'a', got %v", events[0])
	}
	if events[1].eventType != keyEventUnicode || events[1].char != 'b' {
		t.Errorf("event[1]: expected 'b', got %v", events[1])
	}
	if events[2].eventType != keyEventKeycode || events[2].keycode != kVK_Return {
		t.Errorf("event[2]: expected Return, got %v", events[2])
	}
}

func TestParseKeyEvents_ExtendedCSISequences(t *testing.T) {
	tests := []struct {
		input   []byte
		name    string
		keycode uint16
	}{
		{[]byte{0x1B, '[', '1', '~'}, "Home (CSI 1~)", kVK_Home},
		{[]byte{0x1B, '[', '4', '~'}, "End (CSI 4~)", kVK_End},
		{[]byte{0x1B, '[', '5', '~'}, "PageUp (CSI 5~)", kVK_PageUp},
		{[]byte{0x1B, '[', '6', '~'}, "PageDown (CSI 6~)", kVK_PageDown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := parseKeyEvents(tt.input)
			if len(events) != 1 {
				t.Fatalf("expected 1 event, got %d", len(events))
			}
			if events[0].eventType != keyEventKeycode {
				t.Errorf("expected keycode event, got %v", events[0].eventType)
			}
			if events[0].keycode != tt.keycode {
				t.Errorf("expected keycode %d, got %d", tt.keycode, events[0].keycode)
			}
		})
	}
}

func TestParseKeyEvents_IncompleteCSI(t *testing.T) {
	// ESC [ with no final byte — should produce an escape keycode
	events := parseKeyEvents([]byte{0x1B, '['})
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].keycode != kVK_Escape {
		t.Errorf("expected Escape keycode for incomplete CSI, got %d", events[0].keycode)
	}
}

func TestParseKeyEvents_EscapeNotFollowedByBracket(t *testing.T) {
	// ESC followed by a non-'[' character should produce escape + the char
	events := parseKeyEvents([]byte{0x1B, 'O'})
	if len(events) != 2 {
		t.Fatalf("expected 2 events (escape + 'O'), got %d", len(events))
	}
	if events[0].keycode != kVK_Escape {
		t.Errorf("expected Escape keycode, got %d", events[0].keycode)
	}
	if events[1].char != 'O' {
		t.Errorf("expected 'O' char, got %q", events[1].char)
	}
}

func TestParseKeyEvents_UnknownCSIWithLetterTerminator(t *testing.T) {
	// ESC [ 9 Z — unknown CSI with a letter terminator
	events := parseKeyEvents([]byte{0x1B, '[', '9', 'Z'})
	// Unknown sequences should be skipped without panic
	_ = events
}

func TestParseKeyEvents_UnknownNumericCSI(t *testing.T) {
	// Unknown numeric CSI sequence should be skipped gracefully
	events := parseKeyEvents([]byte{0x1B, '[', '9', '9', '~'})
	if len(events) != 0 {
		t.Errorf("expected 0 events for unknown numeric CSI, got %d", len(events))
	}
}

func TestParseKeyEvents_HighByteSkipped(t *testing.T) {
	// Bytes > 0x7E should be skipped
	events := parseKeyEvents([]byte{0x80, 0xFF})
	if len(events) != 0 {
		t.Errorf("expected 0 events for high bytes, got %d", len(events))
	}
}
