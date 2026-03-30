// ABOUTME: Concurrency and distribution tests for session code generation.
// ABOUTME: Verifies thread-safety and reasonable distribution of generated codes.
package session

import (
	"regexp"
	"sync"
	"testing"
)

func TestGenerateConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	const goroutines = 100
	wg.Add(goroutines)

	codes := make([]string, goroutines)
	pattern := regexp.MustCompile(`^[a-z]+-[a-z]+-\d{2}$`)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			codes[idx] = Generate()
		}(i)
	}
	wg.Wait()

	for i, code := range codes {
		if !pattern.MatchString(code) {
			t.Errorf("goroutine %d: invalid format %q", i, code)
		}
	}
}

func TestGenerateDistribution(t *testing.T) {
	// Generate many codes and check that we see variety in adjectives and nouns
	adjSeen := make(map[string]int)
	nounSeen := make(map[string]int)

	const n = 5000
	for i := 0; i < n; i++ {
		code := Generate()
		// Parse: adj-noun-NN
		parts := regexp.MustCompile(`^([a-z]+)-([a-z]+)-\d{2}$`).FindStringSubmatch(code)
		if len(parts) != 3 {
			t.Fatalf("failed to parse code %q", code)
		}
		adjSeen[parts[1]]++
		nounSeen[parts[2]]++
	}

	// With 155 adjectives and 5000 samples, we should see at least 50 distinct adjectives
	if len(adjSeen) < 50 {
		t.Errorf("poor adjective distribution: only %d unique adjectives in %d samples", len(adjSeen), n)
	}

	// With 107 nouns and 5000 samples, we should see at least 50 distinct nouns
	if len(nounSeen) < 50 {
		t.Errorf("poor noun distribution: only %d unique nouns in %d samples", len(nounSeen), n)
	}
}

func TestNumberRange(t *testing.T) {
	// Verify the numeric suffix is always in [10, 99]
	for i := 0; i < 1000; i++ {
		code := Generate()
		parts := regexp.MustCompile(`-(\d+)$`).FindStringSubmatch(code)
		if len(parts) != 2 {
			t.Fatalf("no numeric suffix in %q", code)
		}
		num := 0
		for _, c := range parts[1] {
			num = num*10 + int(c-'0')
		}
		if num < 10 || num > 99 {
			t.Errorf("number %d out of range [10, 99] in code %q", num, code)
		}
	}
}
