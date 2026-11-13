package vouchercode

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

func mustKey(t *testing.T) string {
	t.Helper()
	k, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func mustNew(t *testing.T, key string) *Protector {
	t.Helper()
	p, err := New(key)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSealOpenRoundTripAndNonDeterministic(t *testing.T) {
	p := mustNew(t, mustKey(t))
	a, err := p.Seal("ABCDEFGH")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := p.Seal("ABCDEFGH")
	if a == b {
		t.Fatal("two seals of the same code must differ")
	}
	if strings.Contains(a, "ABCDEFGH") {
		t.Fatal("ciphertext contains the code")
	}
	got, err := p.Open(a)
	if err != nil || got != "ABCDEFGH" {
		t.Fatalf("Open = %q, %v", got, err)
	}
}

func TestOpenFailsWithAnotherKeyOrTampering(t *testing.T) {
	p, other := mustNew(t, mustKey(t)), mustNew(t, mustKey(t))
	sealed, _ := p.Seal("ABCDEFGH")
	if _, err := other.Open(sealed); err == nil {
		t.Fatal("a different key must not open the code")
	}
	if _, err := p.Open(sealed[:len(sealed)-4] + "AAAA"); err == nil {
		t.Fatal("tampered ciphertext must not open")
	}
}

// The blind index must be keyed: an attacker holding only the database cannot recompute it from a guessed code.
func TestBlindIsDeterministicKeyedAndNotAPlainHash(t *testing.T) {
	k := mustKey(t)
	p, same, other := mustNew(t, k), mustNew(t, k), mustNew(t, mustKey(t))
	if p.Blind("ABCDEFGH") != same.Blind("ABCDEFGH") {
		t.Fatal("same key and code must give the same index")
	}
	if p.Blind("ABCDEFGH") == other.Blind("ABCDEFGH") {
		t.Fatal("a different key must give a different index")
	}
	if p.Blind("ABCDEFGH") == p.Blind("ABCDEFGJ") {
		t.Fatal("different codes must give different indexes")
	}
	plain := sha256.Sum256([]byte("ABCDEFGH"))
	if p.Blind("ABCDEFGH") == hex.EncodeToString(plain[:]) {
		t.Fatal("blind index must not be a plain SHA-256 of the code")
	}
	if got := p.Blind("ABCDEFGH"); len(got) != 64 || strings.ToLower(got) != got {
		t.Fatalf("blind index must be 64 lowercase hex characters, got %q", got)
	}
}

// Sealing and indexing use independent derived keys: leaking one must not give the other.
func TestSubkeysAreIndependent(t *testing.T) {
	raw, _ := decodeKey(mustKey(t))
	enc, _ := derive(raw, infoEnc)
	mac, _ := derive(raw, infoMAC)
	if hex.EncodeToString(enc) == hex.EncodeToString(mac) || hex.EncodeToString(enc) == hex.EncodeToString(raw) {
		t.Fatal("derived keys must differ from each other and from the master key")
	}
}

func TestRejectsMissingOrBadKeys(t *testing.T) {
	if _, err := New(""); !errors.Is(err, ErrNoKey) {
		t.Fatalf("empty key: %v", err)
	}
	for _, bad := range []string{"short", strings.Repeat("a", 63), "not base64 or hex !!"} {
		if _, err := New(bad); !errors.Is(err, ErrBadKey) {
			t.Fatalf("key %q: %v", bad, err)
		}
	}
}
