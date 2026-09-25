package voucher

import (
	"bytes"
	"crypto/rand"
	"errors"
	"strings"
	"testing"
	"time"

	"mega-magic/internal/model"
)

func TestCodeAlphabet_unambiguous(t *testing.T) {
	for _, bad := range []string{"0", "1", "I", "L", "O", "U"} {
		if strings.Contains(CodeAlphabet, bad) {
			t.Fatalf("alphabet must not contain ambiguous %q", bad)
		}
	}
	if len(CodeAlphabet) != 30 {
		t.Fatalf("alphabet length = %d, want 30", len(CodeAlphabet))
	}
}

func TestGenerateCode_shape(t *testing.T) {
	code, err := GenerateCode(rand.Reader, DefaultCodeLength)
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}
	if len(code) != DefaultCodeLength {
		t.Fatalf("length = %d, want %d", len(code), DefaultCodeLength)
	}
	for _, r := range code {
		if !strings.ContainsRune(CodeAlphabet, r) {
			t.Fatalf("code %q contains out-of-alphabet rune %q", code, r)
		}
	}
}

func TestGenerateCode_deterministicAndRejection(t *testing.T) {
	// Bytes >= 240 must be rejected; the alphabet index is b%30.
	// 5 -> '7', 6 -> '8', 7 -> '9'.
	src := bytes.NewReader([]byte{255, 240, 5, 6, 7, 8})
	code, err := GenerateCode(src, 3)
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}
	if code != "789" {
		t.Fatalf("code = %q, want %q (rejection sampling)", code, "789")
	}
}

func TestGenerateCode_nilReader(t *testing.T) {
	if _, err := GenerateCode(nil, 12); err == nil {
		t.Fatal("nil reader must error")
	}
}

func TestGenerateCode_defaultLength(t *testing.T) {
	code, err := GenerateCode(rand.Reader, 0)
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}
	if len(code) != DefaultCodeLength {
		t.Fatalf("default length = %d, want %d", len(code), DefaultCodeLength)
	}
}

func TestGenerateCode_unique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 2000; i++ {
		c, err := GenerateCode(rand.Reader, DefaultCodeLength)
		if err != nil {
			t.Fatalf("GenerateCode: %v", err)
		}
		if seen[c] {
			t.Fatalf("duplicate code generated: %s", c)
		}
		seen[c] = true
	}
}

func TestComputeExpiry(t *testing.T) {
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	if e := ComputeExpiry(now, nil); e != nil {
		t.Fatalf("nil duration must give no expiry, got %v", e)
	}
	zero := 0
	if e := ComputeExpiry(now, &zero); e != nil {
		t.Fatalf("zero duration must give no expiry, got %v", e)
	}
	neg := -5
	if e := ComputeExpiry(now, &neg); e != nil {
		t.Fatalf("negative duration must give no expiry, got %v", e)
	}
	d := 60
	e := ComputeExpiry(now, &d)
	if e == nil || !e.Equal(now.Add(60*time.Minute)) {
		t.Fatalf("expiry = %v, want %v", e, now.Add(60*time.Minute))
	}
}

func TestResolvePlanEffect(t *testing.T) {
	cases := []struct {
		key      string
		wantOK   bool
		wantPass bool
	}{
		{"sovereign", true, true},
		{" Sovereign ", true, true},
		{"SOVEREIGN", true, true},
		{"", false, false},
		{"plus", false, false},  // retired tier => deny
		{"basic", false, false}, // unknown => deny
		{"sovereignx", false, false},
	}
	for _, tc := range cases {
		eff, err := ResolvePlanEffect(tc.key)
		if tc.wantOK && err != nil {
			t.Fatalf("%q: unexpected error %v", tc.key, err)
		}
		if !tc.wantOK {
			if err == nil {
				t.Fatalf("%q: must deny unknown plan", tc.key)
			}
			continue
		}
		if eff.ExtendWebfigPass != tc.wantPass {
			t.Fatalf("%q: effect = %+v", tc.key, eff)
		}
	}
}

func TestExtendExpiry(t *testing.T) {
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

	// No current pass -> start from now.
	if got := ExtendExpiry(nil, now, 30); !got.Equal(now.Add(30 * time.Minute)) {
		t.Fatalf("nil current = %v, want %v", got, now.Add(30*time.Minute))
	}
	// Expired current pass -> start from now (no negative credit).
	past := now.Add(-time.Hour)
	if got := ExtendExpiry(&past, now, 30); !got.Equal(now.Add(30 * time.Minute)) {
		t.Fatalf("past current = %v, want %v", got, now.Add(30*time.Minute))
	}
	// Future current pass -> stack on top.
	future := now.Add(time.Hour)
	if got := ExtendExpiry(&future, now, 30); !got.Equal(future.Add(30 * time.Minute)) {
		t.Fatalf("future current = %v, want %v", got, future.Add(30*time.Minute))
	}
}

func activeVoucher() *model.Voucher {
	return &model.Voucher{ID: "v1", BusinessID: "b1", PlanKey: "sovereign", Code: "ABC", Status: StatusActive}
}

func TestRedeemBlockReason(t *testing.T) {
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

	if got := RedeemBlockReason(nil, now); got == "" {
		t.Fatal("nil voucher must be blocked")
	}
	if got := RedeemBlockReason(activeVoucher(), now); got != "" {
		t.Fatalf("active voucher must be redeemable, got %q", got)
	}

	future := now.Add(time.Hour)
	v := activeVoucher()
	v.ExpiresAt = &future
	if got := RedeemBlockReason(v, now); got != "" {
		t.Fatalf("unexpired voucher must be redeemable, got %q", got)
	}

	past := now.Add(-time.Hour)
	v = activeVoucher()
	v.ExpiresAt = &past
	if got := RedeemBlockReason(v, now); got == "" {
		t.Fatal("expired voucher must be blocked")
	}

	used := activeVoucher()
	used.Status = StatusUsed
	if got := RedeemBlockReason(used, now); got == "" {
		t.Fatal("used voucher must be blocked")
	}
}

// TestRedeemClaim_secondRedeemBlocked models the claim transition purely: an
// active voucher is claimable; after the atomic claim flips it to "used", a
// second redemption is blocked. (The DB claim itself is exercised by DB tests
// when a harness exists.)
func TestRedeemClaim_secondRedeemBlocked(t *testing.T) {
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	v := activeVoucher()

	if RedeemBlockReason(v, now) != "" {
		t.Fatal("first redeem should be allowed")
	}
	// Simulate the successful atomic claim.
	v.Status = StatusUsed
	if RedeemBlockReason(v, now) == "" {
		t.Fatal("second redeem must be blocked after the claim")
	}
}

// TestRedeemOutcome_tierFailureLeavesVoucherActive proves the single-transaction
// rollback rule: if the entitlement write fails, the voucher is NOT left 'used'
// (and its pass is unchanged), mirroring the handler's tx Rollback.
func TestRedeemOutcome_tierFailureLeavesVoucherActive(t *testing.T) {
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	v := activeVoucher()
	var noPass *time.Time

	// Entitlement write fails -> rollback: voucher stays active, no pass granted.
	newExpiry, err := RedeemOutcome(v, noPass, now, 60, errTier)
	if err == nil {
		t.Fatal("failed entitlement must surface an error")
	}
	if v.Status != StatusActive {
		t.Fatalf("voucher status = %q, want still active after rollback", v.Status)
	}
	if v.FirstSeenAt != nil {
		t.Fatal("first_seen_at must not be stamped when the tx rolls back")
	}
	if newExpiry != nil {
		t.Fatal("no pass expiry must be applied on rollback")
	}

	// Success -> voucher used and pass extended.
	v2 := activeVoucher()
	gotExpiry, err := RedeemOutcome(v2, noPass, now, 60, nil)
	if err != nil {
		t.Fatalf("success must not error: %v", err)
	}
	if v2.Status != StatusUsed || v2.FirstSeenAt == nil {
		t.Fatalf("voucher after success = %+v, want used+first_seen", v2)
	}
	if gotExpiry == nil || !gotExpiry.Equal(now.Add(60*time.Minute)) {
		t.Fatalf("expiry = %v, want %v", gotExpiry, now.Add(60*time.Minute))
	}
}

var errTier = errors.New("quota write failed")
