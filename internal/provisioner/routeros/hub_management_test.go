package routeros

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

type hubTestRunner struct {
	*networkRunner
	first string
}

func (r *hubTestRunner) Run(ctx context.Context, cmd string) (string, error) {
	if cmd == `:put [:pick [/ip firewall filter find] 0]` {
		return r.first, nil
	}
	out, e := r.networkRunner.Run(ctx, cmd)
	if strings.HasPrefix(cmd, "/ip firewall filter add ") {
		r.first = fmt.Sprintf("*%X", r.next)
		r.objects["filters"][r.first]["dynamic"] = "no"
	}
	return out, e
}
func TestHubFirewallOwnedToggleAndRollback(t *testing.T) {
	ctx := context.Background()
	r := &hubTestRunner{networkRunner: networkFixture()}
	a := quickConfigApplier(r)
	req := NetworkRequest{Kind: "firewall", Values: map[string]string{"interface": "ether1", "disabled": "no"}}
	review, e := a.PreviewNetwork(ctx, "biz-1", "dev-1", req)
	if e != nil {
		t.Fatal(e)
	}
	if len(r.objects["filters"]) != 0 {
		t.Fatal("preview wrote router")
	}
	req.Confirmation = review.Confirmation
	result, e := a.ApplyNetwork(ctx, "biz-1", "dev-1", req)
	if e != nil || !result.Verified {
		t.Fatalf("%+v %v", result, e)
	}
	req.ID = r.first
	req.Values["disabled"] = "yes"
	req.Confirmation = ""
	review, e = a.PreviewNetwork(ctx, "biz-1", "dev-1", req)
	if e != nil {
		t.Fatal(e)
	}
	req.Confirmation = review.Confirmation
	r.failWrite = true
	result, e = a.ApplyNetwork(ctx, "biz-1", "dev-1", req)
	if e == nil || !result.RolledBack || r.objects["filters"][req.ID]["disabled"] != "no" {
		t.Fatalf("rollback failed: %+v %v", result, e)
	}
	r.objects["filters"][req.ID]["action"] = "accept"
	req.Confirmation = ""
	if _, e = a.PreviewNetwork(ctx, "biz-1", "dev-1", req); e == nil {
		t.Fatal("adopted altered firewall")
	}
}
func TestHubShieldRequiresExplicitUnmanagedBypassAndRejectsDrift(t *testing.T) {
	ctx := context.Background()
	r := networkFixture()
	a := quickConfigApplier(r)
	r.objects["bindings"]["*A"] = map[string]string{"type": "bypassed", "mac-address": "02:11:22:33:44:55", "address": "192.0.2.40", "server": "guest", "comment": "Printer", "disabled": "no"}
	req := NetworkRequest{Kind: "shield", ID: "*A", Values: map[string]string{"disabled": "yes"}}
	review, e := a.PreviewNetwork(ctx, "biz-1", "dev-1", req)
	if e != nil {
		t.Fatal(e)
	}
	r.objects["bindings"]["*A"]["mac-address"] = "02:11:22:33:44:66"
	req.Confirmation = review.Confirmation
	if _, e = a.ApplyNetwork(ctx, "biz-1", "dev-1", req); e == nil || r.objects["bindings"]["*A"]["disabled"] != "no" {
		t.Fatal("stale client identity accepted")
	}
	r.objects["bindings"]["*A"]["comment"] = "mm-grace"
	req.Confirmation = ""
	if _, e = a.PreviewNetwork(ctx, "biz-1", "dev-1", req); e == nil {
		t.Fatal("changed managed exemption")
	}
	r.objects["bindings"]["*A"]["comment"] = "Printer"
	review, e = a.PreviewNetwork(ctx, "biz-1", "dev-1", req)
	if e != nil {
		t.Fatal(e)
	}
	req.Confirmation = review.Confirmation
	result, e := a.ApplyNetwork(ctx, "biz-1", "dev-1", req)
	if e != nil || !result.Verified || r.objects["bindings"]["*A"]["disabled"] != "yes" {
		t.Fatalf("%+v %v", result, e)
	}
	req.Values["disabled"] = "no"
	if _, _, e := buildNetworkPlan(ctx, r, req); e == nil {
		t.Fatal("enabled bypass")
	}
}
func TestHubFairshareLifecycleAndConflicts(t *testing.T) {
	ctx := context.Background()
	r := networkFixture()
	a := quickConfigApplier(r)
	req := NetworkRequest{Kind: "fairshare", Values: map[string]string{"name": "guests", "target": "192.0.2.0/24", "max-limit": "10M/50M", "disabled": "no"}}
	review, e := a.PreviewNetwork(ctx, "biz-1", "dev-1", req)
	if e != nil {
		t.Fatal(e)
	}
	if len(r.objects["queues"]) != 0 || len(r.objects["queue_types"]) != 0 {
		t.Fatal("preview mutated")
	}
	req.Confirmation = review.Confirmation
	result, e := a.ApplyNetwork(ctx, "biz-1", "dev-1", req)
	if e != nil || !result.Verified {
		t.Fatalf("%+v %v", result, e)
	}
	for id, q := range r.objects["queues"] {
		req.ID = id
		if q["queue"] != "mm-fair-guests-up/mm-fair-guests-down" {
			t.Fatal("wrong PCQ direction")
		}
	}
	if len(r.objects["queue_types"]) != 2 {
		t.Fatal("missing independent classifiers")
	}
	req.Values["max-limit"] = "5M/20M"
	req.Confirmation = ""
	review, e = a.PreviewNetwork(ctx, "biz-1", "dev-1", req)
	if e != nil {
		t.Fatal(e)
	}
	req.Confirmation = review.Confirmation
	result, e = a.ApplyNetwork(ctx, "biz-1", "dev-1", req)
	if e != nil || !result.Verified {
		t.Fatalf("edit %+v %v", result, e)
	}
	req.Confirmation = ""
	req.Values["max-limit"] = "6M/20M"
	r.objects["filters"]["*F"] = map[string]string{"action": "fasttrack-connection", "disabled": "no"}
	if _, _, e := buildNetworkPlan(ctx, r, req); e == nil {
		t.Fatal("accepted active FastTrack")
	}
	delete(r.objects["filters"], "*F")
	r.objects["queues"]["*E"] = map[string]string{"target": "0.0.0.0/0", "disabled": "no"}
	if _, _, e := buildNetworkPlan(ctx, r, req); e == nil {
		t.Fatal("accepted conflicting queue")
	}
	delete(r.objects["queues"], "*E")
	req.Kind = "queue"
	if _, _, e := buildNetworkPlan(ctx, r, req); e == nil {
		t.Fatal("generic editor bypassed PCQ ownership")
	}
}
func TestHubFairshareFailureRestoresAllAdditions(t *testing.T) {
	r := networkFixture()
	a := quickConfigApplier(r)
	ctx := context.Background()
	req := NetworkRequest{Kind: "fairshare", Values: map[string]string{"name": "guests", "target": "192.0.2.0/24", "max-limit": "10M/50M", "disabled": "no"}}
	review, e := a.PreviewNetwork(ctx, "biz-1", "dev-1", req)
	if e != nil {
		t.Fatal(e)
	}
	req.Confirmation = review.Confirmation
	r.failWrite = true
	result, e := a.ApplyNetwork(ctx, "biz-1", "dev-1", req)
	if e == nil || !result.RolledBack || len(r.objects["queue_types"]) != 0 || len(r.objects["queues"]) != 0 {
		t.Fatalf("%+v %v", result, e)
	}
}

func TestHubProtectionPreservesForeignRulesAndRejectsConflictingMarker(t *testing.T) {
	r := &hubTestRunner{networkRunner: networkFixture(), first: "*A"}
	ctx := context.Background()
	foreign := map[string]string{"chain": "input", "action": "accept", "comment": "Owner management", "disabled": "no", "dynamic": "no"}
	r.objects["filters"]["*A"] = foreign
	req := NetworkRequest{Kind: "firewall", Values: map[string]string{"interface": "ether1", "disabled": "no"}}
	steps, _, e := buildNetworkPlan(ctx, r, req)
	if e != nil {
		t.Fatal(e)
	}
	if len(steps) != 1 || !strings.Contains(networkWriteCommand(steps[0], false), "place-before=0") {
		t.Fatal("existing policy ordering ignored")
	}
	if foreign["action"] != "accept" || foreign["disabled"] != "no" {
		t.Fatal("foreign rule mutated")
	}
	r.objects["filters"]["*B"] = map[string]string{"comment": "mm-protection-invalid-ether1"}
	if _, _, e := buildNetworkPlan(ctx, r, req); e == nil {
		t.Fatal("reserved name collision adopted")
	}
}
func TestHubFairshareRejectsUnsafeTargetAndForeignType(t *testing.T) {
	for _, target := range []string{"0.0.0.0/0", "10.77.0.0/24", "192.0.2.1/24", "198.18.0.0/24", "192.0.2.40/32"} {
		r := networkFixture()
		req := NetworkRequest{Kind: "fairshare", Values: map[string]string{"name": "guests", "target": target, "max-limit": "10M/50M", "disabled": "no"}}
		if _, _, e := buildNetworkPlan(context.Background(), r, req); e == nil {
			t.Fatalf("accepted target %s", target)
		}
	}
	r := networkFixture()
	r.objects["queue_types"]["*A"] = map[string]string{"name": "mm-fair-guests-up", "kind": "pcq", "pcq-classifier": "dst-address", "pcq-rate": "0"}
	req := NetworkRequest{Kind: "fairshare", Values: map[string]string{"name": "guests", "target": "192.0.2.0/24", "max-limit": "10M/50M", "disabled": "no"}}
	if _, _, e := buildNetworkPlan(context.Background(), r, req); e == nil {
		t.Fatal("foreign PCQ type adopted")
	}
	if len(r.objects["queues"]) != 0 || len(r.objects["queue_types"]) != 1 {
		t.Fatal("rejected preview mutated router")
	}
}
func TestHubHealthProjectionFailsClosed(t *testing.T) {
	ctx := context.Background()
	for _, out := range []string{"cpu-load: 1\n", "cpu-load: 1\npassword: SECRET\n", strings.Repeat("x", 4097)} {
		r := commandRunnerFunc(func(context.Context, string) (string, error) { return out, nil })
		if readHubHealth(ctx, r) != nil {
			t.Fatal("untrusted health accepted")
		}
	}
}

func TestHubOverviewDoesNotReadFullConfigurationOrSecrets(t *testing.T) {
	r := networkFixture()
	a := quickConfigApplier(r)
	result, e := a.ReadRouterOverview(context.Background(), "biz-1", "dev-1")
	if e != nil || len(result.Interfaces) == 0 {
		t.Fatalf("%+v %v", result, e)
	}
	for _, cmd := range r.commands {
		if strings.Contains(cmd, "/ppp secret") || strings.Contains(cmd, "/ip firewall") || strings.Contains(cmd, "/queue") {
			t.Fatal("overview read a configuration menu")
		}
	}
}
