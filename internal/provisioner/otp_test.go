package provisioner

import (
	"context"
	"testing"
	"time"
)

// RFC 6238 Appendix B, HMAC-SHA1, secret = ASCII "12345678901234567890".
const rfcSecretB32 = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

func fixedClock(unix int64) func() time.Time {
	return func() time.Time { return time.Unix(unix, 0) }
}

// TestRFC6238_vectors is the authoritative proof: the official RFC 6238
// Appendix B SHA1 test vectors (8-digit).
func TestRFC6238_vectors(t *testing.T) {
	key, err := DecodeBase32Secret(rfcSecretB32)
	if err != nil {
		t.Fatalf("decode RFC secret: %v", err)
	}
	vectors := []struct {
		unix int64
		want string
	}{
		{59, "94287082"},
		{1111111109, "07081804"},
		{1111111111, "14050471"},
		{1234567890, "89005924"},
		{2000000000, "69279037"},
		{20000000000, "65353130"},
	}
	for _, v := range vectors {
		got := HOTP(key, uint64(v.unix/30), 8)
		if got != v.want {
			t.Fatalf("t=%d: got %s, want %s", v.unix, got, v.want)
		}
		t.Logf("RFC6238 SHA1 t=%-12d expected=%s actual=%s ok", v.unix, v.want, got)
		// GenerateTOTP must agree with HOTP.
		if g := GenerateTOTP(key, time.Unix(v.unix, 0), 30*time.Second, 8); g != v.want {
			t.Fatalf("GenerateTOTP t=%d: got %s, want %s", v.unix, g, v.want)
		}
	}

	// 6-digit variant of the same authoritative vector: 94287082 mod 1e6.
	if got := GenerateTOTP(key, time.Unix(59, 0), 30*time.Second, 6); got != "287082" {
		t.Fatalf("6-digit t=59: got %s, want 287082", got)
	}
}

// TestTOTP_citedExample documents that the "JBSWY3DPEHPK3PXP @t=59 -> 287082"
// example is a mis-citation: 287082 belongs to the RFC secret above. The actual
// code for JBSWY3DPEHPK3PXP at t=59 is 996554.
func TestTOTP_citedExample(t *testing.T) {
	key, err := DecodeBase32Secret("JBSWY3DPEHPK3PXP")
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := GenerateTOTP(key, time.Unix(59, 0), 30*time.Second, 6); got != "996554" {
		t.Fatalf("JBSWY3DPEHPK3PXP t=59 6-digit = %s, expected 996554", got)
	}
}

func TestVerify_acceptsKnownRFCVector(t *testing.T) {
	v := NewTOTPVerifier(TOTPConfig{Secret: rfcSecretB32, Digits: 8, Now: fixedClock(59)})
	ok, err := v.Verify(context.Background(), "peer2", "94287082")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !ok {
		t.Fatal("valid RFC code must be accepted")
	}
}

func TestVerify_window(t *testing.T) {
	key, _ := DecodeBase32Secret(rfcSecretB32)
	now := int64(90) // counter 3
	code := func(unix int64) string { return GenerateTOTP(key, time.Unix(unix, 0), 30*time.Second, 6) }

	cases := []struct {
		name string
		unix int64
		want bool
	}{
		{"current step", 90, true},
		{"prev step (lookbehind 1)", 60, true},
		{"next step (lookahead 1)", 120, true},
		{"two steps back", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := NewTOTPVerifier(TOTPConfig{Secret: rfcSecretB32, Digits: 6, Skew: 1, Now: fixedClock(now)})
			ok, err := v.Verify(context.Background(), "p", code(tc.unix))
			if err != nil {
				t.Fatalf("Verify: %v", err)
			}
			if ok != tc.want {
				t.Fatalf("unix=%d accepted=%v, want %v", tc.unix, ok, tc.want)
			}
		})
	}
}

func TestVerify_replayRejected(t *testing.T) {
	v := NewTOTPVerifier(TOTPConfig{Secret: rfcSecretB32, Digits: 8, Now: fixedClock(59)})
	code := "94287082"
	if ok, _ := v.Verify(context.Background(), "p", code); !ok {
		t.Fatal("first use must be accepted")
	}
	if ok, _ := v.Verify(context.Background(), "p", code); ok {
		t.Fatal("replaying the same code within the step must be denied")
	}
	// A different peer may still use the same code (per-peer replay scope).
	if ok, _ := v.Verify(context.Background(), "q", code); !ok {
		t.Fatal("replay guard must be per-peer")
	}
}

func TestVerify_failsClosed(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		cfg  TOTPConfig
		code string
	}{
		{"empty secret", TOTPConfig{Now: fixedClock(59)}, "94287082"},
		{"malformed base32", TOTPConfig{Secret: "NOT!BASE32", Now: fixedClock(59)}, "94287082"},
		{"empty code", TOTPConfig{Secret: rfcSecretB32, Now: fixedClock(59)}, ""},
		{"wrong code", TOTPConfig{Secret: rfcSecretB32, Now: fixedClock(59)}, "00000000"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, err := NewTOTPVerifier(tc.cfg).Verify(ctx, "p", tc.code)
			if err != nil {
				t.Fatalf("fail-closed paths must not error, got %v", err)
			}
			if ok {
				t.Fatal("must deny")
			}
		})
	}
}

func TestVerify_perPeerSecret(t *testing.T) {
	keyA, _ := DecodeBase32Secret(rfcSecretB32)
	keyB, _ := DecodeBase32Secret("JBSWY3DPEHPK3PXP")
	prov := MapOtpSecret{
		"peer-a": rfcSecretB32,
		"peer-b": "JBSWY3DPEHPK3PXP",
	}
	v := NewTOTPVerifierWithProvider(TOTPConfig{Digits: 6, Now: fixedClock(59)}, prov)

	codeA := GenerateTOTP(keyA, time.Unix(59, 0), 30*time.Second, 6)
	codeB := GenerateTOTP(keyB, time.Unix(59, 0), 30*time.Second, 6)

	if ok, _ := v.Verify(context.Background(), "peer-a", codeA); !ok {
		t.Fatal("peer-a's own code must be accepted")
	}
	if ok, _ := v.Verify(context.Background(), "peer-a", codeB); ok {
		t.Fatal("peer-a must not accept peer-b's code")
	}
	if ok, _ := v.Verify(context.Background(), "peer-b", codeB); !ok {
		t.Fatal("peer-b's own code must be accepted")
	}
	// Unknown peer denies.
	if ok, _ := v.Verify(context.Background(), "peer-unknown", codeA); ok {
		t.Fatal("unknown peer must deny")
	}
}

func TestVerify_nilReceiverDenies(t *testing.T) {
	var v *TOTPVerifier
	if ok, err := v.Verify(context.Background(), "p", "123456"); ok || err != nil {
		t.Fatalf("nil verifier = (%v,%v), want (false,nil)", ok, err)
	}
}

func TestConstantTimeEqual(t *testing.T) {
	if !constantTimeEqual("123456", "123456") {
		t.Fatal("equal strings must compare equal")
	}
	if constantTimeEqual("123456", "123457") {
		t.Fatal("different strings must not compare equal")
	}
	if constantTimeEqual("123456", "12345") {
		t.Fatal("length mismatch must not compare equal")
	}
	if constantTimeEqual("123456", "1234567") {
		t.Fatal("length mismatch must not compare equal")
	}
	if constantTimeEqual("", "0") {
		t.Fatal("empty vs non-empty must not compare equal")
	}
	if !constantTimeEqual("", "") {
		t.Fatal("empty vs empty must compare equal")
	}
}

func TestDecodeBase32Secret_toleratesFormatting(t *testing.T) {
	a, err := DecodeBase32Secret("GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ")
	if err != nil {
		t.Fatalf("plain: %v", err)
	}
	b, err := DecodeBase32Secret("gezd gnbv gy3t qojq gezd gnbv gy3t qojq")
	if err != nil {
		t.Fatalf("spaced/lowercase: %v", err)
	}
	if string(a) != "12345678901234567890" || string(b) != string(a) {
		t.Fatalf("decode mismatch: %q vs %q", a, b)
	}
	if _, err := DecodeBase32Secret(""); err == nil {
		t.Fatal("empty secret must error")
	}
}

// TestEvaluateApproval_withRealTOTP ties Slice 2 to the gate: a high-risk plan
// approves ONLY with a valid TOTP, and stays denied with none.
func TestEvaluateApproval_withRealTOTP(t *testing.T) {
	ctx := context.Background()
	verifier := NewTOTPVerifier(TOTPConfig{Secret: rfcSecretB32, Digits: 8, Now: fixedClock(59)})
	plan := highRiskPlan() // rotate_listen_port/escalate

	// No OTP -> denied (otp_required).
	if out, err := EvaluateApproval(ctx, "p", plan, ApprovalDecision{Approved: true}, verifier); err != nil || out.Allowed {
		t.Fatalf("high-risk without OTP = (%+v,%v), want denied", out, err)
	}
	// Valid OTP -> allowed.
	out, err := EvaluateApproval(ctx, "p", plan, ApprovalDecision{Approved: true, OTP: "94287082"}, verifier)
	if err != nil {
		t.Fatalf("EvaluateApproval: %v", err)
	}
	if !out.Allowed || out.Reason != ReasonApproved {
		t.Fatalf("high-risk with valid OTP = %+v, want allowed", out)
	}
	// Empty secret verifier -> still denied.
	noSecret := NewTOTPVerifier(TOTPConfig{Now: fixedClock(59)})
	if out, _ := EvaluateApproval(ctx, "p", plan, ApprovalDecision{Approved: true, OTP: "94287082"}, noSecret); out.Allowed {
		t.Fatal("unconfigured secret must keep high-risk denied")
	}
}
