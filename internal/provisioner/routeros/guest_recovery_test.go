package routeros

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"mega-magic/internal/guestaccess"
)

type recoveryRunner struct {
	g              guestaccess.RecoveryClaim
	now            time.Time
	commands       []string
	users          map[string]map[string]string
	active, cookie bool
	guard          string
	fail           string
	clockReads     int
}

func recoveryRunnerFixture() (*recoveryRunner, guestaccess.RecoveryClaim) {
	now := time.Now().UTC().Truncate(time.Second)
	g := guestaccess.RecoveryClaim{BusinessID: "biz-1", DeviceID: "dev-1", VoucherID: graceTestID, TransferID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", OldMAC: "AA:BB:CC:DD:EE:FE", NewMAC: "AA:BB:CC:DD:EE:FC", TotalBytes: 75000000, ExpiresAt: now.Add(time.Hour)}
	u := map[string]string{"name": g.OldMAC, "mac-address": g.OldMAC, "server": "guest1", "profile": "default", "comment": recoveryComment(g), "limit-bytes-total": "75000000", "limit-uptime": "3600s", "disabled": "no"}
	return &recoveryRunner{g: g, now: now, users: map[string]map[string]string{g.OldMAC: u}, active: true, cookie: true}, g
}
func (r *recoveryRunner) Run(_ context.Context, cmd string) (string, error) {
	r.commands = append(r.commands, cmd)
	mac := r.g.OldMAC
	if strings.Contains(cmd, r.g.NewMAC) {
		mac = r.g.NewMAC
	}
	u := r.users[mac]
	switch {
	case cmd == ":put [:tonsec [:timestamp]]":
		r.clockReads++
		if r.fail == "clock-rollback" && r.clockReads > 1 {
			return strconv.FormatInt(r.now.Add(-time.Minute).UnixNano(), 10), nil
		}
		return strconv.FormatInt(r.now.UnixNano(), 10), nil
	case strings.HasPrefix(cmd, "/ip hotspot host print count-only"):
		return "1", nil
	case strings.HasPrefix(cmd, "/ip hotspot host print value-list"):
		if r.fail == "host" {
			return "mac-address: " + r.g.NewMAC + "\nserver: foreign", nil
		}
		return "mac-address: " + r.g.NewMAC + "\nserver: guest1", nil
	case strings.HasPrefix(cmd, "/ip hotspot user print count-only"):
		if strings.Contains(cmd, " disabled=no") {
			n := 0
			for _, user := range r.users {
				if user["comment"] == recoveryComment(r.g) && !recoveryDisabled(user) {
					n++
				}
			}
			return strconv.Itoa(n), nil
		}
		if u == nil || strings.Contains(cmd, "comment=") && u["comment"] != recoveryComment(r.g) {
			return "0", nil
		}
		return "1", nil
	case strings.HasPrefix(cmd, "/ip hotspot user print value-list"):
		var lines []string
		for k, v := range u {
			lines = append(lines, k+": "+v)
		}
		return strings.Join(lines, "\n"), nil
	case strings.HasPrefix(cmd, "/ip hotspot user disable"):
		u["disabled"] = "yes"
		return "", nil
	case strings.HasPrefix(cmd, "/ip hotspot active remove"):
		if r.fail != "disconnect" {
			r.active = false
		}
		return "", nil
	case strings.HasPrefix(cmd, "/ip hotspot cookie remove"):
		if r.fail != "cookies" {
			r.cookie = false
		}
		return "", nil
	case strings.HasPrefix(cmd, "/ip hotspot active print count-only"):
		if r.active {
			return "1", nil
		}
		return "0", nil
	case strings.HasPrefix(cmd, "/ip hotspot cookie print count-only"):
		if r.cookie {
			return "1", nil
		}
		return "0", nil
	case strings.HasPrefix(cmd, ":local id [/ip hotspot user find"):
		if r.active {
			return "", fmt.Errorf("usage read before disconnect")
		}
		if r.fail == "stats" {
			return "bytes-in: unknown\nbytes-out: 20000000", nil
		}
		return "bytes-in: 20000000\nbytes-out: 20000000", nil
	case strings.HasPrefix(cmd, "/ip hotspot print value-list"):
		return "profile: guest-profile", nil
	case strings.HasPrefix(cmd, "/ip hotspot profile print value-list"):
		return "login-by: http-chap,mac\nmac-auth-password: local-secret", nil
	case strings.HasPrefix(cmd, "/system scheduler print count-only"):
		if r.guard == "" {
			return "0", nil
		}
		return "1", nil
	case strings.HasPrefix(cmd, "/system scheduler add"):
		var err error
		r.guard, err = strconv.Unquote(strings.SplitN(cmd, " on-event=", 2)[1])
		return "", err
	case strings.HasPrefix(cmd, "/system scheduler print value-list"):
		event := r.guard
		if r.fail == "guard" {
			event = "invalid"
		}
		return "name: mm-recovery-" + r.g.TransferID + "\non-event: " + event + "\ninterval: 10s\ndisabled: no", nil
	case strings.HasPrefix(cmd, "/ip hotspot user add"):
		parts := strings.Fields(cmd)
		user := map[string]string{}
		for _, part := range parts {
			kv := strings.SplitN(part, "=", 2)
			if len(kv) != 2 {
				continue
			}
			v := kv[1]
			if strings.HasPrefix(v, `"`) {
				v, _ = strconv.Unquote(v)
			}
			user[kv[0]] = v
		}
		if r.fail == "quota" {
			user["limit-bytes-total"] = "75000000"
		}
		r.users[r.g.NewMAC] = user
		return "", nil
	case strings.HasPrefix(cmd, "/ip hotspot user enable"):
		u["disabled"] = "no"
		if r.fail == "activate" {
			u["limit-bytes-total"] = "0"
		}
		return "", nil
	default:
		return "", fmt.Errorf("unexpected command: %s", cmd)
	}
}
func TestRecoveryAdapterPreservesQuotaAndStopsBeforeMeasuring(t *testing.T) {
	r, g := recoveryRunnerFixture()
	used, err := stopRecoveryAndRead(context.Background(), r, g, r.now)
	if err != nil {
		t.Fatal(err)
	}
	if used != 40000000 || r.active || r.cookie || !recoveryDisabled(r.users[g.OldMAC]) {
		t.Fatal("original binding not fully stopped before usage checkpoint")
	}
	g.UsedBytes = used
	if err := activateRecovery(context.Background(), r, g, 35000000, r.now); err != nil {
		t.Fatal(err)
	}
	newUser := r.users[g.NewMAC]
	if newUser["limit-bytes-total"] != "35000000" || recoveryDisabled(newUser) || !recoveryDisabled(r.users[g.OldMAC]) {
		t.Fatal("data replenished or old binding remained active")
	}
	if !strings.Contains(r.guard, fmt.Sprintf("%ds", g.ExpiresAt.Unix())) {
		t.Fatal("expiry reset")
	}
	commands := strings.Join(r.commands, "\n")
	if strings.Index(commands, "active remove") > strings.Index(commands, ":local id [/ip hotspot user find") || strings.Index(commands, "scheduler add") > strings.Index(commands, "user enable") {
		t.Fatal("unsafe recovery order")
	}
	if strings.Contains(commands, "reset-counters") || strings.Contains(commands, "/ip hotspot ip-binding") {
		t.Fatal("recovery used counter reset or bypass")
	}
	if err := activateRecovery(context.Background(), r, g, 35000000, r.now); err == nil {
		t.Fatal("retry silently recreated user")
	}
}
func TestRecoveryAdapterRejectsForeignOrUnstoppedBinding(t *testing.T) {
	for _, failure := range []string{"foreign", "host", "disconnect", "cookies", "stats"} {
		t.Run(failure, func(t *testing.T) {
			r, g := recoveryRunnerFixture()
			r.fail = failure
			if failure == "foreign" {
				r.users[g.OldMAC]["comment"] = "unrelated"
			}
			if _, err := stopRecoveryAndRead(context.Background(), r, g, r.now); err == nil {
				t.Fatal("unsafe transfer accepted")
			}
			for _, cmd := range r.commands {
				if strings.Contains(cmd, "user enable") || strings.Contains(cmd, "user add") {
					t.Fatal("new access granted on failed stop")
				}
				if failure == "foreign" && strings.Contains(cmd, " disable ") {
					t.Fatal("foreign user mutated")
				}
			}
		})
	}
}
func TestRecoveryAdapterBadReadbackIsContained(t *testing.T) {
	for _, failure := range []string{"guard", "quota", "activate", "clock-rollback"} {
		t.Run(failure, func(t *testing.T) {
			r, g := recoveryRunnerFixture()
			if _, err := stopRecoveryAndRead(context.Background(), r, g, r.now); err != nil {
				t.Fatal(err)
			}
			g.UsedBytes = 40000000
			r.fail = failure
			if err := activateRecovery(context.Background(), r, g, 35000000, r.now); err == nil {
				t.Fatal("bad readback accepted")
			}
			if err := containRecovery(context.Background(), r, g); err != nil {
				t.Fatal(err)
			}
			for _, u := range r.users {
				if !recoveryDisabled(u) {
					t.Fatal("uncertain guest remains enabled")
				}
			}
		})
	}
}

func TestRecoveryAdapterRefusesStaleOriginalSession(t *testing.T) {
	for _, stale := range []string{"active", "cookie"} {
		t.Run(stale, func(t *testing.T) {
			r, g := recoveryRunnerFixture()
			if _, err := stopRecoveryAndRead(context.Background(), r, g, r.now); err != nil {
				t.Fatal(err)
			}
			g.UsedBytes = 40000000
			r.active = stale == "active"
			r.cookie = stale == "cookie"
			if err := activateRecovery(context.Background(), r, g, 35000000, r.now); err == nil {
				t.Fatal("stale original connection accepted")
			}
			for _, cmd := range r.commands {
				if strings.Contains(cmd, "user enable") {
					t.Fatal("replacement enabled while original access remained")
				}
			}
		})
	}
}
