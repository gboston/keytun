//go:build darwin

// ABOUTME: macOS system-level keystroke injector using CGEventPost.
// ABOUTME: Posts key events to the focused application via the Accessibility API.
package inject

import "fmt"

// SystemInjector injects keystrokes at the OS level on macOS using CoreGraphics.
type SystemInjector struct{}

// NewSystem creates a SystemInjector. Requires Accessibility permissions.
func NewSystem() (*SystemInjector, error) {
	// TODO: implement CGEventPost-based injection in Phase 2
	return nil, fmt.Errorf("system mode is not yet implemented")
}

// Inject delivers raw keystroke bytes to the focused application via CGEventPost.
func (s *SystemInjector) Inject(data []byte) error {
	return fmt.Errorf("system mode is not yet implemented")
}

// HasOutput returns false because system mode has no output stream.
func (s *SystemInjector) HasOutput() bool {
	return false
}

// Close is a no-op for the system injector.
func (s *SystemInjector) Close() error {
	return nil
}
