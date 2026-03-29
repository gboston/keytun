// ABOUTME: End-to-end encryption session using X25519 ECDH key exchange and AES-256-GCM.
// ABOUTME: Provides the Session type that host and client use to encrypt all data flowing through the relay.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
)

// Session manages an ephemeral X25519 key pair and derives a shared
// AES-256-GCM key after completing ECDH with the peer's public key.
type Session struct {
	privKey *ecdh.PrivateKey
	aead    cipher.AEAD
}

// NewSession generates an ephemeral X25519 key pair.
func NewSession() (*Session, error) {
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &Session{privKey: priv}, nil
}

// PublicKey returns the 32-byte X25519 public key to send to the peer.
func (s *Session) PublicKey() []byte {
	return s.privKey.PublicKey().Bytes()
}

// Complete performs ECDH with the peer's public key and derives the
// AES-256-GCM symmetric key via HKDF-SHA256. An optional password can be
// provided to mix into the HKDF salt — both sides must use the same password
// for decryption to succeed. An empty string is equivalent to no password.
func (s *Session) Complete(peerPubBytes []byte, password ...string) error {
	peerPub, err := ecdh.X25519().NewPublicKey(peerPubBytes)
	if err != nil {
		return err
	}
	shared, err := s.privKey.ECDH(peerPub)
	if err != nil {
		return err
	}

	// When a password is provided, mix it into the HKDF salt so that both
	// sides must agree on the password to derive the same AES key. An empty
	// password produces a nil salt, preserving backward compatibility.
	var salt []byte
	if len(password) > 0 && password[0] != "" {
		salt = []byte("keytun-password:" + password[0])
	}

	// Derive a 32-byte AES-256 key from the shared secret.
	aesKey, err := hkdf.Key(sha256.New, shared, salt, "keytun-e2e-v1", 32)
	if err != nil {
		return err
	}

	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return err
	}
	s.aead, err = cipher.NewGCM(block)
	if err != nil {
		return err
	}
	return nil
}

// IsReady returns true if the key exchange has been completed and encryption is available.
func (s *Session) IsReady() bool {
	return s.aead != nil
}

// Encrypt encrypts plaintext using AES-256-GCM. Returns nonce || ciphertext.
func (s *Session) Encrypt(plaintext []byte) ([]byte, error) {
	if s.aead == nil {
		return nil, errors.New("crypto: key exchange not completed")
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return s.aead.Seal(nonce, nonce, plaintext, nil), nil
}

// verifyChallenge is the known plaintext used to confirm both sides
// derived the same key (i.e. passwords match).
var verifyChallenge = []byte("keytun-verify-v1")

// VerifyToken encrypts the known challenge so the peer can confirm key agreement.
func (s *Session) VerifyToken() ([]byte, error) {
	return s.Encrypt(verifyChallenge)
}

// CheckVerify decrypts a verify token and checks it matches the expected challenge.
// Returns an error if the token is invalid (e.g. password mismatch).
func (s *Session) CheckVerify(token []byte) error {
	plaintext, err := s.Decrypt(token)
	if err != nil {
		return fmt.Errorf("password mismatch or invalid verify token")
	}
	if string(plaintext) != string(verifyChallenge) {
		return fmt.Errorf("verify challenge mismatch")
	}
	return nil
}

// Decrypt decrypts ciphertext produced by Encrypt. Expects nonce || ciphertext.
func (s *Session) Decrypt(ciphertext []byte) ([]byte, error) {
	if s.aead == nil {
		return nil, errors.New("crypto: key exchange not completed")
	}
	nonceSize := s.aead.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("crypto: ciphertext too short")
	}
	nonce := ciphertext[:nonceSize]
	return s.aead.Open(nil, nonce, ciphertext[nonceSize:], nil)
}
