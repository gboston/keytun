// ABOUTME: Tests for the animated terminal spinner.
// ABOUTME: Verifies start/stop lifecycle and output behavior.
package ui

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestSpinnerStartStop(t *testing.T) {
	var buf bytes.Buffer
	s := NewSpinner(&buf, "Loading...")

	// Let it run a few frames
	time.Sleep(250 * time.Millisecond)
	s.Stop()

	out := buf.String()
	// Should have written something while running
	if len(out) == 0 {
		t.Error("spinner produced no output")
	}
	// Should contain the message text
	if !strings.Contains(out, "Loading...") {
		t.Errorf("spinner output missing message: %q", out)
	}
	// Stop should have written the clear sequence
	if !strings.Contains(out, "\r\x1b[2K") {
		t.Errorf("spinner output missing clear sequence: %q", out)
	}
}

func TestSpinnerDoubleStop(t *testing.T) {
	var buf bytes.Buffer
	s := NewSpinner(&buf, "test")

	time.Sleep(100 * time.Millisecond)
	s.Stop()
	// Second stop should not panic (sync.Once)
	s.Stop()
}

func TestSpinnerImmediateStop(t *testing.T) {
	var buf bytes.Buffer
	s := NewSpinner(&buf, "quick")
	s.Stop()
	// Should not hang or panic
}
