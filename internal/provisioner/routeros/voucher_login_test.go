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
	commands []string
}

func (r *voucherLoginRunner) Run(ctx context.Context, command string) (string, error) {
	r.commands = append(r.commands, command)
	switch {
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
			b.WriteString("absent\n")
			continue
		}
		disabled := "no"
		if strings.Contains(body, "disabled: yes") {
			disabled = "yes"
		}
		b.WriteString("disabled=" + disabled + "\n")
		b.WriteString("uptime-used=" + voucherSyncUptime(body) + "\n")
		b.WriteString("bytes-in=1024\nbytes-out=2048\n")
		if mac, active := r.active[code]; active {
			b.WriteString("mac=" + mac + "\n")
		} else {
			b.WriteString("mac=none\n")
		}
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
