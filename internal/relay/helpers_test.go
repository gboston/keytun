// ABOUTME: Tests for relay helper functions not covered by the main relay tests.
// ABOUTME: Covers IP extraction, origin checks, session redaction, and limiter cleanup.
package relay

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRealIP_CFHeader(t *testing.T) {
	req, _ := http.NewRequest("GET", "/ws", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("CF-Connecting-IP", "203.0.113.50")

	got := realIP(req)
	if got != "203.0.113.50" {
		t.Errorf("expected CF-Connecting-IP, got %q", got)
	}
}

func TestRealIP_Fallback(t *testing.T) {
	req, _ := http.NewRequest("GET", "/ws", nil)
	req.RemoteAddr = "192.168.1.1:5555"

	got := realIP(req)
	if got != "192.168.1.1" {
		t.Errorf("expected parsed remote addr, got %q", got)
	}
}

func TestRealIP_NoPort(t *testing.T) {
	req, _ := http.NewRequest("GET", "/ws", nil)
	req.RemoteAddr = "192.168.1.1" // no port

	got := realIP(req)
	// SplitHostPort fails, ip="" => falls back to full RemoteAddr
	if got != "192.168.1.1" {
		t.Errorf("expected raw remote addr, got %q", got)
	}
}

func TestCheckOrigin_NoOrigin(t *testing.T) {
	req, _ := http.NewRequest("GET", "/ws", nil)
	if !checkOrigin(req) {
		t.Error("expected no-origin (CLI) to be allowed")
	}
}

func TestCheckOrigin_Localhost(t *testing.T) {
	tests := []string{
		"http://localhost:4321",
		"http://localhost",
		"http://127.0.0.1:8080",
	}
	for _, origin := range tests {
		req, _ := http.NewRequest("GET", "/ws", nil)
		req.Header.Set("Origin", origin)
		if !checkOrigin(req) {
			t.Errorf("expected %q to be allowed", origin)
		}
	}
}

func TestCheckOrigin_AllowedDomains(t *testing.T) {
	tests := []string{
		"https://keytun.com",
		"https://www.keytun.com",
	}
	for _, origin := range tests {
		req, _ := http.NewRequest("GET", "/ws", nil)
		req.Header.Set("Origin", origin)
		if !checkOrigin(req) {
			t.Errorf("expected %q to be allowed", origin)
		}
	}
}

func TestCheckOrigin_Rejected(t *testing.T) {
	tests := []string{
		"https://evil.com",
		"https://keytun.com.evil.com",
		"http://keytun.com", // http, not https
		"https://notkeytun.com",
	}
	for _, origin := range tests {
		req, _ := http.NewRequest("GET", "/ws", nil)
		req.Header.Set("Origin", origin)
		if checkOrigin(req) {
			t.Errorf("expected %q to be rejected", origin)
		}
	}
}

func TestRedactSession(t *testing.T) {
	code := "keen-fox-42"
	redacted := redactSession(code)

	// Should be a hex string, 8 chars (4 bytes)
	if len(redacted) != 8 {
		t.Errorf("expected 8-char hex string, got %q", redacted)
	}

	// Same input should produce same output
	if redactSession(code) != redacted {
		t.Error("redactSession not deterministic")
	}

	// Different input should produce different output
	if redactSession("bold-elk-77") == redacted {
		t.Error("different codes produced same redaction")
	}

	// Should not contain the original code
	if strings.Contains(redacted, "keen") || strings.Contains(redacted, "fox") {
		t.Error("redacted output leaks session code")
	}
}

func TestSweepStaleLimiters(t *testing.T) {
	r := &Relay{
		sessions:  make(map[string]*session),
		limiters:  make(map[string]*ipEntry),
		joinBurst: 10,
	}

	// Add a fresh limiter
	r.getLimiter("1.2.3.4")
	// Add a stale limiter by backdating
	r.limitersMu.Lock()
	r.limiters["5.6.7.8"] = &ipEntry{
		limiter:  r.limiters["1.2.3.4"].limiter,
		lastSeen: time.Now().Add(-10 * time.Minute),
	}
	r.limitersMu.Unlock()

	r.sweepStaleLimiters()

	r.limitersMu.Lock()
	defer r.limitersMu.Unlock()

	if _, ok := r.limiters["1.2.3.4"]; !ok {
		t.Error("fresh limiter was incorrectly swept")
	}
	if _, ok := r.limiters["5.6.7.8"]; ok {
		t.Error("stale limiter was not swept")
	}
}

func TestGetLimiter_CreateAndReuse(t *testing.T) {
	r := &Relay{
		sessions:  make(map[string]*session),
		limiters:  make(map[string]*ipEntry),
		joinBurst: 10,
	}

	l1 := r.getLimiter("10.0.0.1")
	l2 := r.getLimiter("10.0.0.1")

	if l1 != l2 {
		t.Error("expected same limiter for same IP")
	}

	l3 := r.getLimiter("10.0.0.2")
	if l1 == l3 {
		t.Error("expected different limiter for different IP")
	}
}
