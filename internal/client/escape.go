// ABOUTME: Double-escape detector for client disconnect.
// ABOUTME: Tracks Escape key presses and signals disconnect when two arrive within a timeout.
package client

import (
	"time"
)

// EscapeAction represents what the caller should do after feeding a byte.
type EscapeAction int

const (
	// PassThrough means the byte should be sent normally.
	PassThrough EscapeAction = iota
	// EscapeHeld means an Escape was received and is being held pending a second one.
	EscapeHeld
	// Disconnect means a double-escape was detected.
	Disconnect
)

// EscapeDetector detects double-escape sequences for disconnecting.
type EscapeDetector struct {
	timeout        time.Duration
	escapeTime     time.Time
	pending        bool
	hadPending     bool
}

// NewEscapeDetector creates a detector with the given timeout between escapes.
func NewEscapeDetector(timeout time.Duration) *EscapeDetector {
	return &EscapeDetector{timeout: timeout}
}

// Feed processes a single byte and returns the action the caller should take.
func (d *EscapeDetector) Feed(b byte) EscapeAction {
	d.hadPending = false

	if b == 0x1B {
		if d.pending && time.Since(d.escapeTime) < d.timeout {
			d.pending = false
			return Disconnect
		}
		d.pending = true
		d.escapeTime = time.Now()
		return EscapeHeld
	}

	// Non-escape byte
	if d.pending {
		d.pending = false
		d.hadPending = true
	}
	return PassThrough
}

// Flush checks if a held Escape has timed out and should be sent.
// Returns true if there was a pending escape that has now expired.
func (d *EscapeDetector) Flush() bool {
	if d.pending && time.Since(d.escapeTime) >= d.timeout {
		d.pending = false
		return true
	}
	return false
}

// HadPendingEscape returns true if the last Feed call flushed a pending escape.
func (d *EscapeDetector) HadPendingEscape() bool {
	return d.hadPending
}

// Pending returns true if an escape is currently held.
func (d *EscapeDetector) Pending() bool {
	return d.pending
}
