// ABOUTME: Tests for wordlist-based session code generation.
// ABOUTME: Validates format, randomness, and collision resistance.
package session

import (
	"regexp"
	"testing"
)

var codePattern = regexp.MustCompile(`^[a-z]+-[a-z]+-\d{2}$`)

func TestGenerateFormat(t *testing.T) {
	code := Generate()
	if !codePattern.MatchString(code) {
		t.Errorf("code %q does not match expected format adj-noun-NN", code)
	}
}

func TestGenerateUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	n := 100
	for i := 0; i < n; i++ {
		code := Generate()
		if seen[code] {
			t.Errorf("duplicate code %q after %d generations", code, i)
		}
		seen[code] = true
	}
}

func TestGenerateMultipleCodes(t *testing.T) {
	// Generate several codes and verify they all match the pattern
	for i := 0; i < 50; i++ {
		code := Generate()
		if !codePattern.MatchString(code) {
			t.Errorf("code %q does not match expected format", code)
		}
	}
}

func TestWordlistsNotEmpty(t *testing.T) {
	if len(adjectives) == 0 {
		t.Error("adjectives wordlist is empty")
	}
	if len(nouns) == 0 {
		t.Error("nouns wordlist is empty")
	}
}
