package routeros

import (
	"context"
	"strings"
	"testing"
)

func TestGatewayDefaultsReadOnlyAndAvoidAssignedAndRoutedNetworks(t *testing.T) {
	r := &gatewayRunner{addresses: "address: 192.168.50.1/24\n", routes: "dst-address: 0.0.0.0/0\ndst-address: 192.168.51.0/24\n"}
	g, err := quickConfigApplier(r).GatewayDefaults(context.Background(), "biz-1", "dev-1")
	if err != nil || g.Address != "192.168.52.1/24" || g.PoolStart != "192.168.52.10" || g.PoolEnd != "192.168.52.250" || g.WAN != "" || g.LAN != "" {
		t.Fatalf("defaults=%+v error=%v", g, err)
	}
	for _, cmd := range r.commands {
		if cmd != gatewayAddressesCommand && cmd != gatewayRoutesCommand {
			t.Fatalf("suggestions mutated router: %s", cmd)
		}
	}
}

func TestGatewayDefaultsFailClosed(t *testing.T) {
	for _, routes := range []string{"unexpected output", "dst-address: invalid", "dst-address: 192.168.0.0/16\ndst-address: 10.0.0.0/8", "dst-address: 192.168.51.0/24\nunparsed record"} {
		r := &gatewayRunner{routes: routes}
		if _, err := quickConfigApplier(r).GatewayDefaults(context.Background(), "biz-1", "dev-1"); err == nil {
			t.Fatalf("accepted unparseable or fully reserved inventory: %q", routes)
		}
	}
}

func TestGatewayApplyRechecksRouteConflictBeforeWriting(t *testing.T) {
	r := &gatewayRunner{objects: map[string]map[string]string{}}
	a := quickConfigApplier(r)
	g := validGatewaySetup()
	plan, err := a.PreviewGateway(context.Background(), "biz-1", "dev-1", g)
	if err != nil {
		t.Fatal(err)
	}
	g.Confirmation = plan.Confirmation
	r.routes = "dst-address: 192.168.50.0/24"
	if _, err := a.ApplyGateway(context.Background(), "biz-1", "dev-1", g); err == nil {
		t.Fatal("accepted route added after preview")
	}
	for _, cmd := range r.commands {
		if strings.Contains(cmd, " add ") || strings.HasPrefix(cmd, "/system backup") {
			t.Fatalf("conflicting apply wrote to router: %s", cmd)
		}
	}
}
