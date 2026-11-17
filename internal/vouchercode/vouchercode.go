// Package vouchercode protects guest and business voucher codes at rest.
//
// A voucher code cannot be stored as a one-way hash: the router's HotSpot user name is the code, and the app must be
// able to read it back to sync, kick, ban, retry and print. So each code is kept as
//
//   - ciphertext (AES-256-GCM via internal/secretbox), recoverable only with the key, and
//   - a keyed blind index (HMAC-SHA256), so a typed code can be found without storing it in clear.
//
// A leaked database, backup or read-only SQL grant then yields neither usable codes nor an offline guessing oracle
// (a plain hash of an 8-letter code, about 35 bits, would fall in minutes). It does not protect against an attacker
// who holds both the database and this key. The key is separate from the credential key so rotating one never breaks
// the other, and it must be backed up: without it protected codes cannot be recovered.
package vouchercode

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"strings"

	"golang.org/x/crypto/hkdf"

	"mega-magic/internal/secretbox"
)

// ErrNoKey means protection was asked for without a usable key. Callers fail closed.
var ErrNoKey = errors.New("vouchercode: no protection key configured")

// ErrBadKey means the key is not a base64 or hex encoded 32-byte value.
var ErrBadKey = errors.New("vouchercode: key must be 32 bytes (base64 or hex encoded)")

const (
	keyLen  = 32
	infoEnc = "mega-magic/voucher-code/aes-256-gcm/v1"
	infoMAC = "mega-magic/voucher-code/hmac-sha256/v1"
)

// Protector seals codes and computes their blind index. It is safe for concurrent use.
type Protector struct {
	box *secretbox.Box
	mac []byte
}

// New derives independent encryption and index keys from one master key (HKDF-SHA256, distinct context strings).
func New(masterKey string) (*Protector, error) {
	raw, err := decodeKey(masterKey)
	if err != nil {
		return nil, err
	}
	encKey, err := derive(raw, infoEnc)
	if err != nil {
		return nil, err
	}
	box, err := secretbox.New(hex.EncodeToString(encKey))
	if err != nil {
		return nil, err
	}
	macKey, err := derive(raw, infoMAC)
	if err != nil {
		return nil, err
	}
	return &Protector{box: box, mac: macKey}, nil
}

// Blind returns the lowercase-hex HMAC-SHA256 of the code exactly as given. Callers normalize first
// (the guest path upper-cases and trims; the business-pass path matches case exactly, as it always has).
func (p *Protector) Blind(code string) string {
	m := hmac.New(sha256.New, p.mac)
	m.Write([]byte(code))
	return hex.EncodeToString(m.Sum(nil))
}

// Seal encrypts a code. Every call yields a different ciphertext.
func (p *Protector) Seal(code string) (string, error) { return p.box.Seal(code) }

// Open decrypts a sealed code. Any error means the value must not be used.
func (p *Protector) Open(sealed string) (string, error) { return p.box.Open(sealed) }

// GenerateKey returns a new random master key in the format New accepts.
func GenerateKey() (string, error) { return secretbox.GenerateKey() }

func decodeKey(encoded string) ([]byte, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, ErrNoKey
	}
	for _, dec := range []func(string) ([]byte, error){base64.StdEncoding.DecodeString, base64.RawStdEncoding.DecodeString, hex.DecodeString} {
		if b, err := dec(encoded); err == nil && len(b) == keyLen {
			return b, nil
		}
	}
	return nil, ErrBadKey
}

func derive(master []byte, info string) ([]byte, error) {
	out := make([]byte, keyLen)
	if _, err := io.ReadFull(hkdf.New(sha256.New, master, nil, []byte(info)), out); err != nil {
		return nil, err
	}
	return out, nil
}
