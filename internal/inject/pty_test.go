// ABOUTME: Tests for the PTYInjector keystroke delivery implementation.
// ABOUTME: Validates PTY spawning, byte injection, output reading, and resizing.
package inject

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestPTYInjectorImplementsInjector(t *testing.T) {
	p, err := NewPTY()
	if err != nil {
		t.Fatalf("NewPTY: %v", err)
	}
	defer p.Close()

	var _ Injector = p
}

func TestPTYInjectorImplementsOutputReader(t *testing.T) {
	p, err := NewPTY()
	if err != nil {
		t.Fatalf("NewPTY: %v", err)
	}
	defer p.Close()

	var _ OutputReader = p
}

func TestPTYInjectorHasOutput(t *testing.T) {
	p, err := NewPTY()
	if err != nil {
		t.Fatalf("NewPTY: %v", err)
	}
	defer p.Close()

	if !p.HasOutput() {
		t.Error("PTYInjector.HasOutput() should return true")
	}
}

func TestPTYInjectorInjectAndRead(t *testing.T) {
	p, err := NewPTY()
	if err != nil {
		t.Fatalf("NewPTY: %v", err)
	}
	defer p.Close()

	// Inject an echo command
	if err := p.Inject([]byte("echo injtest\n")); err != nil {
		t.Fatalf("Inject: %v", err)
	}

	// Read output from the PTY fd until we see the result
	fd := p.OutputFd()
	buf := make([]byte, 4096)
	deadline := time.Now().Add(5 * time.Second)
	var output strings.Builder
	for time.Now().Before(deadline) {
		fd.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, err := fd.Read(buf)
		if n > 0 {
			output.Write(buf[:n])
		}
		if strings.Contains(output.String(), "injtest") {
			return // success
		}
		if err != nil && !os.IsTimeout(err) {
			break
		}
	}
	t.Errorf("expected PTY output to contain 'injtest', got: %q", output.String())
}

func TestPTYInjectorResizePTY(t *testing.T) {
	p, err := NewPTY()
	if err != nil {
		t.Fatalf("NewPTY: %v", err)
	}
	defer p.Close()

	// Should not error on a valid resize
	if err := p.ResizePTY(24, 80); err != nil {
		t.Errorf("ResizePTY: %v", err)
	}
}

func TestPTYInjectorClose(t *testing.T) {
	p, err := NewPTY()
	if err != nil {
		t.Fatalf("NewPTY: %v", err)
	}

	if err := p.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}

	// Inject after close should fail
	if err := p.Inject([]byte("hello")); err == nil {
		t.Error("expected Inject after Close to return error")
	}
}
