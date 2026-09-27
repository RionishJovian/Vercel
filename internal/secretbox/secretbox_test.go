package secretbox

import (
	"encoding/base64"
	"strings"
	"testing"
)

const testKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=" // 32 bytes

func mustBox(t *testing.T) *Box {
	t.Helper()
	b, err := New(testKey)
	if err != nil {
		t.Fatalf("New(%q) failed: %v", testKey, err)
	}
	return b
}

func TestSealOpenRoundTrip(t *testing.T) {
	b := mustBox(t)
	for _, secret := range []string{"hunter2", "p@ss word with spaces", "ünïcödé-секрет", strings.Repeat("A", 512)} {
		sealed, err := b.Seal(secret)
		if err != nil {
			t.Fatalf("Seal(%q): %v", secret, err)
		}
		if strings.Contains(sealed, secret) {
			t.Errorf("sealed value contains the plaintext for %q", secret)
		}
		got, err := b.Open(sealed)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		if got != secret {
			t.Errorf("round trip: got %q want %q", got, secret)
		}
	}
}

// The nonce must be fresh per call, or identical secrets become
// distinguishable in the database and GCM's security proof breaks.
func TestSealIsNonDeterministic(t *testing.T) {
	b := mustBox(t)
	a, err := b.Seal("same-secret")
	if err != nil {
		t.Fatal(err)
	}
	c, err := b.Seal("same-secret")
	if err != nil {
		t.Fatal(err)
	}
	if a == c {
		t.Error("two seals of the same plaintext produced identical ciphertext (nonce reuse)")
	}
}

// A tampered ciphertext MUST fail to open. This is why GCM was chosen over
// unauthenticated AES — a bit-flip must not decrypt to a usable password.
func TestTamperedCiphertextFailsClosed(t *testing.T) {
	b := mustBox(t)
	sealed, err := b.Seal("router-password")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil {
		t.Fatal(err)
	}
	// Flip one bit in the body, past the nonce.
	raw[len(raw)-1] ^= 0x01
	tampered := base64.StdEncoding.EncodeToString(raw)

	if _, err := b.Open(tampered); err == nil {
		t.Fatal("a tampered ciphertext opened successfully — integrity is not enforced")
	}
}

// A different key must not open another key's ciphertext.
func TestWrongKeyFailsClosed(t *testing.T) {
	b := mustBox(t)
	sealed, err := b.Seal("router-password")
	if err != nil {
		t.Fatal(err)
	}
	// A genuinely different key, generated rather than hand-written — my first
	// attempt at a literal was not 32 bytes, which the constructor correctly
	// rejected.
	other, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	ob, err := New(other)
	if err != nil {
		t.Fatalf("New(generated other): %v", err)
	}
	if _, err := ob.Open(sealed); err == nil {
		t.Fatal("a ciphertext opened under the WRONG key")
	}
}

// Every misconfiguration must be an error, never a silent plaintext fallback.
func TestMisconfigurationFailsClosed(t *testing.T) {
	cases := map[string]string{
		"empty":       "",
		"whitespace":  "   ",
		"short":       "YWJj",
		"not-base64":  "!!!not-a-key!!!",
		"wrong-len":   base64.StdEncoding.EncodeToString([]byte("too-short")),
		"hex-16byte":  "00112233445566778899001122334455", // 16 bytes, AES-128 key
		"plain-chars": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	for name, key := range cases {
		if _, err := New(key); err == nil {
			t.Errorf("New(%s) accepted an invalid key %q", name, key)
		}
	}
}

// A nil Box must refuse rather than panic — the resolver may hold a nil box
// when no key is configured, and a panic there would take down the sweep.
func TestNilBoxRefuses(t *testing.T) {
	var b *Box
	if _, err := b.Seal("x"); err == nil {
		t.Error("nil Box.Seal returned no error")
	}
	if _, err := b.Open("x"); err == nil {
		t.Error("nil Box.Open returned no error")
	}
}

// Empty secrets are a bug, not a secret — refuse rather than store "".
func TestEmptySecretRefused(t *testing.T) {
	b := mustBox(t)
	if _, err := b.Seal(""); err == nil {
		t.Error("Seal(\"\") succeeded; an empty credential should be a refusal")
	}
	if _, err := b.Open(""); err == nil {
		t.Error("Open(\"\") succeeded")
	}
}

func TestHexKeyAccepted(t *testing.T) {
	hexKey := "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	b, err := New(hexKey)
	if err != nil {
		t.Fatalf("New(hex) failed: %v", err)
	}
	sealed, err := b.Seal("v")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := b.Open(sealed); got != "v" {
		t.Errorf("hex-key round trip got %q", got)
	}
}

func TestGenerateKeyRoundTrips(t *testing.T) {
	k, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	b, err := New(k)
	if err != nil {
		t.Fatalf("generated key rejected: %v", err)
	}
	sealed, err := b.Seal("s3cret")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := b.Open(sealed); got != "s3cret" {
		t.Errorf("generated-key round trip got %q", got)
	}
}
