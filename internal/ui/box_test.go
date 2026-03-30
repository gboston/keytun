// ABOUTME: Tests for the box-drawing utility.
// ABOUTME: Verifies box structure and padding with known inputs.
package ui

import (
	"bytes"
	"strings"
	"testing"
)

func TestBoxSingleLine(t *testing.T) {
	origTTY := isTTY
	isTTY = false // disable ANSI for predictable output
	defer func() { isTTY = origTTY }()

	var buf bytes.Buffer
	Box(&buf, []string{"hello"}, []int{5})
	out := buf.String()

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (top, content, bottom), got %d: %q", len(lines), out)
	}

	// Top border
	if !strings.HasPrefix(lines[0], "┌") || !strings.HasSuffix(lines[0], "┐") {
		t.Errorf("top border malformed: %q", lines[0])
	}

	// Content line should contain "hello"
	if !strings.Contains(lines[1], "hello") {
		t.Errorf("content line missing text: %q", lines[1])
	}

	// Bottom border
	if !strings.HasPrefix(lines[2], "└") || !strings.HasSuffix(lines[2], "┘") {
		t.Errorf("bottom border malformed: %q", lines[2])
	}
}

func TestBoxMultipleLines(t *testing.T) {
	origTTY := isTTY
	isTTY = false
	defer func() { isTTY = origTTY }()

	var buf bytes.Buffer
	Box(&buf, []string{"short", "a longer line"}, []int{5, 13})
	out := buf.String()

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	// 2 content lines + top + bottom = 4
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d: %q", len(lines), out)
	}

	// Both content lines should have the pipe delimiter
	if !strings.Contains(lines[1], "│") || !strings.Contains(lines[2], "│") {
		t.Errorf("content lines missing borders: %q / %q", lines[1], lines[2])
	}
}

func TestBoxEmptyLines(t *testing.T) {
	origTTY := isTTY
	isTTY = false
	defer func() { isTTY = origTTY }()

	var buf bytes.Buffer
	Box(&buf, []string{}, []int{})
	out := buf.String()

	// Should still produce top and bottom borders
	if !strings.Contains(out, "┌") || !strings.Contains(out, "┘") {
		t.Errorf("empty box missing borders: %q", out)
	}
}
