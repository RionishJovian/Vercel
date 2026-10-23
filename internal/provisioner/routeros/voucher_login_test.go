package routeros

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type voucherLoginRunner struct {
	servers  string // count-only of enabled hotspot servers
	server   string
	users    map[string]string // name -> value-list detail text
	active   map[string]string // user -> mac-address
	guard    *voucherGuardRunner
	clock    string
	commands []string
}

func (r *voucherLoginRunner) guardRunner() *voucherGuardRunner {
	if r.guard == nil {
		clock := r.clock
		if clock == "" {
			clock = guardCurrentClock()
		}
		r.guard = &voucherGuardRunner{clock: clock, scripts: map[string]string{}, schedulers: map[string]string{}, profiles: map[string]string{}}
	}
	return r.guard
}

func (r *voucherLoginRunner) Run(ctx context.Context, command string) (string, error) {
	r.commands = append(r.commands, command)
	switch {
	case command == ":put ([:tonsec [:timestamp]] / 1000000000)" || strings.HasPrefix(command, "/system script ") ||
		strings.HasPrefix(command, "/system scheduler ") || strings.HasPrefix(command, "/ip hotspot user profile "):
		return r.guardRunner().Run(ctx, command)
	case strings.HasPrefix(command, "/ip hotspot print count-only where disabled=no"):
		return r.servers, nil
	case strings.HasPrefix(command, "/ip hotspot print value-list detail where disabled=no"):
		return "name: " + r.server + "\n", nil
	case strings.HasPrefix(command, "/ip hotspot user print value-list detail where name="):
		name := strings.Trim(strings.TrimPrefix(command, "/ip hotspot user print value-list detail where name="), "\"")
		body, ok := r.users[name]
		if !ok {
			return "", nil
		}
		return "name: " + name + "\n" + body, nil
	case strings.HasPrefix(command, "/ip hotspot user add name="):
		name := command[len("/ip hotspot user add name="):]
		if i := strings.Index(name, " password="); i >= 0 {
			name = name[:i]
		}
		name = strings.Trim(name, "\"")
		if _, exists := r.users[name]; exists {
			return "", errors.New("user exists")
		}
		body := "name: " + name + "\n"
		for _, field := range []string{"server", "profile", "comment", "limit-uptime", "limit-bytes-total", "disabled"} {
			prefix := field + "="
			if i := strings.Index(command, " "+prefix); i >= 0 {
				rest := command[i+len(prefix)+1:]
				value := rest
				if j := strings.Index(rest, " "); j >= 0 {
					value = rest[:j]
				}
				body += field + ": " + strings.Trim(value, "\"") + "\n"
			}
		}
		r.users[name] = body
		return "", nil
	case strings.HasPrefix(command, "/ip hotspot active find user="):
		user := strings.Trim(strings.TrimPrefix(command, "/ip hotspot active find user="), "\"")
		if _, ok := r.active[user]; ok {
			return "1", nil
		}
		return "0", nil
	}
	// The sync script is a single composite command.
	if strings.HasPrefix(command, "{ :put \"__MM_VS__") {
		return voucherSyncScriptOutput(r), nil
	}
	return "", errors.New("unexpected command: " + command)
}

func voucherSyncScriptOutput(r *voucherLoginRunner) string {
	var b strings.Builder
	for _, id := range []string{"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"} {
		code := map[string]string{"11111111-1111-4111-8111-111111111111": "ABCD1234", "22222222-2222-4222-8222-222222222222": "WXYZ5678"}[id]
		b.WriteString("__MM_VS__" + id + "__start\n")
		body, ok := r.users[code]
		if !ok {
			b.WriteString("absent\n__MM_VS__" + id + "__end\n")
			continue
		}
		disabled := "no"
		if strings.Contains(body, "disabled: yes") {
			disabled = "yes"
		}
		b.WriteString("comment=mega-magic:vlogin:" + id + "\n")
		b.WriteString("disabled=" + disabled + "\n")
		b.WriteString("uptime=" + voucherSyncUptime(body) + "\n")
		b.WriteString("bytes-in=1024\nbytes-out=2048\n")
		if mac, active := r.active[code]; active {
			b.WriteString("mac=" + mac + "\n")
		} else {
			b.WriteString("mac=none\n")
		}
		b.WriteString("__MM_VS__" + id + "__end\n")
	}
	return b.String()
}

func voucherSyncUptime(body string) string {
	if strings.Contains(body, "limit-uptime: 60m") {
		return "30m"
	}
	return "0s"
}

func voucherTestLogins() []VoucherLogin {
	return []VoucherLogin{
		{VoucherID: "11111111-1111-4111-8111-111111111111", Code: "ABCD1234", Minutes: 60, DataMB: 256},
		{VoucherID: "22222222-2222-4222-8222-222222222222", Code: "WXYZ5678", Minutes: 60, DataMB: 256},
	}
}

func TestProvisionVoucherLoginsCreatesAndVerifies(t *testing.T) {
	r := &voucherLoginRunner{servers: "1", server: "hs1", users: map[string]string{}, active: map[string]string{}}
	if err := provisionVoucherLogins(context.Background(), r, voucherTestLogins()); err != nil {
		t.Fatalf("provision failed: %v", err)
	}
	for _, l := range voucherTestLogins() {
		body, ok := r.users[l.Code]
		if !ok {
			t.Fatalf("router user for %s was not created", l.Code)
		}
		if !strings.Contains(body, "limit-uptime: 3600s") || !strings.Contains(body, "limit-bytes-total: 256000000") {
			t.Errorf("user %s missing limits: %q", l.Code, body)
		}
	}
	// Retries are idempotent: the same batch must succeed without changes.
	if err := provisionVoucherLogins(context.Background(), r, voucherTestLogins()); err != nil {
		t.Fatalf("retry failed: %v", err)
	}
}

func TestProvisionVoucherLoginsRejectsBadInputAndForeignUsers(t *testing.T) {
	r := &voucherLoginRunner{servers: "1", server: "hs1", users: map[string]string{}, active: map[string]string{}}
	for _, bad := range [][]VoucherLogin{
		{},
		{{VoucherID: "11111111-1111-4111-8111-111111111111", Code: "lower case", Minutes: 60, DataMB: 256}},
		{{VoucherID: "11111111-1111-4111-8111-111111111111", Code: "ABCD1234", Minutes: 0, DataMB: 256}},
		{{VoucherID: "11111111-1111-4111-8111-111111111111", Code: "ABCD1234", Minutes: 60, DataMB: 0}},
		{{VoucherID: "not-a-uuid", Code: "ABCD1234", Minutes: 60, DataMB: 256}},
		{{VoucherID: "11111111-1111-4111-8111-111111111111", Code: "ABCD1234", Minutes: 60, DataMB: 256}, {VoucherID: "22222222-2222-4222-8222-222222222222", Code: "ABCD1234", Minutes: 60, DataMB: 256}},
	} {
		if err := provisionVoucherLogins(context.Background(), r, bad); err == nil {
			t.Errorf("expected rejection for %+v", bad)
		}
	}
	// Two enabled hotspot servers must be refused.
	r.servers = "2"
	if err := provisionVoucherLogins(context.Background(), r, voucherTestLogins()); err == nil {
		t.Error("expected rejection with two enabled hotspot servers")
	}
	r.servers = "1"
	// A foreign user occupying a code must be refused.
	r.users["ABCD1234"] = "server: other\nprofile: default\ncomment: someone-else\nlimit-uptime: 1m\nlimit-bytes-total: 1\ndisabled: no\n"
	if err := provisionVoucherLogins(context.Background(), r, voucherTestLogins()); err == nil {
		t.Error("expected rejection for a foreign user on the code")
	}
}

func TestProvisionVoucherLoginsAdoptsStampedUsersAndUsesOwnedProfile(t *testing.T) {
	r := &voucherLoginRunner{servers: "1", server: "hs1", users: map[string]string{}, active: map[string]string{}}
	if err := provisionVoucherLogins(context.Background(), r, voucherTestLogins()); err != nil {
		t.Fatalf("provision failed: %v", err)
	}
	for _, l := range voucherTestLogins() {
		if body := r.users[l.Code]; !strings.Contains(body, "profile: "+voucherGuardProfile) {
			t.Errorf("user %s not on the versioned owned profile: %q", l.Code, body)
		}
	}
	// A retry after the guard stamped first-use evidence adopts the same
	// allowance: the anchor is immutable and never reset by provisioning.
	r.users["ABCD1234"] = "server: hs1\nprofile: " + voucherGuardProfile + "\ncomment: mega-magic:vlogin:11111111-1111-4111-8111-111111111111;ft=1792941234;fu=02:ab:cd:ef:12:34\nlimit-uptime: 3600s\nlimit-bytes-total: 256000000\ndisabled: no\n"
	if err := provisionVoucherLogins(context.Background(), r, voucherTestLogins()); err != nil {
		t.Fatalf("stamped retry failed: %v", err)
	}
	if !strings.Contains(r.users["ABCD1234"], ";ft=1792941234") {
		t.Fatal("retry must not touch the persisted first-use anchor")
	}
	// An unknown profile on the same allowance is somebody else's user.
	r.users["ABCD1234"] = "server: hs1\nprofile: someone-else\ncomment: mega-magic:vlogin:11111111-1111-4111-8111-111111111111\nlimit-uptime: 3600s\nlimit-bytes-total: 256000000\ndisabled: no\n"
	if err := provisionVoucherLogins(context.Background(), r, voucherTestLogins()); err == nil {
		t.Fatal("expected rejection for a foreign profile on the code")
	}
}

func TestVoucherLegacyMigrationRefusesUsedUnanchoredBeforeUserWrite(t *testing.T) {
	l := voucherTestLogins()[0]
	body := "server: hs1\nprofile: default\ncomment: " + voucherLoginPrefix + l.VoucherID + "\nlimit-uptime: 3600s\nlimit-bytes-total: 256000000\ndisabled: no\nuptime: 60s\nbytes-in: 100\n"
	r := &voucherLoginRunner{servers: "1", server: "hs1", users: map[string]string{l.Code: body}, active: map[string]string{}}
	var writes []string
	wrapper := runnerFunc(func(ctx context.Context, cmd string) (string, error) {
		if strings.HasPrefix(cmd, "/ip hotspot user set ") || strings.HasPrefix(cmd, "/ip hotspot user add ") {
			writes = append(writes, cmd)
		}
		return r.Run(ctx, cmd)
	})
	err := provisionVoucherLogins(context.Background(), wrapper, []VoucherLogin{l})
	if err == nil || !strings.Contains(err.Error(), "Manual review required") || len(writes) != 0 || r.users[l.Code] != body {
		t.Fatalf("used legacy voucher changed before refusal: err=%v writes=%v body=%q", err, writes, r.users[l.Code])
	}
}

func TestVoucherLegacyMigrationReadbackRequiresOwnedProfileAndServer(t *testing.T) {
	l := voucherTestLogins()[0]
	for _, tc := range []struct {
		name, profile, server string
		wantErr               bool
	}{
		{"verified", voucherGuardProfile, "hs1", false},
		{"ignored write", "default", "hs1", true},
		{"wrong server", voucherGuardProfile, "other", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := runnerFunc(func(_ context.Context, cmd string) (string, error) {
				if strings.HasPrefix(cmd, "/ip hotspot user set ") {
					return "", nil
				}
				return "name: " + l.Code + "\nserver: " + tc.server + "\nprofile: " + tc.profile + "\ncomment: " + voucherLoginPrefix + l.VoucherID + "\nlimit-uptime: 3600s\nlimit-bytes-total: 256000000\ndisabled: no\n", nil
			})
			if err := migrateVoucherLoginProfile(context.Background(), r, l, "hs1"); (err != nil) != tc.wantErr {
				t.Fatalf("readback err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestReadVoucherLoginsParsesRouterAccounting(t *testing.T) {
	r := &voucherLoginRunner{servers: "1", server: "hs1", users: map[string]string{
		"ABCD1234": "server: hs1\nprofile: default\ncomment: mega-magic:vlogin:11111111-1111-4111-8111-111111111111\nlimit-uptime: 60m\ndisabled: no\n",
	}, active: map[string]string{"ABCD1234": "aa:bb:cc:dd:ee:ff"}}
	states, err := readVoucherLogins(context.Background(), r, voucherTestLogins())
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if len(states) != 2 {
		t.Fatalf("expected 2 states, got %d", len(states))
	}
	byID := map[string]VoucherLoginState{}
	for _, s := range states {
		byID[s.VoucherID] = s
	}
	used := byID["11111111-1111-4111-8111-111111111111"]
	if !used.Exists || used.Disabled || used.UptimeSeconds != 1800 || used.BytesUsed != 3072 || used.ActiveMAC != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("11111111-1111-4111-8111-111111111111 state wrong: %+v", used)
	}
	if s := byID["22222222-2222-4222-8222-222222222222"]; s.Exists {
		t.Errorf("22222222-2222-4222-8222-222222222222 must be absent on the router: %+v", s)
	}
	// A disabled voucher reports disabled.
	r.users["ABCD1234"] = strings.Replace(r.users["ABCD1234"], "disabled: no", "disabled: yes", 1)
	states, err = readVoucherLogins(context.Background(), r, voucherTestLogins())
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	for _, s := range states {
		if s.VoucherID == "11111111-1111-4111-8111-111111111111" && !s.Disabled {
			t.Errorf("11111111-1111-4111-8111-111111111111 must report disabled: %+v", s)
		}
	}
	// Truncated router output must fail closed, not invent accounting.
	r2 := &voucherLoginRunner{servers: "1", server: "hs1", users: map[string]string{}, active: map[string]string{}}
	short := func(ctx context.Context, command string) (string, error) {
		return "__MM_VS__11111111-1111-4111-8111-111111111111__start\n", nil
	}
	_ = r2
	if _, err := readVoucherLogins(context.Background(), runnerFunc(short), voucherTestLogins()); err == nil {
		t.Error("truncated router output must be rejected")
	}
}

type runnerFunc func(ctx context.Context, command string) (string, error)

func (f runnerFunc) Run(ctx context.Context, command string) (string, error) { return f(ctx, command) }
