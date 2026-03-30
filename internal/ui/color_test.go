// ABOUTME: Tests for ANSI color formatting functions.
// ABOUTME: Verifies correct escape codes and plain-text fallback.
package ui

import (
	"strings"
	"testing"
)

func TestWrapWithTTY(t *testing.T) {
	// Force TTY mode for these tests
	origTTY := isTTY
	isTTY = true
	defer func() { isTTY = origTTY }()

	tests := []struct {
		name string
		fn   func(string) string
		code string
	}{
		{"Bold", Bold, "\x1b[1m"},
		{"Dim", Dim, "\x1b[2m"},
		{"Green", Green, "\x1b[32m"},
		{"Yellow", Yellow, "\x1b[33m"},
		{"Red", Red, "\x1b[31m"},
		{"Cyan", Cyan, "\x1b[36m"},
		{"BoldRed", BoldRed, "\x1b[1;31m"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.fn("hello")
			if !strings.HasPrefix(got, tc.code) {
				t.Errorf("%s: expected prefix %q, got %q", tc.name, tc.code, got)
			}
			if !strings.HasSuffix(got, "\x1b[0m") {
				t.Errorf("%s: expected reset suffix, got %q", tc.name, got)
			}
			if !strings.Contains(got, "hello") {
				t.Errorf("%s: missing content", tc.name)
			}
		})
	}
}

func TestWrapWithoutTTY(t *testing.T) {
	origTTY := isTTY
	isTTY = false
	defer func() { isTTY = origTTY }()

	funcs := []struct {
		name string
		fn   func(string) string
	}{
		{"Bold", Bold},
		{"Dim", Dim},
		{"Green", Green},
		{"Yellow", Yellow},
		{"Red", Red},
		{"Cyan", Cyan},
		{"BoldRed", BoldRed},
	}

	for _, tc := range funcs {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.fn("hello")
			if got != "hello" {
				t.Errorf("%s: non-TTY should return plain string, got %q", tc.name, got)
			}
		})
	}
}

func TestFormatFunctions(t *testing.T) {
	origTTY := isTTY
	isTTY = true
	defer func() { isTTY = origTTY }()

	got := Greenf("count: %d", 42)
	if !strings.Contains(got, "count: 42") {
		t.Errorf("Greenf: expected formatted content, got %q", got)
	}

	got = Yellowf("warn: %s", "oops")
	if !strings.Contains(got, "warn: oops") {
		t.Errorf("Yellowf: expected formatted content, got %q", got)
	}

	got = Redf("err: %v", "fail")
	if !strings.Contains(got, "err: fail") {
		t.Errorf("Redf: expected formatted content, got %q", got)
	}
}

func TestEmptyString(t *testing.T) {
	origTTY := isTTY
	isTTY = true
	defer func() { isTTY = origTTY }()

	got := Bold("")
	if got != "\x1b[1m\x1b[0m" {
		t.Errorf("Bold empty: got %q", got)
	}
}
