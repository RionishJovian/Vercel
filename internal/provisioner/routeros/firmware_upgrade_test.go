package routeros

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

const firmwareOld = "model: C53UiG+5HPaxD2HPaxD\ncurrent-firmware: 7.6\nupgrade-firmware: 7.24.5\ninstalled-version: 7.24.5\nlatest-version: 7.24.5\nchannel: stable\n"

type firmwareRouter struct {
	firmware string
	jobs     string
	fail     map[string]bool
	log      []string
}

func (f *firmwareRouter) Run(_ context.Context, command string) (string, error) {
	f.log = append(f.log, command)
	for prefix := range f.fail {
		if strings.HasPrefix(command, prefix) {
			return "private detail", errors.New("boom")
		}
	}
	switch {
	case command == TelemetryFirmwareCommand:
		return f.firmware, nil
	case command == "/system script job print count-only":
		return f.jobs, nil
	case command == firmwareRebootCommand:
		return "*1F\n", nil
	}
	return "", nil
}
func (f *firmwareRouter) wrote() []string {
	var out []string
	for _, c := range f.log {
		if c != TelemetryFirmwareCommand && c != "/system script job print count-only" {
			out = append(out, c)
		}
	}
	return out
}
func firmwareApplier(r *firmwareRouter) *DeviceApplier {
	return NewDeviceApplier(capabilitySource{}, time.Second, func(SSHConfig) CommandRunner { return r }, DefaultInterface, 0, 0)
}

func TestFirmwarePreviewIsReadOnlyAndShowsTheRealVersions(t *testing.T) {
	r := &firmwareRouter{firmware: firmwareOld, jobs: "1"}
	review, err := firmwareApplier(r).PreviewFirmware(context.Background(), "biz-1", "dev-1")
	if err != nil {
		t.Fatal(err)
	}
	if review.Current != "7.6" || review.Target != "7.24.5" || review.Confirmation == "" || len(review.Steps) != 3 || len(review.Warnings) < 3 {
		t.Fatalf("%+v", review)
	}
	if len(r.wrote()) != 0 {
		t.Fatalf("a preview changed the router: %v", r.wrote())
	}
	if !strings.Contains(strings.Join(review.Warnings, " "), "cannot be undone") || !strings.Contains(strings.Join(review.Warnings, " "), "power") {
		t.Fatal("the risks must be stated")
	}
}

func TestFirmwarePreviewRefusesWhatItCannotProve(t *testing.T) {
	cases := map[string]string{
		"already current": "current-firmware: 7.24.5\nupgrade-firmware: 7.24.5\ninstalled-version: 7.24.5\n",
		"older offered":   "current-firmware: 7.24.5\nupgrade-firmware: 7.6\ninstalled-version: 7.24.5\n",
		"newer than ROS":  "current-firmware: 7.6\nupgrade-firmware: 7.30\ninstalled-version: 7.24.5\n",
		"unreadable":      "current-firmware: abc\nupgrade-firmware: 7.24.5\n",
		"nothing":         "\n",
		"no upgrade":      "current-firmware: 7.6\n",
	}
	for name, out := range cases {
		r := &firmwareRouter{firmware: out, jobs: "1"}
		if _, err := firmwareApplier(r).PreviewFirmware(context.Background(), "biz-1", "dev-1"); err == nil {
			t.Errorf("%s: preview was offered", name)
		}
		if len(r.wrote()) != 0 {
			t.Errorf("%s: router changed", name)
		}
	}
	pending := &firmwareRouter{firmware: firmwareOld, jobs: "1"}
	networkPending.Store("biz-1:dev-1", networkPendingEntry{})
	defer networkPending.Delete("biz-1:dev-1")
	if _, err := firmwareApplier(pending).PreviewFirmware(context.Background(), "biz-1", "dev-1"); err == nil {
		t.Fatal("a held network change must block a firmware upgrade")
	}
}

func TestFirmwareApplyOrderBackupThenWriteThenRestart(t *testing.T) {
	r := &firmwareRouter{firmware: firmwareOld, jobs: "1"}
	a := firmwareApplier(r)
	review, _ := a.PreviewFirmware(context.Background(), "biz-1", "dev-1")
	result, err := a.ApplyFirmware(context.Background(), "biz-1", "dev-1", review.Confirmation)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Restarting || result.Current != "7.6" || result.Target != "7.24.5" || result.Backup == "" {
		t.Fatalf("%+v", result)
	}
	w := r.wrote()
	if len(w) != 4 || !strings.HasPrefix(w[0], "/export file=") || !strings.HasPrefix(w[1], "/system backup save name=") || w[2] != firmwareUpgradeCommand || w[3] != firmwareRebootCommand {
		t.Fatalf("wrong order or commands: %v", w)
	}
}

func TestFirmwareApplyRefusesStaleReusedOrForeignApprovals(t *testing.T) {
	ctx := context.Background()
	r := &firmwareRouter{firmware: firmwareOld, jobs: "1"}
	a := firmwareApplier(r)
	review, _ := a.PreviewFirmware(ctx, "biz-1", "dev-1")
	if _, err := a.ApplyFirmware(ctx, "biz-1", "dev-1", review.Confirmation); err != nil {
		t.Fatal(err)
	}
	before := len(r.wrote())
	if _, err := a.ApplyFirmware(ctx, "biz-1", "dev-1", review.Confirmation); err == nil {
		t.Fatal("an approval was used twice")
	}
	if _, err := a.ApplyFirmware(ctx, "biz-1", "dev-1", "not-a-token"); err == nil {
		t.Fatal("an unknown approval was accepted")
	}
	other, _ := a.PreviewFirmware(ctx, "biz-1", "dev-1")
	if _, err := a.ApplyFirmware(ctx, "biz-2", "dev-1", other.Confirmation); err == nil {
		t.Fatal("another business used this approval")
	}
	if len(r.wrote()) != before {
		t.Fatal("a refused approval still changed the router")
	}
	changed := &firmwareRouter{firmware: firmwareOld, jobs: "1"}
	b := firmwareApplier(changed)
	rev, _ := b.PreviewFirmware(ctx, "biz-1", "dev-1")
	changed.firmware = strings.Replace(firmwareOld, "upgrade-firmware: 7.24.5", "upgrade-firmware: 7.24.4", 1)
	if _, err := b.ApplyFirmware(ctx, "biz-1", "dev-1", rev.Confirmation); err == nil || len(changed.wrote()) != 0 {
		t.Fatalf("a changed router state was applied: %v %v", err, changed.wrote())
	}
}

func TestFirmwareApplyStopsAtTheFirstFailureAndSaysWhatHappened(t *testing.T) {
	ctx := context.Background()
	run := func(fail string, jobs string) (*firmwareRouter, error) {
		r := &firmwareRouter{firmware: firmwareOld, jobs: jobs, fail: map[string]bool{}}
		a := firmwareApplier(r)
		review, err := a.PreviewFirmware(ctx, "biz-1", "dev-1")
		if err != nil {
			t.Fatal(err)
		}
		if fail != "" {
			r.fail[fail] = true
		}
		_, err = a.ApplyFirmware(ctx, "biz-1", "dev-1", review.Confirmation)
		return r, err
	}
	r, err := run("/system backup save", "1")
	if err == nil || !strings.Contains(err.Error(), "not changed") {
		t.Fatalf("backup failure: %v", err)
	}
	for _, c := range r.wrote() {
		if c == firmwareUpgradeCommand || c == firmwareRebootCommand {
			t.Fatal("firmware written or router restarted after a failed backup")
		}
	}
	r, err = run(firmwareUpgradeCommand, "1")
	if err == nil || !strings.Contains(err.Error(), "not restarted") {
		t.Fatalf("write failure: %v", err)
	}
	for _, c := range r.wrote() {
		if c == firmwareRebootCommand {
			t.Fatal("router restarted after a refused firmware write")
		}
	}
	if _, err = run(firmwareRebootCommand, "1"); err == nil || !strings.Contains(err.Error(), "restart the router yourself") {
		t.Fatalf("restart failure: %v", err)
	}
	r, err = run("", "3")
	if err == nil || len(r.wrote()) != 0 {
		t.Fatalf("an active script job must stop the upgrade before any write: %v %v", err, r.wrote())
	}
	if err != nil && strings.Contains(err.Error(), "private detail") {
		t.Fatal("transport details leaked")
	}
}

func TestFirmwareApplyForgetsTheRetainedFirmwareStatus(t *testing.T) {
	ctx := context.Background()
	r := &firmwareRouter{firmware: firmwareOld, jobs: "1"}
	a := firmwareApplier(r)
	read := func() RouterTelemetry {
		result := baseTelemetry()
		a.collectExtras(ctx, r, "biz-1", "dev-1", &result)
		return result
	}
	if got := read().Firmware; got == nil || got.CurrentFirmware != "7.6" {
		t.Fatalf("setup: %+v", got)
	}
	review, _ := a.PreviewFirmware(ctx, "biz-1", "dev-1")
	if _, err := a.ApplyFirmware(ctx, "biz-1", "dev-1", review.Confirmation); err != nil {
		t.Fatal(err)
	}
	r.firmware = strings.Replace(strings.Replace(firmwareOld, "current-firmware: 7.6", "current-firmware: 7.24.5", 1), "upgrade-firmware: 7.24.5", "upgrade-firmware: 7.24.5", 1)
	if got := read().Firmware; got == nil || got.CurrentFirmware != "7.24.5" {
		t.Fatalf("the page would keep showing the old firmware for up to 30 minutes: %+v", got)
	}
}
