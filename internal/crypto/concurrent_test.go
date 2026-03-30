// ABOUTME: Concurrency tests for the crypto Session.
// ABOUTME: Verifies thread-safety of Encrypt/Decrypt under parallel access.
package crypto

import (
	"bytes"
	"sync"
	"testing"
)

func setupPair(t *testing.T) (*Session, *Session) {
	t.Helper()
	alice, err := NewSession()
	if err != nil {
		t.Fatalf("NewSession alice: %v", err)
	}
	bob, err := NewSession()
	if err != nil {
		t.Fatalf("NewSession bob: %v", err)
	}
	if err := alice.Complete(bob.PublicKey()); err != nil {
		t.Fatalf("alice.Complete: %v", err)
	}
	if err := bob.Complete(alice.PublicKey()); err != nil {
		t.Fatalf("bob.Complete: %v", err)
	}
	return alice, bob
}

func TestConcurrentEncrypt(t *testing.T) {
	alice, bob := setupPair(t)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	ciphertexts := make([][]byte, goroutines)
	errs := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			ct, err := alice.Encrypt([]byte("hello"))
			ciphertexts[idx] = ct
			errs[idx] = err
		}(i)
	}
	wg.Wait()

	// All should succeed
	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: encrypt error: %v", i, err)
		}
	}

	// All ciphertexts should be unique (unique nonces)
	seen := make(map[string]bool)
	for i, ct := range ciphertexts {
		key := string(ct)
		if seen[key] {
			t.Errorf("goroutine %d: duplicate ciphertext (nonce reuse!)", i)
		}
		seen[key] = true
	}

	// All should decrypt correctly
	for i, ct := range ciphertexts {
		pt, err := bob.Decrypt(ct)
		if err != nil {
			t.Errorf("goroutine %d: decrypt error: %v", i, err)
		}
		if !bytes.Equal(pt, []byte("hello")) {
			t.Errorf("goroutine %d: got %q, want %q", i, pt, "hello")
		}
	}
}

func TestConcurrentEncryptDecrypt(t *testing.T) {
	alice, bob := setupPair(t)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Half encrypt with alice, half encrypt with bob
	aliceCTs := make([][]byte, goroutines)
	bobCTs := make([][]byte, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			ct, err := alice.Encrypt([]byte("from-alice"))
			if err != nil {
				t.Errorf("alice encrypt %d: %v", idx, err)
			}
			aliceCTs[idx] = ct
		}(i)

		go func(idx int) {
			defer wg.Done()
			ct, err := bob.Encrypt([]byte("from-bob"))
			if err != nil {
				t.Errorf("bob encrypt %d: %v", idx, err)
			}
			bobCTs[idx] = ct
		}(i)
	}
	wg.Wait()

	// Cross-decrypt
	for i := 0; i < goroutines; i++ {
		if aliceCTs[i] != nil {
			pt, err := bob.Decrypt(aliceCTs[i])
			if err != nil {
				t.Errorf("bob decrypt alice[%d]: %v", i, err)
			} else if !bytes.Equal(pt, []byte("from-alice")) {
				t.Errorf("bob decrypt alice[%d]: got %q", i, pt)
			}
		}
		if bobCTs[i] != nil {
			pt, err := alice.Decrypt(bobCTs[i])
			if err != nil {
				t.Errorf("alice decrypt bob[%d]: %v", i, err)
			} else if !bytes.Equal(pt, []byte("from-bob")) {
				t.Errorf("alice decrypt bob[%d]: got %q", i, pt)
			}
		}
	}
}
