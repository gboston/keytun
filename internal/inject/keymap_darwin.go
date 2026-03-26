//go:build darwin

// ABOUTME: Byte-to-CGEvent mapping for macOS keystroke injection.
// ABOUTME: Converts raw terminal bytes and ANSI escape sequences to key events.
package inject

// keyEventType distinguishes how a key event should be posted.
type keyEventType int

const (
	// keyEventUnicode posts a key event with a unicode character string.
	keyEventUnicode keyEventType = iota
	// keyEventKeycode posts a key event with a specific virtual keycode.
	keyEventKeycode
	// keyEventControl posts a key event with a letter key + Control modifier.
	keyEventControl
)

// keyEvent represents a single key event to post via CGEventPost.
type keyEvent struct {
	eventType keyEventType
	char      rune   // for unicode and control events
	keycode   uint16 // for keycode events
}

// macOS virtual keycodes (from Carbon/HIToolbox/Events.h)
const (
	kVK_Return     uint16 = 0x24
	kVK_Tab        uint16 = 0x30
	kVK_Delete     uint16 = 0x33 // Backspace
	kVK_Escape     uint16 = 0x35
	kVK_LeftArrow  uint16 = 0x7B
	kVK_RightArrow uint16 = 0x7C
	kVK_DownArrow  uint16 = 0x7D
	kVK_UpArrow    uint16 = 0x7E
	kVK_Home       uint16 = 0x73
	kVK_End        uint16 = 0x77
	kVK_PageUp     uint16 = 0x74
	kVK_PageDown   uint16 = 0x79
	kVK_F1         uint16 = 0x7A
	kVK_F2         uint16 = 0x78
	kVK_F3         uint16 = 0x63
	kVK_F4         uint16 = 0x76
	kVK_F5         uint16 = 0x60
	kVK_F6         uint16 = 0x61
	kVK_F7         uint16 = 0x62
	kVK_F8         uint16 = 0x64
	kVK_F9         uint16 = 0x65
	kVK_F10        uint16 = 0x6D
	kVK_F11        uint16 = 0x67
	kVK_F12        uint16 = 0x6F
)

// parseKeyEvents converts raw terminal bytes into a sequence of key events.
// It handles printable ASCII, control characters, and ANSI escape sequences.
func parseKeyEvents(data []byte) []keyEvent {
	var events []keyEvent
	i := 0
	for i < len(data) {
		b := data[i]

		switch {
		case b == 0x1B: // Escape
			ev, consumed := parseEscapeSequence(data[i:])
			if ev != nil {
				events = append(events, *ev)
			}
			i += consumed

		case b == 0x0A || b == 0x0D: // Newline or Carriage Return
			events = append(events, keyEvent{eventType: keyEventKeycode, keycode: kVK_Return})
			i++

		case b == 0x09: // Tab
			events = append(events, keyEvent{eventType: keyEventKeycode, keycode: kVK_Tab})
			i++

		case b == 0x7F: // DEL (Backspace)
			events = append(events, keyEvent{eventType: keyEventKeycode, keycode: kVK_Delete})
			i++

		case b >= 0x01 && b <= 0x1A: // Control characters (Ctrl+A through Ctrl+Z)
			letter := rune(b + 0x60) // 0x01 -> 'a', 0x02 -> 'b', etc.
			events = append(events, keyEvent{eventType: keyEventControl, char: letter})
			i++

		case b >= 0x20 && b <= 0x7E: // Printable ASCII
			events = append(events, keyEvent{eventType: keyEventUnicode, char: rune(b)})
			i++

		default:
			// Skip unrecognized bytes
			i++
		}
	}
	return events
}

// parseEscapeSequence parses an ANSI escape sequence starting at data[0] == 0x1B.
// Returns the key event and number of bytes consumed.
func parseEscapeSequence(data []byte) (*keyEvent, int) {
	if len(data) < 2 {
		// Bare escape
		ev := keyEvent{eventType: keyEventKeycode, keycode: kVK_Escape}
		return &ev, 1
	}

	if data[1] != '[' {
		// Escape not followed by '[' — treat as bare escape
		ev := keyEvent{eventType: keyEventKeycode, keycode: kVK_Escape}
		return &ev, 1
	}

	if len(data) < 3 {
		// Incomplete CSI sequence, treat as bare escape
		ev := keyEvent{eventType: keyEventKeycode, keycode: kVK_Escape}
		return &ev, 2
	}

	// CSI sequences: ESC [ <final byte>
	// Simple single-character final byte sequences
	switch data[2] {
	case 'A':
		ev := keyEvent{eventType: keyEventKeycode, keycode: kVK_UpArrow}
		return &ev, 3
	case 'B':
		ev := keyEvent{eventType: keyEventKeycode, keycode: kVK_DownArrow}
		return &ev, 3
	case 'C':
		ev := keyEvent{eventType: keyEventKeycode, keycode: kVK_RightArrow}
		return &ev, 3
	case 'D':
		ev := keyEvent{eventType: keyEventKeycode, keycode: kVK_LeftArrow}
		return &ev, 3
	case 'H':
		ev := keyEvent{eventType: keyEventKeycode, keycode: kVK_Home}
		return &ev, 3
	case 'F':
		ev := keyEvent{eventType: keyEventKeycode, keycode: kVK_End}
		return &ev, 3
	}

	// Extended CSI sequences: ESC [ <number> ~
	// Find the end of the sequence (look for a letter or ~)
	end := 2
	for end < len(data) && data[end] >= '0' && data[end] <= '9' {
		end++
	}
	if end < len(data) && data[end] == '~' {
		// Parse the number
		numStr := string(data[2:end])
		consumed := end + 1 // include the '~'
		switch numStr {
		case "1":
			ev := keyEvent{eventType: keyEventKeycode, keycode: kVK_Home}
			return &ev, consumed
		case "4":
			ev := keyEvent{eventType: keyEventKeycode, keycode: kVK_End}
			return &ev, consumed
		case "5":
			ev := keyEvent{eventType: keyEventKeycode, keycode: kVK_PageUp}
			return &ev, consumed
		case "6":
			ev := keyEvent{eventType: keyEventKeycode, keycode: kVK_PageDown}
			return &ev, consumed
		default:
			// Unknown numeric CSI sequence — skip it
			return nil, consumed
		}
	}

	// Unknown CSI sequence — skip to the end or consume what we have
	if end < len(data) {
		return nil, end + 1
	}
	return nil, len(data)
}
