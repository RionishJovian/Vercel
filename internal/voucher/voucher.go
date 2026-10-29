// Package voucher holds the pure, side-effect-free logic for voucher code
// generation, expiry math, and plan→entitlement mapping. Keeping it free of DB
// and HTTP makes the money-affecting rules directly unit-testable.
package voucher

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"mega-magic/internal/model"
)

// CodeAlphabet is the unambiguous Crockford-style alphabet: no 0/O, 1/I/L, or U
// (avoids the classic "read the printed code aloud" confusions).
const CodeAlphabet = "23456789ABCDEFGHJKMNPQRSTVWXYZ"

// DefaultCodeLength is the generated code length.
const DefaultCodeLength = 12

// Guest internet codes are short, uppercase letters for entry from printed cards.
// Keep existing business-pass generation and acceptance of older codes unchanged.
const GuestCodeLength = 8
const GuestCodeAlphabet = "ABCDEFGHJKMNPQRSTVWXYZ"

// StatusActive is the state a voucher must be in to be redeemable. It is also
// the status CreateVoucher assigns.
const StatusActive = "active"

// StatusUsed is the terminal state after a successful redemption.
const StatusUsed = "used"

// ErrUnknownPlan is returned for a plan_key with no defined entitlement effect.
// Redeem fails closed on unknown plans.
var ErrUnknownPlan = errors.New("voucher: unknown plan_key")

// GenerateCode returns a cryptographically-random, unambiguous code of the given
// length (<=0 uses DefaultCodeLength). It uses rejection sampling to avoid the
// modulo bias of a naive byte%len mapping.
func GenerateCode(r io.Reader, length int) (string, error) {
	if length <= 0 {
		length = DefaultCodeLength
	}
	return generateCode(r, length, CodeAlphabet)
}

// GenerateGuestCode uses uniform cryptographic sampling for new guest vouchers.
func GenerateGuestCode(r io.Reader) (string, error) {
	return generateCode(r, GuestCodeLength, GuestCodeAlphabet)
}

func generateCode(r io.Reader, length int, alphabet string) (string, error) {
	if r == nil {
		return "", errors.New("voucher: nil random source")
	}
	limit := 256 - (256 % len(alphabet))

	out := make([]byte, 0, length)
	buf := make([]byte, length*2)
	for len(out) < length {
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", fmt.Errorf("voucher: read random: %w", err)
		}
		for _, b := range buf {
			if int(b) >= limit {
				continue // reject to keep the distribution uniform
			}
			out = append(out, alphabet[int(b)%len(alphabet)])
			if len(out) == length {
				break
			}
		}
	}
	return string(out), nil
}

// ComputeExpiry returns now+duration_minutes when a positive duration is given,
// else nil (no expiry => the voucher stays valid until used). This fixes the
// "expired at creation" bug.
func ComputeExpiry(now time.Time, durationMinutes *int) *time.Time {
	if durationMinutes == nil || *durationMinutes <= 0 {
		return nil
	}
	t := now.Add(time.Duration(*durationMinutes) * time.Minute)
	return &t
}

// RedeemBlockReason returns "" when a voucher may be redeemed, else a
// human-readable reason. It is the pre-check; the DB claim is still the
// authoritative, race-safe gate.
func RedeemBlockReason(v *model.Voucher, now time.Time) string {
	if v == nil {
		return "voucher not found"
	}
	if v.Status != StatusActive {
		return "voucher is not redeemable (status=" + v.Status + ")"
	}
	if v.ExpiresAt != nil && !now.Before(*v.ExpiresAt) {
		return "voucher has expired"
	}
	return ""
}

// PlanEffect is the entitlement a plan grants on successful redemption.
type PlanEffect struct {
	PlanKey          string
	ExtendWebfigPass bool
}

// ResolvePlanEffect maps a plan_key to its entitlement. Unknown/empty plans are
// denied (fail closed) so a voucher can never grant nothing by accident.
func ResolvePlanEffect(planKey string) (PlanEffect, error) {
	switch strings.ToLower(strings.TrimSpace(planKey)) {
	case "sovereign":
		return PlanEffect{PlanKey: "sovereign", ExtendWebfigPass: true}, nil
	default:
		return PlanEffect{}, fmt.Errorf("%w: %q", ErrUnknownPlan, planKey)
	}
}

// ExtendExpiry computes the new webfig-pass expiry when a voucher is applied:
// if the current pass is still in the future it is extended from there, else it
// starts from now. minutes must be > 0.
func ExtendExpiry(current *time.Time, now time.Time, minutes int) time.Time {
	base := now
	if current != nil && current.After(now) {
		base = *current
	}
	return base.Add(time.Duration(minutes) * time.Minute)
}

// RedeemOutcome models the Go-side single-transaction redemption semantics in a
// pure, DB-free form, so the commit/rollback rule is unit-testable: the voucher
// becomes 'used' ONLY when the entitlement write succeeds. On failure nothing is
// mutated — the real handler rolls the whole transaction back, leaving the
// voucher 'active' so a paid code is never consumed without its entitlement.
func RedeemOutcome(v *model.Voucher, passExpiry *time.Time, now time.Time, minutes int, applyErr error) (*time.Time, error) {
	if applyErr != nil {
		return passExpiry, applyErr // rollback: voucher and pass unchanged
	}
	v.Status = StatusUsed
	if v.FirstSeenAt == nil {
		stamp := now
		v.FirstSeenAt = &stamp
	}
	newExpiry := ExtendExpiry(passExpiry, now, minutes)
	return &newExpiry, nil
}
