package routeros

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// Runs the HotSpot wizard against a disposable CHR, with a port inside a bridge as
// on a factory router, a terminal left open, and a port with no link. See docs/CHR-LAB.md.
// The app connects as its own account, as on a real router, so an admin's console
// login counts as somebody else's terminal.

type labSource struct{ user string }

func (s labSource) TargetForDevice(context.Context, string, string) (*DeviceTarget, error) {
	return &DeviceTarget{DeviceID: "lab", Addr: "192.168.64.4:22", User: s.user, Secret: "unused", Kind: "password", Fingerprint: "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}, nil
}

// failOn makes one command fail, to prove rollback on a real router.
type failOn struct {
	CommandRunner
	prefix string
}

func (f failOn) Run(ctx context.Context, cmd string) (string, error) {
	if strings.HasPrefix(cmd, f.prefix) {
		return "", errors.New("simulated failure")
	}
	return f.CommandRunner.Run(ctx, cmd)
}

func TestCHRLabWizardOnAFactoryStylePort(t *testing.T) {
	target := os.Getenv("MM_CHR_SSH")
	if target == "" {
		t.Skip("set MM_CHR_SSH to run against a disposable CHR")
	}
	key := os.Getenv("MM_CHR_KEY")
	pub, err := os.ReadFile(key + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	admin := chrLab{t, chrRunner{target}, ctx}
	host := target[strings.Index(target, "@")+1:]
	raw := make([]byte, 24)
	if _, err = rand.Read(raw); err != nil {
		t.Fatal(err)
	}

	tidy := func() {
		for _, cmd := range []string{
			`/ip hotspot remove [find name~"^mm-labfx"]`, `/ip hotspot profile remove [find name~"^mm-labfx"]`,
			`/ip dhcp-server remove [find name~"^mm-labfx"]`, `/ip dhcp-server network remove [find comment~"^mm-labfx"]`,
			`/ip firewall nat remove [find comment~"^mm-labfx"]`, `/ip firewall filter remove [find comment~"^mm-labfx"]`,
			`/ip address remove [find comment~"^mm-labfx"]`, `/ip pool remove [find name~"^mm-labfx"]`,
			`/interface bridge port remove [find interface=labfx0]`, `/interface macvlan remove [find name=labfx0]`,
			`/interface bridge remove [find name=labfxbr]`, `/interface macvlan remove [find name=labfxq]`, `/interface bridge remove [find name=labfxdown]`,
			`/user ssh-keys remove [find user=mmlab]`, `/user remove [find name=mmlab]`,
		} {
			_, _ = admin.r.Run(context.Background(), cmd)
		}
	}
	tidy()
	defer tidy()
	admin.put(`/user add name=mmlab group=full password="` + hex.EncodeToString(raw) + `"`)
	admin.put(`/user ssh-keys add user=mmlab key="` + strings.TrimSpace(string(pub)) + `"`)
	admin.put(`/interface bridge add name=labfxbr`)
	admin.put(`/interface macvlan add name=labfx0 interface=ether1 mode=bridge`)
	admin.put(`/interface bridge port add bridge=labfxbr interface=labfx0 comment=defconf`)
	// A macvlan on a disabled bridge has no link, like a Wi-Fi AP with no client. RouterOS
	// refuses a DHCP server on a tunnel such as GRE, so a tunnel can't stand in for this.
	admin.put(`/interface bridge add name=labfxdown disabled=yes`)
	admin.put(`/interface macvlan add name=labfxq interface=labfxdown mode=bridge`)

	app := chrRunner{"mmlab@" + host}
	var runner CommandRunner = app
	a := NewDeviceApplier(labSource{"mmlab"}, time.Minute, func(SSHConfig) CommandRunner { return runner }, DefaultInterface, 0, 0)
	setup := func(lan, name string, hotspot bool) GatewaySetup {
		return GatewaySetup{Name: name, WAN: "ether1", LAN: lan, Address: "10.84.0.1/24", PoolStart: "10.84.0.10", PoolEnd: "10.84.0.50", DNS: []string{"1.1.1.1"}, Hotspot: hotspot}
	}
	count := func(cmd string) string { return admin.put(cmd + " print count-only") }

	t.Run("an open terminal doesn't block setup", func(t *testing.T) {
		all := admin.put(`/system script job print count-only`)
		mine := strings.TrimSpace(mustRun(t, ctx, app, activeJobsCommand("mmlab")))
		t.Logf("unscoped=%s scoped=%s", all, mine)
		if all == "1" {
			t.Fatal("this test needs a login open on the CHR's console")
		}
		if mine != "1" {
			t.Fatalf("the app's own command should be the only job counted: %s", mine)
		}
	})

	t.Run("a failed step puts the port back", func(t *testing.T) {
		g := setup("labfx0", "labfxrb", true)
		plan, e := a.PreviewGateway(ctx, "b", "lab", g)
		if e != nil {
			t.Fatal(e)
		}
		g.Confirmation = plan.Confirmation
		runner = failOn{app, "/ip hotspot add"}
		result, e := a.ApplyGateway(ctx, "b", "lab", g)
		runner = app
		if e == nil || !result.RolledBack {
			t.Fatalf("%+v %v", result, e)
		}
		if got := admin.put(`:put [/interface bridge port get [find interface=labfx0] bridge]`); got != "labfxbr" {
			t.Fatalf("port not back in its bridge: %q", got)
		}
		if got := admin.put(`:put [/interface bridge port get [find interface=labfx0] comment]`); got != "defconf" {
			t.Fatalf("comment not restored: %q", got)
		}
		if n := count(`/ip dhcp-server`); strings.Contains(admin.put(`/ip dhcp-server print terse where name~"labfx"`), "labfx") {
			t.Fatalf("leftovers: %s", n)
		}
	})

	t.Run("a real job still blocks setup", func(t *testing.T) {
		g := setup("labfx0", "labfxjob", false)
		plan, e := a.PreviewGateway(ctx, "b", "lab", g)
		if e != nil {
			t.Fatal(e)
		}
		g.Confirmation = plan.Confirmation
		admin.put(`:execute script=":delay 25s"`)
		time.Sleep(time.Second)
		_, e = a.ApplyGateway(ctx, "b", "lab", g)
		if e == nil || !strings.Contains(e.Error(), "active script job") {
			t.Fatalf("a running script must block setup: %v", e)
		}
		if got := admin.put(`:put [/interface bridge port get [find interface=labfx0] bridge]`); got != "labfxbr" {
			t.Fatalf("a refused setup changed the bridge: %q", got)
		}
		time.Sleep(26 * time.Second)
	})

	t.Run("a factory-style port leaves its bridge and works", func(t *testing.T) {
		g := setup("labfx0", "labfx", true)
		plan, e := a.PreviewGateway(ctx, "b", "lab", g)
		if e != nil {
			t.Fatal(e)
		}
		t.Logf("first step: %s", plan.Steps[0])
		if !strings.Contains(plan.Steps[0], "labfxbr") {
			t.Fatalf("%v", plan.Steps)
		}
		g.Confirmation = plan.Confirmation
		result, e := a.ApplyGateway(ctx, "b", "lab", g)
		if e != nil || !result.Verified {
			t.Fatalf("%+v %v", result, e)
		}
		if strings.Contains(admin.put(`/interface bridge port print terse where interface=labfx0`), "labfx0") {
			t.Fatal("port is still in the bridge")
		}
		t.Logf("hotspot: %s", admin.put(`/ip hotspot print terse where name="mm-labfx"`))
		if a2, e := a.PreviewGateway(ctx, "b", "lab", setup("labfx0", "labfx2", true)); e == nil {
			t.Fatalf("a configured port must be refused: %+v", a2)
		}
	})
	admin.put(`/ip hotspot remove [find name~"^mm-labfx"]`)
	admin.put(`/ip hotspot profile remove [find name~"^mm-labfx"]`)
	admin.put(`/ip dhcp-server remove [find name~"^mm-labfx"]`)
	admin.put(`/ip dhcp-server network remove [find comment~"^mm-labfx"]`)
	admin.put(`/ip firewall nat remove [find comment~"^mm-labfx"]`)
	admin.put(`/ip address remove [find comment~"^mm-labfx"]`)
	admin.put(`/ip pool remove [find name~"^mm-labfx"]`)

	t.Run("a port with no link is accepted, with a warning", func(t *testing.T) {
		g := setup("labfxq", "labfxq", true)
		plan, e := a.PreviewGateway(ctx, "b", "lab", g)
		if e != nil {
			t.Fatal(e)
		}
		t.Logf("steps: %v", plan.Steps)
		if !strings.Contains(strings.Join(plan.Steps, " "), "first client") {
			t.Fatalf("no warning: %v", plan.Steps)
		}
		g.Confirmation = plan.Confirmation
		result, e := a.ApplyGateway(ctx, "b", "lab", g)
		if e != nil || !result.Verified {
			t.Fatalf("%+v %v", result, e)
		}
		invalid := func(menu string) string { return admin.put(`:put [` + menu + ` get [find name="mm-labfxq"] invalid]`) }
		if invalid("/ip dhcp-server") != "true" || invalid("/ip hotspot") != "true" {
			t.Fatal("expected both to wait for a link")
		}
		admin.put(`/interface bridge enable [find name=labfxdown]`)
		time.Sleep(6 * time.Second)
		if invalid("/ip dhcp-server") != "false" || invalid("/ip hotspot") != "false" {
			t.Fatal("both should start by themselves when the link comes up")
		}
	})
}

func mustRun(t *testing.T, ctx context.Context, r CommandRunner, cmd string) string {
	t.Helper()
	out, err := r.Run(ctx, cmd)
	if err != nil {
		t.Fatalf("%s: %v", cmd, err)
	}
	return out
}

func TestCHRLabConnectedSessionsAndBackup(t *testing.T) {
	target := os.Getenv("MM_CHR_SSH")
	if target == "" {
		t.Skip("set MM_CHR_SSH to run against a disposable CHR")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	runner := chrRunner{target}
	a := NewDeviceApplier(labSource{"admin"}, time.Minute, func(SSHConfig) CommandRunner { return runner }, DefaultInterface, 0, 0)

	inventory, err := a.ReadNetwork(ctx, "b", "lab")
	if err != nil {
		t.Fatal(err)
	}
	if !inventory.Available["ppp_active"] {
		t.Fatal("the connected-clients read failed on a real router")
	}
	sessions := inventory.Objects["ppp_active"]
	if len(sessions) == 0 {
		t.Fatal("this test needs a PPPoE client connected on the CHR")
	}
	for _, s := range sessions {
		t.Logf("session: %v", s.Values)
		if s.Values["name"] == "" || s.Values["service"] != "pppoe" || s.Values["address"] == "" || s.Values["uptime"] == "" {
			t.Fatalf("missing fields: %v", s.Values)
		}
	}

	before := strings.TrimSpace(mustRun(t, ctx, runner, `/export`))
	snap, err := a.Snapshot(ctx, "b", "lab", nil)
	if err != nil {
		t.Fatalf("Capture backup failed on a real router: %v", err)
	}
	content, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("a backup saves %d bytes; the router export is %d bytes", len(content), len(before))
	t.Logf("backup content: %.400s", content)
	if strings.TrimSpace(mustRun(t, ctx, runner, `/export`)) == "" || before == "" {
		t.Fatal("router export unreadable")
	}
}
