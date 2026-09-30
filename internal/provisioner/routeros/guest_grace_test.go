package routeros

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

const graceTestID = "11111111-2222-3333-4444-555555555555"

type graceRunner struct {
	commands    []string
	login       string
	user        string
	badReadback bool
}

func (r *graceRunner) Run(_ context.Context, cmd string) (string, error) {
	r.commands = append(r.commands, cmd)
	switch {
	case strings.HasPrefix(cmd, "/ip hotspot host print"):
		return "mac-address: AA:BB:CC:DD:EE:FE\nserver: guest1\naddress: 192.168.88.10", nil
	case strings.HasPrefix(cmd, "/ip hotspot print"):
		return "name: guest1\nprofile: guest-profile", nil
	case strings.HasPrefix(cmd, "/ip hotspot profile print"):
		return "login-by: " + r.login, nil
	case strings.HasPrefix(cmd, "/ip hotspot user print"):
		return r.user, nil
	case cmd == "/system clock print":
		return "date: 2026-09-30\ntime: 23:59:00", nil
	case strings.HasPrefix(cmd, "/system scheduler print count-only"):
		return "1", nil
	case strings.HasPrefix(cmd, "/system scheduler "):
		return "", nil
	case strings.HasPrefix(cmd, "/ip hotspot user add"):
		r.user = fmt.Sprintf("name: AA:BB:CC:DD:EE:FE\nmac-address: AA:BB:CC:DD:EE:FE\nserver: guest1\ncomment: mega-magic:grace:%s\nlimit-uptime: 7m\nlimit-bytes-total: 75000000", graceTestID)
		if r.badReadback {
			r.user = strings.ReplaceAll(r.user, "75000000", "0")
		}
		return "", nil
	case strings.HasPrefix(cmd, "/ip hotspot user remove"):
		r.user = ""
		return "", nil
	default:
		return "", fmt.Errorf("unexpected command")
	}
}

func TestGuestGraceEnforcesBothLimitsAndDoesNotResetRetry(t *testing.T) {
	r := &graceRunner{login: "http-chap,mac"}
	g := GuestGrace{RequestID: graceTestID, ClientMAC: "AA:BB:CC:DD:EE:FE", Minutes: 7, DataMB: 75}
	if err := quickConfigApplier(r).ConfigureGuestGrace(context.Background(), "biz-1", "dev-1", g); err != nil {
		t.Fatal(err)
	}
	commands := strings.Join(r.commands, "\n")
	if !strings.Contains(commands, "limit-uptime=420s limit-bytes-total=75000000") || !strings.Contains(commands, `server="guest1"`) {
		t.Fatalf("limits/client binding missing: %s", commands)
	}
	if !strings.Contains(commands, "start-date=2026-10-01") || !strings.Contains(commands, "interval=0s policy=read,write") || !strings.Contains(commands, "/ip hotspot user disable") {
		t.Fatal("wall-clock expiry was not scheduled across midnight")
	}
	r.commands = nil
	if err := configureGuestGrace(context.Background(), r, g); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range r.commands {
		if strings.Contains(cmd, " user add ") || strings.Contains(cmd, " user set ") {
			t.Fatal("retry reset the allowance")
		}
	}
}

func TestGuestRejectionNeverRemovesForeignUser(t *testing.T) {
	r := &graceRunner{user: "name: AA:BB:CC:DD:EE:FE\ncomment: unrelated-owner-user"}
	if err := quickConfigApplier(r).DecideGuestAccess(context.Background(), "biz-1", "dev-1", graceTestID, "AA:BB:CC:DD:EE:FE", false, 0, 0); err == nil {
		t.Fatal("foreign user should be refused")
	}
	if len(r.commands) != 1 || !strings.Contains(r.commands[0], " user print ") {
		t.Fatalf("foreign user was mutated: %v", r.commands)
	}
}

func TestGuestApprovalGrantsPaidQuotaAndRetryDoesNotGrantTwice(t *testing.T) {
	initial := fmt.Sprintf("name: AA:BB:CC:DD:EE:FE\ncomment: mega-magic:grace:%s\nuptime: 30s\nbytes-in: 1000\nbytes-out: 2000", graceTestID)
	paid := fmt.Sprintf("name: AA:BB:CC:DD:EE:FE\ncomment: mega-magic:paid:%s\nlimit-uptime: 1h30s\nlimit-bytes-total: 1024003000", graceTestID)
	var commands []string
	user := initial
	r := commandRunnerFunc(func(_ context.Context, cmd string) (string, error) {
		commands = append(commands, cmd)
		switch {
		case strings.Contains(cmd, " user print "):
			return user, nil
		case strings.Contains(cmd, " user set "):
			if !strings.Contains(cmd, "limit-uptime=3630s limit-bytes-total=1024003000") || !strings.Contains(cmd, "disabled=no") {
				t.Fatalf("wrong purchased allowance: %s", cmd)
			}
			user = paid
			return "", nil
		case strings.Contains(cmd, " scheduler remove "), strings.Contains(cmd, " active remove "):
			return "", nil
		default:
			return "", fmt.Errorf("unexpected command")
		}
	})
	a := quickConfigApplier(r)
	if err := a.DecideGuestAccess(context.Background(), "biz-1", "dev-1", graceTestID, "AA:BB:CC:DD:EE:FE", true, 60, 1024); err != nil {
		t.Fatal(err)
	}
	commands = nil
	if err := a.DecideGuestAccess(context.Background(), "biz-1", "dev-1", graceTestID, "AA:BB:CC:DD:EE:FE", true, 60, 1024); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range commands {
		if strings.Contains(cmd, " user set ") {
			t.Fatal("retry granted the purchased quota twice")
		}
	}
}

func TestGuestGraceFailsClosedWithoutMACLoginOrOnForeignUser(t *testing.T) {
	for _, r := range []*graceRunner{{login: "http-chap"}, {login: "mac", user: "name: AA:BB:CC:DD:EE:FE\ncomment: unrelated-owner-user"}} {
		err := configureGuestGrace(context.Background(), r, GuestGrace{RequestID: graceTestID, ClientMAC: "AA:BB:CC:DD:EE:FE", Minutes: 7, DataMB: 75})
		if err == nil {
			t.Fatal("expected refusal")
		}
		for _, cmd := range r.commands {
			if strings.Contains(cmd, " user add ") || strings.Contains(cmd, " user remove ") || strings.Contains(cmd, " profile set ") {
				t.Fatalf("unexpected mutation: %s", cmd)
			}
		}
	}
}

func TestGuestGraceRemovesOnlyOwnUserWhenVerificationFails(t *testing.T) {
	r := &graceRunner{login: "mac", badReadback: true}
	err := configureGuestGrace(context.Background(), r, GuestGrace{RequestID: graceTestID, ClientMAC: "AA:BB:CC:DD:EE:FE", Minutes: 7, DataMB: 75})
	if err == nil || r.user != "" {
		t.Fatal("failed verification must remove the new temporary user")
	}
	last := r.commands[len(r.commands)-1]
	if !strings.Contains(last, `comment="mega-magic:grace:`+graceTestID+`"`) {
		t.Fatal("cleanup is not scoped to this purchase")
	}
}

func TestGuestGraceRejectsInvalidLimitsBeforeRouterLookup(t *testing.T) {
	for _, g := range []GuestGrace{
		{RequestID: graceTestID, ClientMAC: "AA:BB:CC:DD:EE:FE", Minutes: 7, DataMB: 0},
		{RequestID: graceTestID, ClientMAC: "bad; /system reboot", Minutes: 7, DataMB: 50},
		{RequestID: graceTestID, ClientMAC: "FF:FF:FF:FF:FF:FF", Minutes: 7, DataMB: 50},
	} {
		r := &graceRunner{}
		if err := quickConfigApplier(r).ConfigureGuestGrace(context.Background(), "biz-1", "dev-1", g); err == nil || len(r.commands) != 0 {
			t.Fatal("invalid input reached router")
		}
	}
	r := &graceRunner{login: "mac"}
	if err := quickConfigApplier(r).ConfigureGuestGrace(context.Background(), "other-tenant", "dev-1", GuestGrace{RequestID: graceTestID, ClientMAC: "AA:BB:CC:DD:EE:FE", Minutes: 7, DataMB: 50}); err == nil || len(r.commands) != 0 {
		t.Fatal("cross-tenant request reached router")
	}
}
