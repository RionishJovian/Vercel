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
		if cmd != gatewayAddressesCommand && cmd != gatewayRoutesCommand && cmd != gatewayPoolsCommand {
			t.Fatalf("suggestions mutated router: %s", cmd)
		}
	}
}

func TestGatewayInterfacesScalarRecords(t *testing.T) {
	r := &gatewayRunner{interfaces: "name: ether1\r\ntype: ether\r\nrunning: true\r\nname: ether2\r\ntype: ether\r\nrunning: false\r\n"}
	interfaces, err := quickConfigApplier(r).GatewayInterfaces(context.Background(), "biz-1", "dev-1")
	if err != nil || len(interfaces) != 2 || interfaces[0].Name != "ether1" || !interfaces[0].Running || interfaces[1].Running {
		t.Fatalf("interfaces=%+v error=%v", interfaces, err)
	}
	if len(r.commands) != 1 || r.commands[0] != gatewayInterfacesCommand {
		t.Fatal("interface discovery must use a fixed read-only scalar command")
	}
	r.interfaces = "name: ether1 ether2"
	if _, err := quickConfigApplier(r).GatewayInterfaces(context.Background(), "biz-1", "dev-1"); err == nil {
		t.Fatal("silently accepted unusable columnar output")
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

func TestGatewayDefaultsReserveUnroutedSubscriberPool(t *testing.T) {
	r := &gatewayRunner{objects: map[string]map[string]string{"/ip pool": {"ranges": "192.168.50.10-192.168.50.200"}}}
	g, err := quickConfigApplier(r).GatewayDefaults(context.Background(), "biz-1", "dev-1")
	if err != nil || g.Address != "192.168.51.1/24" {
		t.Fatalf("unrouted subscriber pool reused: %+v %v", g, err)
	}
	if _, err = quickConfigApplier(r).PreviewGateway(context.Background(), "biz-1", "dev-1", validGatewaySetup()); err == nil {
		t.Fatal("gateway preview reused an existing subscriber pool")
	}
	for _, ranges := range []string{"192.168.50.200-192.168.50.10", "bad-range", "192.168.50.10-192.168.50.200-extra"} {
		r.objects["/ip pool"]["ranges"] = ranges
		if _, err = quickConfigApplier(r).GatewayDefaults(context.Background(), "biz-1", "dev-1"); err == nil {
			t.Fatalf("malformed pool accepted: %q", ranges)
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
