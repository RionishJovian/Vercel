package routeros

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"mega-magic/internal/provisioner"
)

// fakeRunner is a CommandRunner that records calls and returns canned output
// keyed by a substring match (first match wins in map iteration, so tests use
// disjoint keys).
type fakeRunner struct {
	calls []string
	out   map[string]string
	err   map[string]error
}

func (f *fakeRunner) Run(_ context.Context, cmd string) (string, error) {
	f.calls = append(f.calls, cmd)
	for k, e := range f.err {
		if strings.Contains(cmd, k) {
			return f.out[k], e
		}
	}
	for k, o := range f.out {
		if strings.Contains(cmd, k) {
			return o, nil
		}
	}
	return "", nil
}

func (f *fakeRunner) joined() string { return strings.Join(f.calls, "\n") }

func TestTranslate_actions(t *testing.T) {
	a := NewApplier(&fakeRunner{}, "wg-magichub", 13241, 0)

	cases := []struct {
		action  string
		wantCmd []string
		wantNil bool // no device command
	}{
		{provisioner.ActionEnsureKeepalive, []string{"/interface wireguard peers set [find interface=wg-magichub] persistent-keepalive=25s"}, false},
		{provisioner.ActionRotateListenPort, []string{"/interface wireguard set [find name=wg-magichub] listen-port=13241"}, false},
		{provisioner.ActionResetPeer, []string{"disable [find interface=wg-magichub]", "enable [find interface=wg-magichub]"}, false},
		{provisioner.ActionReprobe, nil, true},
		{provisioner.ActionEscalate, nil, true},
		{provisioner.ActionReboot, []string{"/system reboot"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			steps, err := a.Translate(provisioner.Change{Action: tc.action})
			if err != nil {
				t.Fatalf("Translate: %v", err)
			}
			var cmds []string
			for _, s := range steps {
				if s.Command != "" {
					cmds = append(cmds, s.Command)
				}
			}
			if tc.wantNil {
				if len(cmds) != 0 {
					t.Fatalf("action %q should not touch the device, got %v", tc.action, cmds)
				}
				return
			}
			got := strings.Join(cmds, "\n")
			for _, want := range tc.wantCmd {
				if !strings.Contains(got, want) {
					t.Fatalf("action %q cmds = %q, want substring %q", tc.action, got, want)
				}
			}
		})
	}
}

func TestTranslate_failsClosed(t *testing.T) {
	// rotate without a target port.
	if _, err := NewApplier(&fakeRunner{}, "wg-magichub", 0, 0).Translate(provisioner.Change{Action: provisioner.ActionRotateListenPort}); err == nil {
		t.Fatal("rotate_listen_port without a target port must fail closed")
	}
	// unknown action.
	if _, err := NewApplier(&fakeRunner{}, "wg-magichub", 13241, 0).Translate(provisioner.Change{Action: "delete_everything"}); err == nil {
		t.Fatal("unknown action must be refused")
	}
	if _, err := NewApplier(&fakeRunner{}, "wg-magichub", 13241, 0).Translate(provisioner.Change{Action: provisioner.ActionHardenManagement, Proposed: `{"trusted_cidrs":["0.0.0.0/999"]}`}); err == nil {
		t.Fatal("harden_management must reject an invalid management CIDR")
	}
}

func TestTranslate_hardenManagement(t *testing.T) {
	a := NewApplier(&fakeRunner{}, "wg-magichub", 0, 0)
	steps, err := a.Translate(provisioner.Change{Action: provisioner.ActionHardenManagement, Proposed: `{"trusted_cidrs":["10.99.0.0/24","10.77.0.9/32"]}`})
	if err != nil {
		t.Fatal(err)
	}
	var commands []string
	for _, step := range steps {
		commands = append(commands, step.Command)
	}
	joined := strings.Join(commands, "\n")
	for _, want := range []string{
		"name=telnet]", "name=ftp]", "name=www]", "name=api]", "name=api-ssl]",
		"name=ssh] address=10.99.0.0/24,10.77.0.9/32 disabled=no",
		"name=winbox] address=10.99.0.0/24,10.77.0.9/32 disabled=no",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("hardening commands missing %q in:\n%s", want, joined)
		}
	}
}

func TestApply_resetOrderAndError(t *testing.T) {
	fr := &fakeRunner{}
	a := NewApplier(fr, "wg-magichub", 13241, 0)
	if err := a.Apply(context.Background(), "biz-1", "peer2", []provisioner.Change{{Action: provisioner.ActionResetPeer}}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	joined := fr.joined()
	di := strings.Index(joined, "disable")
	ei := strings.Index(joined, "enable")
	if di < 0 || ei < 0 || di > ei {
		t.Fatalf("reset_peer must disable before enable: %q", joined)
	}

	// A runner error must propagate.
	bad := &fakeRunner{err: map[string]error{"disable": errors.New("ssh refused")}}
	if err := NewApplier(bad, "wg-magichub", 0, 0).Apply(context.Background(), "biz-1", "peer2", []provisioner.Change{{Action: provisioner.ActionResetPeer}}); err == nil {
		t.Fatal("runner error must propagate from Apply")
	}
}

func TestSnapshot_populatesFields(t *testing.T) {
	fr := &fakeRunner{out: map[string]string{
		"print value-list detail where name=":            "         name: wg-magichub\n  listen-port: 13231\n",
		"peers print value-list detail where interface=": "persistent-keepalive: 25s\n   allowed-address: 10.99.0.1/32\n",
		"/ip route print detail":                         "DAc 10.99.0.0/30 wg-magichub main 0\n",
	}}
	a := NewApplier(fr, "wg-magichub", 13241, 0)
	snap, err := a.Snapshot(context.Background(), "biz-1", "peer2", []provisioner.Change{{Action: provisioner.ActionRotateListenPort}})
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if !strings.Contains(fr.joined(), "/export file=mm-snap-peer2-") {
		t.Fatalf("snapshot must export to file: %q", fr.joined())
	}
	if !strings.Contains(fr.joined(), "/system backup save name=mm-snap-peer2-") {
		t.Fatalf("snapshot must save a backup: %q", fr.joined())
	}
	if snap.ExportFile == "" || !strings.HasSuffix(snap.ExportFile, ".rsc") {
		t.Fatalf("ExportFile = %q", snap.ExportFile)
	}
	if snap.WGListenPort != 13231 {
		t.Fatalf("WGListenPort = %d, want 13231", snap.WGListenPort)
	}
	if snap.WGPeerKeepalive != "25s" {
		t.Fatalf("WGPeerKeepalive = %q", snap.WGPeerKeepalive)
	}
	if snap.WGPeerAllowedAddress != "10.99.0.1/32" {
		t.Fatalf("WGPeerAllowedAddress = %q", snap.WGPeerAllowedAddress)
	}
	if len(snap.IPRoutes) == 0 {
		t.Fatal("routes must be captured")
	}
}

func TestSnapshotAndRestore_managementServices(t *testing.T) {
	fr := &fakeRunner{out: map[string]string{
		"print value-list detail where name=":            "name: wg-magichub\nlisten-port: 13231\n",
		"peers print value-list detail where interface=": "persistent-keepalive: 25s\nallowed-address: 10.99.0.1/32\n",
		"/ip route print detail":                         "DAc 10.99.0.0/30 wg-magichub main 0\n",
		"/ip service print value-list detail":            "name: telnet\naddress:\ndisabled: false\n\nname: ssh\naddress: 192.168.99.0/24\ndisabled: false\n\nname: api\naddress:\ndisabled: true\n",
	}}
	a := NewApplier(fr, "wg-magichub", 0, 0)
	snap, err := a.Snapshot(context.Background(), "biz-1", "peer2", []provisioner.Change{{Action: provisioner.ActionHardenManagement}})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.RouterOSServices) != 3 {
		t.Fatalf("services = %#v", snap.RouterOSServices)
	}
	if err := a.Restore(context.Background(), "biz-1", snap); err != nil {
		t.Fatal(err)
	}
	joined := fr.joined()
	for _, want := range []string{
		`name=telnet] address="" disabled=no`,
		"name=ssh] address=192.168.99.0/24 disabled=no",
		`name=api] address="" disabled=yes`,
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("restore missing %q in:\n%s", want, joined)
		}
	}
}

func TestSnapshot_nilRunner(t *testing.T) {
	if _, err := NewApplier(nil, "wg-magichub", 0, 0).Snapshot(context.Background(), "biz-1", "p", nil); err == nil {
		t.Fatal("nil runner must error")
	}
}

func TestRestore_revertsAndReenables(t *testing.T) {
	fr := &fakeRunner{}
	a := NewApplier(fr, "wg-magichub", 13241, 0)
	snap := provisioner.RollbackSnapshot{
		PeerID:               "peer2",
		WGListenPort:         13231,
		WGPeerKeepalive:      "25s",
		WGPeerAllowedAddress: "10.99.0.1/32",
	}
	if err := a.Restore(context.Background(), "biz-1", snap); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	joined := fr.joined()
	for _, want := range []string{
		"/interface wireguard set [find name=wg-magichub] listen-port=13231",
		"persistent-keepalive=25s",
		"allowed-address=10.99.0.1/32",
		"/interface wireguard peers enable [find interface=wg-magichub]",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("restore missing %q in:\n%s", want, joined)
		}
	}
}

func TestSnapshotName_sanitizes(t *testing.T) {
	got := snapshotName("ab/cd:ef 12", time.Date(2026, 9, 25, 11, 0, 0, 0, time.UTC))
	if strings.ContainsAny(got, "/: ") {
		t.Fatalf("snapshot name not sanitized: %q", got)
	}
	if !strings.HasPrefix(got, "mm-snap-ab-cd-ef-12-20260925-110000") {
		t.Fatalf("snapshot name = %q", got)
	}
	if snapshotName("", time.Now()) == "" {
		t.Fatal("empty peer should still yield a name")
	}
}
