// ABOUTME: Tests for the end-to-end encryption session used between host and client.
// ABOUTME: Verifies key exchange, encrypt/decrypt round-trips, and failure cases.
package crypto

import (
	"bytes"
	"testing"
)

func TestNewSession(t *testing.T) {
	s, err := NewSession()
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	pub := s.PublicKey()
	if len(pub) != 32 {
		t.Fatalf("expected 32-byte public key, got %d bytes", len(pub))
	}
}

func TestKeyExchangeSymmetry(t *testing.T) {
	// Two sessions exchange public keys and should derive the same shared secret.
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

	// Verify by encrypting on one side and decrypting on the other.
	plaintext := []byte("hello keytun")
	ciphertext, err := alice.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("alice.Encrypt: %v", err)
	}
	decrypted, err := bob.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("bob.Decrypt: %v", err)
	}
	if !bytes.Equal(plaintext, decrypted) {
		t.Fatalf("expected %q, got %q", plaintext, decrypted)
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	alice, bob := completedPair(t)

	cases := [][]byte{
		[]byte("a"),
		[]byte("hello world"),
		{0x00, 0x01, 0x1b, 0xff},  // binary data including escape and null
		bytes.Repeat([]byte("x"), 4096), // larger payload
	}

	for _, plaintext := range cases {
		ct, err := alice.Encrypt(plaintext)
		if err != nil {
			t.Fatalf("Encrypt: %v", err)
		}
		pt, err := bob.Decrypt(ct)
		if err != nil {
			t.Fatalf("Decrypt: %v", err)
		}
		if !bytes.Equal(plaintext, pt) {
			t.Fatalf("round-trip mismatch for input len %d", len(plaintext))
		}
	}
}

func TestDecryptWrongKey(t *testing.T) {
	alice, _ := completedPair(t)
	eve, _ := completedPair(t) // independent pair, different keys

	ct, err := alice.Encrypt([]byte("secret"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	_, err = eve.Decrypt(ct)
	if err == nil {
		t.Fatal("expected decryption to fail with wrong key")
	}
}

func TestNonceUniqueness(t *testing.T) {
	alice, _ := completedPair(t)

	plaintext := []byte("same input")
	ct1, err := alice.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt 1: %v", err)
	}
	ct2, err := alice.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt 2: %v", err)
	}
	if bytes.Equal(ct1, ct2) {
		t.Fatal("two encryptions of the same plaintext produced identical ciphertext")
	}
}

func TestTamperedCiphertext(t *testing.T) {
	alice, bob := completedPair(t)

	ct, err := alice.Encrypt([]byte("important data"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Flip a bit in the ciphertext (after the 12-byte nonce)
	if len(ct) > 12 {
		ct[12] ^= 0x01
	}

	_, err = bob.Decrypt(ct)
	if err == nil {
		t.Fatal("expected decryption to fail on tampered ciphertext")
	}
}

func TestEncryptBeforeComplete(t *testing.T) {
	s, err := NewSession()
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	_, err = s.Encrypt([]byte("data"))
	if err == nil {
		t.Fatal("expected error when encrypting before key exchange")
	}
}

func TestDecryptBeforeComplete(t *testing.T) {
	s, err := NewSession()
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	_, err = s.Decrypt([]byte("data"))
	if err == nil {
		t.Fatal("expected error when decrypting before key exchange")
	}
}

// completedPair creates two sessions that have completed key exchange.
func completedPair(t *testing.T) (*Session, *Session) {
	t.Helper()
	a, err := NewSession()
	if err != nil {
		t.Fatalf("NewSession a: %v", err)
	}
	b, err := NewSession()
	if err != nil {
		t.Fatalf("NewSession b: %v", err)
	}
	if err := a.Complete(b.PublicKey()); err != nil {
		t.Fatalf("a.Complete: %v", err)
	}
	if err := b.Complete(a.PublicKey()); err != nil {
		t.Fatalf("b.Complete: %v", err)
	}
	return a, b
}
