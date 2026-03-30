// ABOUTME: Tests for pure utility functions in the join command.
// ABOUTME: Covers backoff calculation, error classification helpers.
package cmd

import (
	"fmt"
	"testing"
	"time"
)

func TestBackoffDelay_Exponential(t *testing.T) {
	initial := 500 * time.Millisecond
	max := 15 * time.Second
	mult := 2.0
	jitter := 0.0 // no jitter for deterministic tests

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{1, 500 * time.Millisecond},  // 500ms * 2^0
		{2, 1000 * time.Millisecond}, // 500ms * 2^1
		{3, 2000 * time.Millisecond}, // 500ms * 2^2
		{4, 4000 * time.Millisecond}, // 500ms * 2^3
		{5, 8000 * time.Millisecond}, // 500ms * 2^4
	}

	for _, tc := range tests {
		got := backoffDelay(tc.attempt, initial, max, mult, jitter)
		if got != tc.expected {
			t.Errorf("attempt %d: got %v, want %v", tc.attempt, got, tc.expected)
		}
	}
}

func TestBackoffDelay_CapsAtMax(t *testing.T) {
	initial := 500 * time.Millisecond
	max := 15 * time.Second
	mult := 2.0
	jitter := 0.0

	// attempt 6 => 500ms * 2^5 = 16s, should cap at 15s
	got := backoffDelay(6, initial, max, mult, jitter)
	if got != max {
		t.Errorf("expected max %v, got %v", max, got)
	}

	// Very large attempt should also cap
	got = backoffDelay(100, initial, max, mult, jitter)
	if got != max {
		t.Errorf("large attempt: expected max %v, got %v", max, got)
	}
}

func TestBackoffDelay_JitterBounds(t *testing.T) {
	initial := 1 * time.Second
	max := 30 * time.Second
	mult := 2.0
	jitter := 0.25

	for i := 0; i < 100; i++ {
		got := backoffDelay(1, initial, max, mult, jitter)
		// With 25% jitter on 1s base, result should be in [750ms, 1250ms]
		if got < 750*time.Millisecond || got > 1250*time.Millisecond {
			t.Errorf("iteration %d: delay %v out of expected jitter range [750ms, 1250ms]", i, got)
		}
	}
}

func TestIsSessionGone(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{fmt.Errorf("session not found"), true},
		{fmt.Errorf("no such session"), true},
		{fmt.Errorf("relay: session not found for code abc"), true},
		{fmt.Errorf("connection refused"), false},
		{fmt.Errorf("timeout"), false},
		{fmt.Errorf(""), false},
	}

	for _, tc := range tests {
		got := isSessionGone(tc.err)
		if got != tc.want {
			t.Errorf("isSessionGone(%q) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func TestIsPasswordError(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{fmt.Errorf("wrong session password"), true},
		{fmt.Errorf("relay: wrong session password"), true},
		{fmt.Errorf("session not found"), false},
		{fmt.Errorf("connection refused"), false},
	}

	for _, tc := range tests {
		got := isPasswordError(tc.err)
		if got != tc.want {
			t.Errorf("isPasswordError(%q) = %v, want %v", tc.err, got, tc.want)
		}
	}
}
