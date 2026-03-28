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
// AES-256-GCM symmetric key via HKDF-SHA256.
func (s *Session) Complete(peerPubBytes []byte) error {
	peerPub, err := ecdh.X25519().NewPublicKey(peerPubBytes)
	if err != nil {
		return err
	}
	shared, err := s.privKey.ECDH(peerPub)
	if err != nil {
		return err
	}

	// Derive a 32-byte AES-256 key from the shared secret.
	aesKey, err := hkdf.Key(sha256.New, shared, nil, "keytun-e2e-v1", 32)
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
