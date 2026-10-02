package routeros

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"mega-magic/internal/guestaccess"
)

type bindingRunner struct {
	base             *recoveryRunner
	guardName, event string
	failure          string
}

func (r *bindingRunner) Run(ctx context.Context, cmd string) (string, error) {
	switch {
	case strings.HasPrefix(cmd, "/system scheduler print count-only"):
		r.base.commands = append(r.base.commands, cmd)
		if r.failure == "duplicate guard" {
			return "2", nil
		}
		return "1", nil
	case strings.HasPrefix(cmd, "/system scheduler print value-list"):
		r.base.commands = append(r.base.commands, cmd)
		event, disabled, start := r.event, "no", "startup"
		if r.failure == "guard changed" {
			event += " :put bypass"
		}
		if r.failure == "disabled guard" {
			disabled = "yes"
		}
		if r.failure == "non-startup guard" {
			start = "00:00:00"
		}
		return "name: " + r.guardName + "\non-event: " + event + "\ninterval: 10s\ndisabled: " + disabled + "\nstart-time: " + start, nil
	case strings.HasPrefix(cmd, ":local id [/ip hotspot user find"):
		r.base.commands = append(r.base.commands, cmd)
		if r.base.active {
			return "", fmt.Errorf("active snapshot unavailable")
		}
		if r.failure == "active race" {
			r.base.active = true
		}
		if r.failure == "invalid counters" {
			return "bytes-in: unknown\nbytes-out: 1", nil
		}
		if r.failure == "negative counters" {
			return "bytes-in: -1\nbytes-out: 1", nil
		}
		if r.failure == "exhausted" {
			return "bytes-in: 35000000\nbytes-out: 0", nil
		}
		return "bytes-in: 6000000\nbytes-out: 4000000", nil
	}
	return r.base.Run(ctx, cmd)
}
func newBindingRunner(original bool) (*bindingRunner, guestaccess.BindingSnapshot) {
	base, claim := recoveryRunnerFixture()
	base.active = false
	base.g.NewMAC = claim.OldMAC
	g := guestaccess.BindingSnapshot{BusinessID: claim.BusinessID, DeviceID: claim.DeviceID, VoucherID: claim.VoucherID, ClientMAC: claim.OldMAC, TotalBytes: 75000000, PriorUsedBytes: 40000000, ExpiresAt: claim.ExpiresAt, Revision: time.Now()}
	r := &bindingRunner{base: base, guardName: "mm-recovery-" + claim.TransferID}
	base.users[g.ClientMAC]["limit-bytes-total"] = "35000000"
	claim.NewMAC = g.ClientMAC
	r.event = recoveryExpiryEvent(claim, r.guardName)
	if original {
		g.PriorUsedBytes = 0
		base.users[g.ClientMAC]["limit-bytes-total"] = "75000000"
		r.guardName = "mm-voucher-" + g.VoucherID
		r.event = voucherExpiryEvent(g.VoucherID, g.ClientMAC, g.ExpiresAt, r.guardName)
	}
	return r, g
}
func TestCurrentBindingVerifierIsReadOnlyAndPreservesQuota(t *testing.T) {
	for _, original := range []bool{true, false} {
		r, g := newBindingRunner(original)
		used, err := verifyGuestBinding(context.Background(), r, g, r.base.now)
		if err != nil || used != 10000000 {
			t.Fatalf("used=%d error=%v", used, err)
		}
		for _, cmd := range r.base.commands {
			if !strings.Contains(cmd, " print ") && !strings.HasPrefix(cmd, ":put [:tonsec [:timestamp]]") && !strings.HasPrefix(cmd, ":local id [/ip hotspot user find") {
				t.Fatalf("non-read operation: %s", cmd)
			}
			for _, write := range []string{" user add ", " user enable ", " user disable ", " active remove ", " cookie remove ", "reset-counters", "ip-binding", " scheduler add "} {
				if strings.Contains(cmd, write) {
					t.Fatalf("resume mutated router: %s", cmd)
				}
			}
		}
		if recoveryDisabled(r.base.users[g.ClientMAC]) || r.base.active || r.base.users[g.ClientMAC]["limit-bytes-total"] != fmt.Sprint(g.TotalBytes-g.PriorUsedBytes) {
			t.Fatal("resume changed current connection")
		}
	}
}
func TestCurrentBindingVerifierRejectsUncertainAuthority(t *testing.T) {
	for _, failure := range []string{"foreign user", "wrong quota", "disabled user", "foreign host", "clock skew", "duplicate guard", "guard changed", "disabled guard", "non-startup guard", "foreign guard", "active", "active race", "invalid counters", "negative counters", "exhausted"} {
		t.Run(failure, func(t *testing.T) {
			r, g := newBindingRunner(false)
			r.failure = failure
			switch failure {
			case "foreign user":
				r.base.users[g.ClientMAC]["comment"] = "foreign"
			case "wrong quota":
				r.base.users[g.ClientMAC]["limit-bytes-total"] = "75000000"
			case "disabled user":
				r.base.users[g.ClientMAC]["disabled"] = "yes"
			case "foreign host":
				r.base.fail = "host"
			case "clock skew":
				r.base.now = r.base.now.Add(-time.Minute)
			case "foreign guard":
				r.guardName = "unrelated"
			case "active":
				r.base.active = true
			}
			used, err := verifyGuestBinding(context.Background(), r, g, time.Now())
			if err == nil || used != 0 {
				t.Fatal("unverified snapshot returned usable quota")
			}
			for _, cmd := range r.base.commands {
				if strings.Contains(cmd, " remove ") || strings.Contains(cmd, "reset-counters") {
					t.Fatal("failed status disconnected or reset guest")
				}
			}
		})
	}
}
