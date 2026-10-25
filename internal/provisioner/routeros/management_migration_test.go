package routeros

import (
	"context"
	"fmt"
	"mega-magic/internal/managementmigration"
	"strings"
	"testing"
)

type migrationRunner struct {
	*networkRunner
	key string
}

func (r *migrationRunner) Run(ctx context.Context, cmd string) (string, error) {
	if strings.HasPrefix(cmd, ":put [/interface wireguard get") {
		return r.key, nil
	}
	out, e := r.networkRunner.Run(ctx, cmd)
	if strings.HasPrefix(cmd, "/ip address add ") {
		r.objects["addresses"][fmt.Sprintf("*%X", r.next)]["dynamic"] = "no"
	}
	return out, e
}
func migrationFixture(t *testing.T) (*ManagementMigrationRouter, *migrationRunner, *managementmigration.Job, *DeviceTarget) {
	t.Helper()
	target := &DeviceTarget{DeviceID: "device", Addr: "10.77.0.2:22", User: "fixture", Secret: "fixture-never-returned", Kind: "password", Fingerprint: "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
	runner := &migrationRunner{networkFixture(), "fixture-public-key"}
	iface, marker := managementInterface("device")
	runner.objects["addresses"] = map[string]map[string]string{"*A": {"address": "10.77.0.2/32", "interface": iface, "comment": marker, "disabled": "no", "dynamic": "no"}}
	j := &managementmigration.Job{ID: "11111111-1111-4111-8111-111111111111", BusinessID: "biz", DeviceID: "device", OldAddress: "10.77.0.2", NewAddress: "10.77.0.3", RouterKey: runner.key}
	r, e := NewManagementMigrationRouter(context.Background(), migrationTarget{target}, []string{"10.77.0.0/24"}, "biz", "device", func(cfg SSHConfig) CommandRunner {
		if cfg.HostKeyFingerprint != target.Fingerprint || cfg.InsecureIgnoreHostKey {
			t.Fatal("pin lost")
		}
		return runner
	})
	if e != nil {
		t.Fatal(e)
	}
	return r, runner, j, target
}
func TestManagementRouterDualAddressAndVerifiedCleanup(t *testing.T) {
	r, runner, j, _ := migrationFixture(t)
	ctx := context.Background()
	if e := r.Inspect(ctx, j); e != nil {
		t.Fatal(e)
	}
	if e := r.Stage(ctx, j); e != nil {
		t.Fatal(e)
	}
	if len(runner.objects["addresses"]) != 2 || !runner.guard || j.Backup == "" {
		t.Fatal("old path or guard missing")
	}
	if e := r.VerifyNew(ctx, j); e != nil {
		t.Fatal(e)
	}
	if e := r.RetainNew(ctx, j); e != nil {
		t.Fatal(e)
	}
	if runner.guard {
		t.Fatal("timer still active")
	}
	if e := r.RemoveOld(ctx, j); e != nil {
		t.Fatal(e)
	}
	if len(runner.objects["addresses"]) != 1 {
		t.Fatal("cleanup incomplete")
	}
	if e := r.RemoveOld(ctx, j); e != nil {
		t.Fatal("cleanup retry not idempotent", e)
	}
	if e := r.VerifyNew(ctx, j); e != nil {
		t.Fatal(e)
	}
}
func TestManagementRouterRollbackOnlyRemovesOperationAlias(t *testing.T) {
	r, runner, j, _ := migrationFixture(t)
	ctx := context.Background()
	if e := r.Inspect(ctx, j); e != nil {
		t.Fatal(e)
	}
	if e := r.Stage(ctx, j); e != nil {
		t.Fatal(e)
	}
	runner.objects["addresses"]["*FF"] = map[string]string{"address": "192.168.1.1/24", "interface": "ether2", "comment": "Owner LAN", "disabled": "no", "dynamic": "no"}
	if e := r.Rollback(ctx, j); e != nil {
		t.Fatal(e)
	}
	if runner.objects["addresses"]["*A"] == nil || runner.objects["addresses"]["*FF"] == nil || len(runner.objects["addresses"]) != 2 {
		t.Fatal("rollback altered original or foreign address")
	}
}
func TestManagementRouterRejectsForeignIdentityAndAddressDrift(t *testing.T) {
	for _, kind := range []string{"new-key", "old-marker", "reused-old-id", "target-drift", "disallowed-new"} {
		t.Run(kind, func(t *testing.T) {
			r, runner, j, target := migrationFixture(t)
			ctx := context.Background()
			if e := r.Inspect(ctx, j); e != nil {
				t.Fatal(e)
			}
			if kind == "disallowed-new" {
				j.NewAddress = "192.168.99.1"
				if e := r.Inspect(ctx, j); e == nil {
					t.Fatal("outbound allowlist bypassed")
				}
				return
			}
			if e := r.Stage(ctx, j); e != nil {
				t.Fatal(e)
			}
			switch kind {
			case "new-key":
				runner.key = "foreign-key"
				if e := r.VerifyNew(ctx, j); e == nil {
					t.Fatal("foreign router accepted")
				}
			case "old-marker":
				runner.objects["addresses"]["*A"]["comment"] = "foreign"
				if e := r.VerifyOld(ctx, j); e == nil {
					t.Fatal("foreign address adopted")
				}
			case "reused-old-id":
				runner.objects["addresses"]["*A"]["address"] = "192.168.88.1/24"
				if e := r.RemoveOld(ctx, j); e == nil {
					t.Fatal("reused ID deleted")
				}
			case "target-drift":
				target.Addr = "10.77.0.9:22"
				if e := r.VerifyNew(ctx, j); e == nil {
					t.Fatal("saved target drift ignored")
				}
			}
		})
	}
}
