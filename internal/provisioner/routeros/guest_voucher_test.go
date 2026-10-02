package routeros

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
)

type voucherRunner struct {
	commands                          []string
	now                               time.Time
	clockReads                        int
	expireDuringSetup                 bool
	badQuota, badGuard, badActivation bool
	login, user, event                string
}

func (r *voucherRunner) Run(_ context.Context, cmd string) (string, error) {
	r.commands = append(r.commands, cmd)
	switch {
	case cmd == ":put [:tonsec [:timestamp]]":
		r.clockReads++
		at := r.now
		if r.expireDuringSetup && r.clockReads > 1 {
			at = at.Add(time.Minute)
		}
		return strconv.FormatInt(at.UnixNano(), 10), nil
	case strings.HasPrefix(cmd, "/ip hotspot host print"):
		return "mac-address: AA:BB:CC:DD:EE:FE\nserver: guest1", nil
	case strings.HasPrefix(cmd, "/ip hotspot print"):
		return "profile: guest-profile", nil
	case strings.HasPrefix(cmd, "/ip hotspot profile print"):
		return "login-by: " + r.login + "\nmac-auth-password: local-secret", nil
	case strings.HasPrefix(cmd, "/ip hotspot user print"):
		return r.user, nil
	case strings.HasPrefix(cmd, "/system scheduler print count-only"):
		if r.event == "" {
			return "0", nil
		}
		return "1", nil
	case strings.HasPrefix(cmd, "/system scheduler add"):
		var err error
		r.event, err = strconv.Unquote(strings.SplitN(cmd, " on-event=", 2)[1])
		return "", err
	case strings.HasPrefix(cmd, "/system scheduler print value-list"):
		event := r.event
		if r.badGuard {
			event = "wrong deadline"
		}
		return "name: mm-voucher-" + graceTestID + "\non-event: " + event + "\ninterval: 10s\ndisabled: no", nil
	case strings.HasPrefix(cmd, "/ip hotspot user add"):
		r.user = fmt.Sprintf("name: AA:BB:CC:DD:EE:FE\nmac-address: AA:BB:CC:DD:EE:FE\nserver: guest1\ncomment: mega-magic:voucher:%s\nlimit-uptime: 1m\nlimit-bytes-total: 75000000\ndisabled: yes", graceTestID)
		if r.badQuota {
			r.user = strings.ReplaceAll(r.user, "75000000", "0")
		}
		return "", nil
	case strings.HasPrefix(cmd, "/ip hotspot user enable"):
		r.user = strings.ReplaceAll(r.user, "disabled: yes", "disabled: no")
		if r.badActivation {
			r.user = strings.ReplaceAll(r.user, "75000000", "0")
		}
		return "", nil
	case strings.HasPrefix(cmd, ":if ([:len [/ip hotspot user find"):
		r.user = strings.ReplaceAll(r.user, "disabled: no", "disabled: yes")
		return "", nil
	default:
		return "", fmt.Errorf("unexpected command: %s", cmd)
	}
}

func voucherFixture() (*voucherRunner, GuestVoucherAccess) {
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	return &voucherRunner{now: now, login: "http-chap,mac"}, GuestVoucherAccess{VoucherID: graceTestID, ClientMAC: "AA:BB:CC:DD:EE:FE", Minutes: 1, DataMB: 75, ExpiresAt: now.Add(time.Minute)}
}

func TestGuestVoucherGuardBeforeActivationAndRetryPreservesUsage(t *testing.T) {
	r, g := voucherFixture()
	if err := configureGuestVoucher(context.Background(), r, g, r.now); err != nil {
		t.Fatal(err)
	}
	commands := strings.Join(r.commands, "\n")
	guardAt, createAt, enableAt := strings.Index(commands, "/system scheduler add"), strings.Index(commands, "/ip hotspot user add"), strings.Index(commands, "/ip hotspot user enable")
	if guardAt < 0 || createAt < guardAt || enableAt < createAt || !strings.Contains(commands, "limit-uptime=60s limit-bytes-total=75000000 disabled=yes") {
		t.Fatal("guard and verified disabled user must precede activation")
	}
	if !strings.Contains(r.event, fmt.Sprintf("%ds", g.ExpiresAt.Unix())) {
		t.Fatal("guard lost the fixed epoch deadline")
	}
	r.commands = nil
	if err := configureGuestVoucher(context.Background(), r, g, r.now); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range r.commands {
		if strings.Contains(cmd, " user add ") || strings.Contains(cmd, " user set ") || strings.Contains(cmd, " user enable ") || strings.Contains(cmd, " scheduler add ") {
			t.Fatal("retry changed entitlement or reset usage")
		}
	}
}

func TestGuestVoucherFailuresNeverReportReady(t *testing.T) {
	for _, failure := range []string{"clock-skew", "no-mac-login", "foreign-user", "guard-readback", "quota-readback", "deadline-elapsed", "activation-readback", "disabled-user"} {
		t.Run(failure, func(t *testing.T) {
			r, g := voucherFixture()
			now := r.now
			switch failure {
			case "clock-skew":
				r.now = now.Add(-time.Minute)
			case "no-mac-login":
				r.login = "http-chap"
			case "foreign-user":
				r.user = "name: AA:BB:CC:DD:EE:FE\ncomment: owner-account"
			case "guard-readback":
				r.badGuard = true
			case "quota-readback":
				r.badQuota = true
			case "deadline-elapsed":
				r.expireDuringSetup = true
			case "activation-readback":
				r.badActivation = true
			case "disabled-user":
				if err := configureGuestVoucher(context.Background(), r, g, now); err != nil {
					t.Fatal(err)
				}
				r.user = strings.ReplaceAll(r.user, "disabled: no", "disabled: yes")
				r.commands = nil
			}
			if err := configureGuestVoucher(context.Background(), r, g, now); err == nil {
				t.Fatal("unsafe allowance accepted")
			}
			if failure == "activation-readback" {
				if !strings.Contains(r.user, "disabled: yes") {
					t.Fatal("uncertain activation was not contained")
				}
			} else {
				for _, cmd := range r.commands {
					if strings.Contains(cmd, " user enable ") {
						t.Fatal("unsafe user enabled")
					}
				}
			}
		})
	}
}

func TestGuestVoucherActiveRetryPastDeadlineIsContained(t *testing.T) {
	r, g := voucherFixture()
	if err := configureGuestVoucher(context.Background(), r, g, r.now); err != nil {
		t.Fatal(err)
	}
	r.commands = nil
	r.clockReads = 0
	r.expireDuringSetup = true
	if err := configureGuestVoucher(context.Background(), r, g, r.now); err == nil {
		t.Fatal("expired active retry reported ready")
	}
	if !strings.Contains(r.user, "disabled: yes") {
		t.Fatal("expired owned user was not contained")
	}
	for _, cmd := range r.commands {
		if strings.Contains(cmd, " user enable ") || strings.Contains(cmd, " user add ") {
			t.Fatal("expired retry granted access")
		}
	}
}
