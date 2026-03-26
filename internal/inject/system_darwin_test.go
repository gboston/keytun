//go:build darwin

// ABOUTME: Tests for the macOS SystemInjector.
// ABOUTME: Validates CGEventPost-based keystroke injection (requires Accessibility permissions).
package inject

import (
	"testing"
)

func TestSystemInjectorImplementsInjector(t *testing.T) {
	var _ Injector = &SystemInjector{}
}

func TestSystemInjectorHasNoOutput(t *testing.T) {
	s := &SystemInjector{}
	if s.HasOutput() {
		t.Error("SystemInjector.HasOutput() should return false")
	}
}

func TestSystemInjectorClose(t *testing.T) {
	s := &SystemInjector{}
	if err := s.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestNewSystemRequiresAccessibility(t *testing.T) {
	// This test verifies NewSystem either succeeds (if Accessibility is granted)
	// or returns a clear error message about permissions.
	s, err := NewSystem()
	if err != nil {
		t.Logf("NewSystem returned error (likely no Accessibility permission): %v", err)
		return
	}
	defer s.Close()
	t.Log("NewSystem succeeded — Accessibility permissions are granted")
}

func TestSystemInjectorInject(t *testing.T) {
	s, err := NewSystem()
	if err != nil {
		t.Skipf("skipping: system injector not available: %v", err)
	}
	defer s.Close()

	// Inject a harmless keypress — we can't easily verify it landed in the
	// focused app, but we can verify it doesn't error or panic.
	if err := s.Inject([]byte("a")); err != nil {
		t.Errorf("Inject: %v", err)
	}
}

func TestNewSystemWithTargetFindsRunningApp(t *testing.T) {
	// Finder is always running on macOS
	s, err := NewSystemWithTarget("Finder")
	if err != nil {
		t.Skipf("skipping: targeted system injector not available: %v", err)
	}
	defer s.Close()

	if s.targetPID == 0 {
		t.Error("expected non-zero target PID for Finder")
	}
}

func TestNewSystemWithTargetRejectsUnknownApp(t *testing.T) {
	_, err := NewSystemWithTarget("ThisAppDefinitelyDoesNotExist12345")
	if err == nil {
		t.Error("expected error for unknown app name")
	}
}

func TestSystemInjectorTargetedInject(t *testing.T) {
	// Finder is always running — inject a harmless key event to it
	s, err := NewSystemWithTarget("Finder")
	if err != nil {
		t.Skipf("skipping: targeted system injector not available: %v", err)
	}
	defer s.Close()

	// Should not error or panic
	if err := s.Inject([]byte("a")); err != nil {
		t.Errorf("targeted Inject: %v", err)
	}
}
