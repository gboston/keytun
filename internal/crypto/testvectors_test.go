// ABOUTME: Generates test vectors for cross-language crypto compatibility testing.
// ABOUTME: Outputs base64 values that the JS web client tests use to verify HKDF + AES-GCM match.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"testing"
)

func TestPrintCrossCompatVectors(t *testing.T) {
	// Fixed private key bytes (32 bytes each) for reproducibility.
	alicePrivBytes := []byte{
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
		0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20,
	}
	bobPrivBytes := []byte{
		0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28,
		0x29, 0x2a, 0x2b, 0x2c, 0x2d, 0x2e, 0x2f, 0x30,
		0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38,
		0x39, 0x3a, 0x3b, 0x3c, 0x3d, 0x3e, 0x3f, 0x40,
	}

	alicePriv, err := ecdh.X25519().NewPrivateKey(alicePrivBytes)
	if err != nil {
		t.Fatal(err)
	}
	bobPriv, err := ecdh.X25519().NewPrivateKey(bobPrivBytes)
	if err != nil {
		t.Fatal(err)
	}

	alicePub := alicePriv.PublicKey().Bytes()
	bobPub := bobPriv.PublicKey().Bytes()

	// Alice performs ECDH with Bob's public key
	shared, err := alicePriv.ECDH(bobPriv.PublicKey())
	if err != nil {
		t.Fatal(err)
	}

	// Derive AES key via HKDF (same as Session.Complete)
	aesKeyBytes, err := hkdf.Key(sha256.New, shared, nil, "keytun-e2e-v1", 32)
	if err != nil {
		t.Fatal(err)
	}

	// Encrypt a known plaintext with a known nonce
	block, _ := aes.NewCipher(aesKeyBytes)
	gcm, _ := cipher.NewGCM(block)
	nonce := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b}
	plaintext := []byte("hello keytun")
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)

	b64 := base64.StdEncoding.EncodeToString
	fmt.Printf("VECTOR_ALICE_PUB=%s\n", b64(alicePub))
	fmt.Printf("VECTOR_BOB_PUB=%s\n", b64(bobPub))
	fmt.Printf("VECTOR_AES_KEY=%s\n", b64(aesKeyBytes))
	fmt.Printf("VECTOR_CIPHERTEXT=%s\n", b64(ciphertext))
	fmt.Printf("VECTOR_PLAINTEXT=%s\n", b64(plaintext))
}
