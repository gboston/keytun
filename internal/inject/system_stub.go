//go:build !darwin

// ABOUTME: Stub SystemInjector for platforms that don't support OS-level keystroke injection.
// ABOUTME: Returns an error on NewSystem() — system mode is only available on macOS.
package inject

import "fmt"

// SystemInjector is a placeholder on non-macOS platforms.
type SystemInjector struct{}

// NewSystem returns an error because system mode is only supported on macOS.
func NewSystem() (*SystemInjector, error) {
	return nil, fmt.Errorf("system mode is only supported on macOS")
}

// Inject is not supported on this platform.
func (s *SystemInjector) Inject(data []byte) error {
	return fmt.Errorf("system mode is not supported on this platform")
}

// HasOutput returns false because system mode has no output stream.
func (s *SystemInjector) HasOutput() bool {
	return false
}

// Close is a no-op on unsupported platforms.
func (s *SystemInjector) Close() error {
	return nil
}
