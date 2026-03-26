// ABOUTME: Defines the Injector interface for keystroke delivery strategies.
// ABOUTME: Implementations deliver raw bytes to either a PTY or the OS input system.
package inject

import "os"

// Injector delivers keystroke bytes to a target input sink.
type Injector interface {
	// Inject delivers raw keystroke bytes to the target.
	Inject(data []byte) error

	// HasOutput returns true if this injector produces readable output (e.g. PTY mode).
	HasOutput() bool

	// Close releases resources held by this injector.
	Close() error
}

// OutputReader is implemented by injectors that produce readable output.
// The host uses this to read output and forward it to the relay.
type OutputReader interface {
	// OutputFd returns a readable file descriptor for the injector's output stream.
	OutputFd() *os.File
}
