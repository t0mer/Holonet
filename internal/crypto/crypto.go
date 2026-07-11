// Package crypto seals secrets at rest with AES-256-GCM.
//
// Every credential HoloNet persists (SNMP community strings, SNMPv3 auth/priv
// passwords, channel tokens, gateway credentials) is sealed through a Sealer
// keyed from a master key supplied at boot. No plaintext secret is ever written
// to the database.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// Sealer encrypts and decrypts secrets with an AES-256-GCM key derived from the
// master key. It is safe for concurrent use.
type Sealer struct {
	gcm cipher.AEAD
}

// New derives a Sealer from the master key. The key is hashed to a 32-byte
// AES-256 key, so any non-empty string is accepted, but operators should supply
// a high-entropy value (e.g. `openssl rand -base64 32`). An empty key is
// rejected — sealing must never fall back to a zero key.
func New(masterKey string) (*Sealer, error) {
	if masterKey == "" {
		return nil, errors.New("crypto: master key must not be empty")
	}
	sum := sha256.Sum256([]byte(masterKey))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: new gcm: %w", err)
	}
	return &Sealer{gcm: gcm}, nil
}

// Seal encrypts plaintext and returns base64(nonce || ciphertext).
func (s *Sealer) Seal(plaintext []byte) (string, error) {
	nonce := make([]byte, s.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("crypto: read nonce: %w", err)
	}
	sealed := s.gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Open reverses Seal, authenticating and decrypting the value.
func (s *Sealer) Open(sealed string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil {
		return nil, fmt.Errorf("crypto: decode base64: %w", err)
	}
	ns := s.gcm.NonceSize()
	if len(raw) < ns {
		return nil, errors.New("crypto: sealed value too short")
	}
	nonce, ciphertext := raw[:ns], raw[ns:]
	plaintext, err := s.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: open: %w", err)
	}
	return plaintext, nil
}

// SealString is Seal for string values.
func (s *Sealer) SealString(plaintext string) (string, error) {
	return s.Seal([]byte(plaintext))
}

// OpenString is Open returning a string.
func (s *Sealer) OpenString(sealed string) (string, error) {
	b, err := s.Open(sealed)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
