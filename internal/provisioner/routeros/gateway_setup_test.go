package routeros

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
)

type gatewayRunner struct {
	commands  []string
	objects   map[string]map[string]string
	failMenu  string
	addresses string
}

var gatewayTestFields = regexp.MustCompile(`([a-z-]+)="([^"\\]*)"`)

func (r *gatewayRunner) Run(_ context.Context, cmd string) (string, error) {
	r.commands = append(r.commands, cmd)
	if strings.HasPrefix(cmd, "/export ") || strings.HasPrefix(cmd, "/system backup ") {
		return "", nil
	}
	if cmd == "/ip address print value-list detail" {
		return r.addresses, nil
	}
	if strings.HasPrefix(cmd, "/interface print count-only") {
		return "1", nil
	}
	menu := strings.Split(cmd, " print")[0]
	if strings.Contains(cmd, " print count-only") {
		if len(r.objects[menu]) > 0 {
			return "1", nil
		}
		return "0", nil
	}
	if strings.Contains(cmd, " add ") {
		menu = strings.Split(cmd, " add ")[0]
		if menu == r.failMenu {
			return "", errors.New("simulated timeout")
		}
		fields := map[string]string{}
		for _, m := range gatewayTestFields.FindAllStringSubmatch(cmd, -1) {
			fields[m[1]] = m[2]
		}
		r.objects[menu] = fields
		return "", nil
	}
	if strings.Contains(cmd, " print value-list") {
		text := ""
		for k, v := range r.objects[menu] {
			text += fmt.Sprintf("%s: %s\n", k, v)
		}
		return text, nil
	}
	if strings.Contains(cmd, " remove ") {
		delete(r.objects, strings.Split(cmd, " remove ")[0])
		return "", nil
	}
	if strings.Contains(cmd, " move ") {
		return "", nil
	}
	return "", fmt.Errorf("unexpected command: %s", cmd)
}

func validGatewaySetup() GatewaySetup {
	return GatewaySetup{Name: "guest", WAN: "ether1", LAN: "ether2", Address: "192.168.50.1/24", PoolStart: "192.168.50.10", PoolEnd: "192.168.50.200", DNS: []string{"1.1.1.1"}, Hotspot: true}
}

func TestGatewaySetupPreviewApplyVerifyAndRejectDuplicate(t *testing.T) {
	r := &gatewayRunner{objects: map[string]map[string]string{}}
	a := quickConfigApplier(r)
	g := validGatewaySetup()
	plan, e := a.PreviewGateway(context.Background(), "biz-1", "dev-1", g)
	if e != nil {
		t.Fatal(e)
	}
	for _, cmd := range r.commands {
		if strings.Contains(cmd, " add ") {
			t.Fatal("preview mutated router")
		}
	}
	g.Confirmation = plan.Confirmation
	result, e := a.ApplyGateway(context.Background(), "biz-1", "dev-1", g)
	if e != nil || !result.Verified || result.Backup == "" || result.RolledBack {
		t.Fatalf("result=%+v error=%v", result, e)
	}
	if r.objects["/ip firewall nat"]["src-address"] != "192.168.50.0/24" || r.objects["/ip hotspot profile"]["login-by"] != "http-chap,mac" {
		t.Fatal("missing scoped NAT or grace login method")
	}
	if _, e = a.ApplyGateway(context.Background(), "biz-1", "dev-1", g); e == nil {
		t.Fatal("duplicate setup was accepted")
	}
}

func TestGatewaySetupRollbackAfterPartialWrite(t *testing.T) {
	r := &gatewayRunner{objects: map[string]map[string]string{}, failMenu: "/ip hotspot"}
	a := quickConfigApplier(r)
	g := validGatewaySetup()
	plan, e := a.PreviewGateway(context.Background(), "biz-1", "dev-1", g)
	if e != nil {
		t.Fatal(e)
	}
	g.Confirmation = plan.Confirmation
	result, e := a.ApplyGateway(context.Background(), "biz-1", "dev-1", g)
	if e == nil || !result.RolledBack || len(r.objects) > 0 {
		t.Fatalf("rollback failed: result=%+v objects=%v error=%v", result, r.objects, e)
	}
}

func TestGatewaySetupRefusesOverlapInjectionAndForeignTenant(t *testing.T) {
	r := &gatewayRunner{objects: map[string]map[string]string{}, addresses: "address: 192.168.50.2/24"}
	a := quickConfigApplier(r)
	g := validGatewaySetup()
	if _, e := a.PreviewGateway(context.Background(), "biz-1", "dev-1", g); e == nil {
		t.Fatal("overlapping subnet accepted")
	}
	r.commands = nil
	if _, e := a.PreviewGateway(context.Background(), "other-tenant", "dev-1", g); e == nil || len(r.commands) > 0 {
		t.Fatal("foreign tenant reached router")
	}
	for _, field := range []string{"name", "pool", "gateway", "interface"} {
		bad := validGatewaySetup()
		switch field {
		case "name":
			bad.Name = `guest; /system reboot`
		case "pool":
			bad.PoolStart = "192.168.50.1"
		case "gateway":
			bad.Address = "8.8.8.8/24"
		case "interface":
			bad.LAN = bad.WAN
		}
		if _, _, e := normalizeGatewaySetup(bad); e == nil {
			t.Fatalf("invalid %s accepted", field)
		}
	}
}

func TestGatewaySetupSecretNotExposedAndPreviewBinding(t *testing.T) {
	g := validGatewaySetup()
	g.VLAN = 50
	g.RadiusAddress = "10.0.0.10"
	g.RadiusSecret = "secret-for-test-only"
	g, network, e := normalizeGatewaySetup(g)
	if e != nil {
		t.Fatal(e)
	}
	plan, steps := gatewaySteps(g, network)
	if strings.Contains(fmt.Sprintf("%+v", plan), g.RadiusSecret) || len(steps) != 10 || plan.Interface != "mm-guest-vlan" {
		t.Fatal("invalid VLAN/RADIUS plan or leaked secret")
	}
	r := &gatewayRunner{objects: map[string]map[string]string{}}
	g.Confirmation = plan.Confirmation
	g.PoolEnd = "192.168.50.100"
	if _, e := quickConfigApplier(r).ApplyGateway(context.Background(), "biz-1", "dev-1", g); e == nil || len(r.commands) > 0 {
		t.Fatal("changed preview reached router")
	}
}
