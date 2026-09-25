package provisioner

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// RFC 6238 TOTP verifier (Slice 2)
// ---------------------------------------------------------------------------
//
// Implements the provisioner.OtpVerifier seam: a genuine RFC 6238 TOTP
// (HMAC-SHA1, 30s step, 6 digits by default) over a base32 shared secret. Same
// fail-closed posture as the rest of the provisioner: any missing secret,
// malformed input, or unknown peer DENIES (returns false) — it never errors the
// approval path and never panics.

// OtpSecretProvider resolves the base32 TOTP secret for a peer. It honors the
// interface's per-peer intent even when a single shared lab secret is used.
type OtpSecretProvider interface {
	SecretFor(peerID string) (secretBase32 string, ok bool)
}

// SharedOtpSecret returns one base32 secret for every peer.
type SharedOtpSecret struct{ Secret string }

func (s SharedOtpSecret) SecretFor(string) (string, bool) { return s.Secret, s.Secret != "" }

// MapOtpSecret resolves per-peer base32 secrets. Missing/empty => fail closed.
type MapOtpSecret map[string]string

func (m MapOtpSecret) SecretFor(peerID string) (string, bool) {
	s, ok := m[peerID]
	return s, ok && s != ""
}

// TOTPConfig configures the verifier. Zero values fall back to the RFC defaults
// (6 digits, 30s step, ±1 step skew, wall clock, replay guard on).
type TOTPConfig struct {
	Secret             string        // base32 shared secret; empty => always deny
	Digits             int           // default 6
	Step               time.Duration // default 30s
	Skew               int           // default 1 (± steps accepted)
	Now                func() time.Time
	DisableReplayGuard bool // default false (replay guard ON)
}

// TOTPVerifier is a real RFC 6238 TOTP verifier.
type TOTPVerifier struct {
	secrets OtpSecretProvider
	digits  int
	step    time.Duration
	skew    int
	now     func() time.Time
	replay  bool

	mu   sync.Mutex
	used map[string]map[int64]bool // peerID -> accepted step counters (replay guard)
}

// NewTOTPVerifier builds a verifier from a single shared base32 secret.
func NewTOTPVerifier(cfg TOTPConfig) *TOTPVerifier {
	return NewTOTPVerifierWithProvider(cfg, SharedOtpSecret{Secret: cfg.Secret})
}

// NewTOTPVerifierWithProvider builds a verifier over a secret provider (for
// per-peer secrets).
func NewTOTPVerifierWithProvider(cfg TOTPConfig, secrets OtpSecretProvider) *TOTPVerifier {
	digits := cfg.Digits
	if digits <= 0 {
		digits = 6
	}
	step := cfg.Step
	if step <= 0 {
		step = 30 * time.Second
	}
	skew := cfg.Skew
	if skew < 0 {
		skew = 0
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	if secrets == nil {
		secrets = SharedOtpSecret{}
	}
	return &TOTPVerifier{
		secrets: secrets,
		digits:  digits,
		step:    step,
		skew:    skew,
		now:     now,
		replay:  !cfg.DisableReplayGuard,
		used:    map[string]map[int64]bool{},
	}
}

var _ OtpVerifier = (*TOTPVerifier)(nil)

// Verify implements provisioner.OtpVerifier. It returns true only for a code
// that matches the peer's secret within the accepted window AND has not already
// been accepted for that step. Every failure path returns (false, nil).
func (v *TOTPVerifier) Verify(_ context.Context, peerID, code string) (bool, error) {
	if v == nil {
		return false, nil
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return false, nil
	}

	secretB32, ok := v.secrets.SecretFor(peerID)
	if !ok || secretB32 == "" {
		return false, nil // no secret configured for this peer => deny (fail closed)
	}
	key, err := DecodeBase32Secret(secretB32)
	if err != nil || len(key) == 0 {
		return false, nil // malformed secret => deny
	}

	counter := v.now().Unix() / int64(v.step/time.Second)

	matched := int64(-1)
	for d := -v.skew; d <= v.skew; d++ {
		c := counter + int64(d)
		if c < 0 {
			continue
		}
		if constantTimeEqual(GenerateTOTP(key, time.Unix(c*int64(v.step/time.Second), 0), v.step, v.digits), code) {
			matched = c
		}
	}
	if matched < 0 {
		return false, nil
	}

	if v.replay {
		v.mu.Lock()
		defer v.mu.Unlock()
		if v.used[peerID] == nil {
			v.used[peerID] = map[int64]bool{}
		}
		if v.used[peerID][matched] {
			return false, nil // replay within the same step => deny
		}
		v.used[peerID][matched] = true
	}
	return true, nil
}

// ---------------------------------------------------------------------------
// RFC 6238 primitives (exported for tests/diagnostics and Slice 3)
// ---------------------------------------------------------------------------

// HOTP computes the RFC 4226 HOTP value for a counter.
func HOTP(key []byte, counter uint64, digits int) string {
	if digits <= 0 {
		digits = 6
	}
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, counter)

	mac := hmac.New(sha1.New, key)
	mac.Write(buf)
	sum := mac.Sum(nil)

	// Dynamic truncation (RFC 4226 section 5.3).
	off := sum[len(sum)-1] & 0x0f
	code := (uint32(sum[off]&0x7f) << 24) |
		(uint32(sum[off+1]) << 16) |
		(uint32(sum[off+2]) << 8) |
		uint32(sum[off+3])

	mod := uint32(1)
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", digits, code%mod)
}

// GenerateTOTP computes the RFC 6238 TOTP value for a time and step.
func GenerateTOTP(key []byte, t time.Time, step time.Duration, digits int) string {
	if step <= 0 {
		step = 30 * time.Second
	}
	counter := uint64(t.Unix() / int64(step/time.Second))
	return HOTP(key, counter, digits)
}

// DecodeBase32Secret decodes a base32 TOTP secret, tolerating spaces/hyphens
// and missing padding (both common in authenticator exports).
func DecodeBase32Secret(s string) ([]byte, error) {
	clean := strings.ToUpper(strings.TrimSpace(s))
	clean = strings.NewReplacer(" ", "", "-", "").Replace(clean)
	if clean == "" {
		return nil, fmt.Errorf("provisioner: empty TOTP secret")
	}
	if b, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(clean); err == nil {
		return b, nil
	}
	return base32.StdEncoding.DecodeString(clean)
}

// constantTimeEqual compares two strings without leaking length or prefix via an
// early exit: it always scans the longer length and folds a length mismatch into
// the result.
func constantTimeEqual(a, b string) bool {
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	var v byte
	for i := 0; i < n; i++ {
		var ca, cb byte
		if i < len(a) {
			ca = a[i]
		}
		if i < len(b) {
			cb = b[i]
		}
		v |= ca ^ cb
	}
	if len(a) != len(b) {
		v |= 1
	}
	return v == 0
}
