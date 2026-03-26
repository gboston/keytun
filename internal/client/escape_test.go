// ABOUTME: Tests for the double-escape disconnect detector.
// ABOUTME: Verifies that two rapid Escape presses trigger disconnect while single Escape passes through.
package client

import (
	"testing"
	"time"
)

func TestEscapeDetector_DoubleEscapeTriggersDisconnect(t *testing.T) {
	d := NewEscapeDetector(500 * time.Millisecond)

	// First Esc: should not trigger disconnect, should hold the byte
	action := d.Feed(0x1B)
	if action != EscapeHeld {
		t.Fatalf("first Esc: want EscapeHeld, got %v", action)
	}

	// Second Esc within timeout: should trigger disconnect
	action = d.Feed(0x1B)
	if action != Disconnect {
		t.Fatalf("second Esc: want Disconnect, got %v", action)
	}
}

func TestEscapeDetector_SingleEscapePassesThrough(t *testing.T) {
	d := NewEscapeDetector(50 * time.Millisecond)

	action := d.Feed(0x1B)
	if action != EscapeHeld {
		t.Fatalf("first Esc: want EscapeHeld, got %v", action)
	}

	// Wait for timeout to expire
	time.Sleep(80 * time.Millisecond)

	// Non-escape byte: should flush the held Esc and pass this byte through
	action = d.Feed('a')
	if action != PassThrough {
		t.Fatalf("after timeout + normal byte: want PassThrough, got %v", action)
	}

	// The pending escape should have been reported
	if !d.HadPendingEscape() {
		t.Fatal("expected pending escape to have been flushed")
	}
}

func TestEscapeDetector_NonEscapeBytesPassThrough(t *testing.T) {
	d := NewEscapeDetector(500 * time.Millisecond)

	action := d.Feed('x')
	if action != PassThrough {
		t.Fatalf("normal byte: want PassThrough, got %v", action)
	}
}

func TestEscapeDetector_EscapeThenNonEscapeQuickly(t *testing.T) {
	d := NewEscapeDetector(500 * time.Millisecond)

	action := d.Feed(0x1B)
	if action != EscapeHeld {
		t.Fatalf("first Esc: want EscapeHeld, got %v", action)
	}

	// Non-escape byte before timeout: flush the held Esc and pass through
	action = d.Feed('a')
	if action != PassThrough {
		t.Fatalf("non-escape after Esc: want PassThrough, got %v", action)
	}
	if !d.HadPendingEscape() {
		t.Fatal("expected pending escape to have been flushed")
	}
}

func TestEscapeDetector_FlushAfterTimeout(t *testing.T) {
	d := NewEscapeDetector(50 * time.Millisecond)

	action := d.Feed(0x1B)
	if action != EscapeHeld {
		t.Fatalf("first Esc: want EscapeHeld, got %v", action)
	}

	time.Sleep(80 * time.Millisecond)

	// Flush should return the held escape
	flushed := d.Flush()
	if !flushed {
		t.Fatal("expected Flush to return true after timeout")
	}

	// Second flush should return false (nothing pending)
	flushed = d.Flush()
	if flushed {
		t.Fatal("expected second Flush to return false")
	}
}

func TestEscapeDetector_FlushBeforeTimeout(t *testing.T) {
	d := NewEscapeDetector(500 * time.Millisecond)

	d.Feed(0x1B)

	// Flush before timeout: nothing to flush yet
	flushed := d.Flush()
	if flushed {
		t.Fatal("expected Flush to return false before timeout")
	}
}

func TestEscapeDetector_ResetAfterDisconnect(t *testing.T) {
	d := NewEscapeDetector(500 * time.Millisecond)

	d.Feed(0x1B)
	d.Feed(0x1B) // Disconnect

	// After disconnect, state should be reset
	action := d.Feed(0x1B)
	if action != EscapeHeld {
		t.Fatalf("after reset: want EscapeHeld, got %v", action)
	}
}
