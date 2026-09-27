// Package secretbox encrypts small secrets at rest with AES-256-GCM.
//
// WHY THIS EXISTS: the per-device SSH transport needs each device's own
// credentials, which means storing them. Storing a router password in plaintext
// in Postgres would be a worse bug than the one this work fixes, so the cipher
// is the load-bearing part, not an add-on.
//
// DESIGN RULES
//   - AES-256-GCM with a random 96-bit nonce, stored as nonce||ciphertext in
//     base64. GCM authenticates, so a tampered or truncated value fails to open
//     rather than decrypting to garbage.
//   - FAIL CLOSED: no key, short key or malformed input is an ERROR. There is
//     deliberately no "return the plaintext" fallback anywhere in this file.
//   - The key comes from config/env, never from the database — an attacker
//     reading a DB dump alone cannot decrypt.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// KeyLen is the required key size: AES-256 wants exactly 32 bytes.
const KeyLen = 32

var (
	ErrNoKey      = errors.New("secretbox: no encryption key configured")
	ErrBadKeyLen  = errors.New("secretbox: key must be 32 bytes (base64 or hex encoded)")
	ErrEmptyInput = errors.New("secretbox: empty input")
	ErrMalformed  = errors.New("secretbox: malformed sealed value")
)

// Box seals and opens secrets with one key.
type Box struct {
	aead cipher.AEAD
}

// New builds a Box from a base64 or hex encoded 32-byte key. An empty key is
// ErrNoKey (not an empty Box) so a misconfiguration can never masquerade as
// "encryption is on" while storing plaintext.
func New(encoded string) (*Box, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, ErrNoKey
	}

	var key []byte
	for _, dec := range []func(string) ([]byte, error){base64.StdEncoding.DecodeString, base64.RawStdEncoding.DecodeString, hexDecode} {
		if b, err := dec(encoded); err == nil && len(b) == KeyLen {
			key = b
			break
		}
	}
	if key == nil {
		return nil, ErrBadKeyLen
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secretbox: aes: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secretbox: gcm: %w", err)
	}
	return &Box{aead: aead}, nil
}

// Seal encrypts plaintext and returns base64(nonce||ciphertext).
// An empty plaintext is refused: an empty secret is a bug, not a secret.
func (b *Box) Seal(plaintext string) (string, error) {
	if b == nil || b.aead == nil {
		return "", ErrNoKey
	}
	if plaintext == "" {
		return "", ErrEmptyInput
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("secretbox: nonce: %w", err)
	}
	// Seal appends the ciphertext to nonce, so the result IS nonce||ct.
	return base64.StdEncoding.EncodeToString(b.aead.Seal(nonce, nonce, []byte(plaintext), nil)), nil
}

// Open reverses Seal. Any error means "do not use this value" — callers must
// treat a failure as absent credentials, never as an empty password.
func (b *Box) Open(sealed string) (string, error) {
	if b == nil || b.aead == nil {
		return "", ErrNoKey
	}
	if strings.TrimSpace(sealed) == "" {
		return "", ErrEmptyInput
	}
	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil {
		return "", ErrMalformed
	}
	ns := b.aead.NonceSize()
	if len(raw) <= ns {
		return "", ErrMalformed
	}
	plain, err := b.aead.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		// Wrong key OR tampered ciphertext — same answer either way.
		return "", ErrMalformed
	}
	return string(plain), nil
}

// GenerateKey returns a fresh base64 key, for provisioning a new deployment.
func GenerateKey() (string, error) {
	key := make([]byte, KeyLen)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("secretbox: generate key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(key), nil
}

func hexDecode(s string) ([]byte, error) {
	if len(s) != KeyLen*2 {
		return nil, ErrBadKeyLen
	}
	out := make([]byte, KeyLen)
	for i := 0; i < KeyLen; i++ {
		hi, ok := hexVal(s[i*2])
		if !ok {
			return nil, ErrBadKeyLen
		}
		lo, ok := hexVal(s[i*2+1])
		if !ok {
			return nil, ErrBadKeyLen
		}
		out[i] = hi<<4 | lo
	}
	return out, nil
}

func hexVal(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}
