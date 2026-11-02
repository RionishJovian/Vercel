package routeros

import (
	"context"
	"sort"
	"strings"
	"testing"
)

// A believable small router: an internet uplink on ether1 (DHCP client and default
// route), a customer bridge with DHCP, the Hub tunnel, one bridged port and free ports.
func addressFixture() *networkRunner {
	r := networkFixture()
	r.ifaces = map[string]string{"ether1": "ether", "ether2": "ether", "ether3": "ether", "ether4": "ether", "bridge": "bridge", "mmh-abc": "wireguard", "lo": "loopback", "vlan10": "vlan", "bond1": "bond"}
	r.bridgePorts = map[string]bool{"ether2": true}
	r.objects["addresses"] = map[string]map[string]string{
		"*10": {"address": "192.168.88.1/24", "interface": "bridge", "comment": "", "disabled": "no", "dynamic": "no"},
		"*11": {"address": "10.77.0.8/32", "interface": "mmh-abc", "comment": "mega-magic", "disabled": "no", "dynamic": "no"},
		"*12": {"address": "203.0.113.2/30", "interface": "ether1", "comment": "", "disabled": "no", "dynamic": "yes"},
		"*13": {"address": "192.168.60.1/24", "interface": "ether4", "comment": "mega-magic", "disabled": "no", "dynamic": "no"},
		"*14": {"address": "172.20.0.1/24", "interface": "vlan10", "comment": "", "disabled": "no", "dynamic": "no"},
		"*15": {"address": "192.168.70.5/24", "interface": "ether3", "comment": "", "disabled": "no", "dynamic": "yes"}, // learned automatically on a free port
	}
	r.objects["dhcp_clients"]["*20"] = map[string]string{"interface": "ether1", "gateway": "203.0.113.1", "add-default-route": "yes", "disabled": "no"}
	r.objects["routes"]["*30"] = map[string]string{"dst-address": "0.0.0.0/0", "gateway": "203.0.113.1", "dynamic": "yes", "distance": "1", "check-gateway": "", "scope": "", "target-scope": "", "routing-table": "main", "comment": "", "disabled": "no"}
	r.objects["dhcp"]["*40"] = map[string]string{"name": "dhcp1", "interface": "bridge", "address-pool": "customers", "lease-time": "10m", "disabled": "no"}
	r.objects["vlans"]["*50"] = map[string]string{"name": "vlan10", "vlan-id": "10", "interface": "ether4", "disabled": "no"}
	r.objects["vlans"]["*51"] = map[string]string{"name": "spare20", "vlan-id": "20", "interface": "ether4", "disabled": "no"}
	delete(r.objects["addresses"], "*4")
	delete(r.objects["addresses"], "*5")
	return r
}

func preview(t *testing.T, r *networkRunner, req NetworkRequest) (NetworkReview, error) {
	t.Helper()
	return quickConfigApplier(r).PreviewNetwork(context.Background(), "biz-1", "dev-1", req)
}

func applyOK(t *testing.T, r *networkRunner, req NetworkRequest) NetworkResult {
	t.Helper()
	a := quickConfigApplier(r)
	review, err := a.PreviewNetwork(context.Background(), "biz-1", "dev-1", req)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	req.Confirmation = review.Confirmation
	result, err := a.ApplyNetwork(context.Background(), "biz-1", "dev-1", req)
	if err != nil || !result.Verified {
		t.Fatalf("apply: %+v %v", result, err)
	}
	return result
}

func addr(address, iface string) NetworkRequest {
	return NetworkRequest{Kind: "ip_address", Values: map[string]string{"address": address, "interface": iface, "disabled": "no"}}
}

func TestIPAddressCreateIsReviewedAppliedAndVerified(t *testing.T) {
	r := addressFixture()
	review, err := preview(t, r, addr("192.168.50.1/24", "ether3"))
	if err != nil {
		t.Fatal(err)
	}
	if len(review.Steps) != 1 || !strings.Contains(review.Steps[0], "192.168.50.1/24") || !strings.Contains(strings.Join(review.Warnings, " "), "restores the previous settings") {
		t.Fatalf("review = %+v", review)
	}
	applyOK(t, r, addr("192.168.50.1/24", "ether3"))
	found := false
	for _, row := range r.objects["addresses"] {
		if row["address"] == "192.168.50.1/24" && row["interface"] == "ether3" && row["comment"] == "mega-magic" {
			found = true
		}
	}
	if !found {
		t.Fatal("address was not created on the router")
	}
}

func TestIPAddressCreateRollsBackWhenTheWriteFails(t *testing.T) {
	r := addressFixture()
	req := addr("192.168.50.1/24", "ether3")
	a := quickConfigApplier(r)
	review, err := a.PreviewNetwork(context.Background(), "biz-1", "dev-1", req)
	if err != nil {
		t.Fatal(err)
	}
	req.Confirmation = review.Confirmation
	r.failWrite = true
	result, err := a.ApplyNetwork(context.Background(), "biz-1", "dev-1", req)
	if err == nil || result.Verified || !result.RolledBack {
		t.Fatalf("want a rolled-back failure, got %+v %v", result, err)
	}
	for _, row := range r.objects["addresses"] {
		if row["address"] == "192.168.50.1/24" {
			t.Fatal("rolled-back address is still on the router")
		}
	}
}

func TestIPAddressCreateRefusesDangerousRequests(t *testing.T) {
	cases := map[string]NetworkRequest{
		"internet uplink port":        addr("192.168.50.1/24", "ether1"),
		"hub tunnel":                  addr("192.168.50.1/24", "mmh-abc"),
		"loopback":                    addr("192.168.50.1/24", "lo"),
		"port that belongs to bridge": addr("192.168.50.1/24", "ether2"),
		"unknown interface":           addr("192.168.50.1/24", "ether9"),
		"overlaps existing LAN":       addr("192.168.88.77/24", "ether3"),
		"overlaps a wider network":    addr("192.168.0.1/16", "ether3"),
		"inside the Hub range":        addr("10.77.0.9/24", "ether3"),
		"wider than the Hub range":    addr("10.0.0.1/8", "ether3"),
		"public address":              addr("8.8.8.1/24", "ether3"),
		"prefix too long":             addr("192.168.50.1/32", "ether3"),
		"point-to-point prefix":       addr("192.168.50.1/31", "ether3"),
		"prefix too short":            addr("10.1.1.1/7", "ether3"),
		"network address":             addr("192.168.50.0/24", "ether3"),
		"broadcast address":           addr("192.168.50.255/24", "ether3"),
		"ipv6":                        addr("fd00::1/64", "ether3"),
		"no prefix":                   addr("192.168.50.1", "ether3"),
		"garbage":                     addr("not-an-address", "ether3"),
		"injection in address":        addr(`192.168.50.1/24"; /system reset-configuration; "`, "ether3"),
		"injection in interface":      addr("192.168.50.1/24", `ether3"; /system reset-configuration; "`),
		"unexpected field":            {Kind: "ip_address", Values: map[string]string{"address": "192.168.50.1/24", "interface": "ether3", "gateway": "1.1.1.1"}},
		"bad enabled state":           {Kind: "ip_address", Values: map[string]string{"address": "192.168.50.1/24", "interface": "ether3", "disabled": "maybe"}},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			r := addressFixture()
			if _, err := preview(t, r, req); err == nil {
				t.Fatalf("%s must be refused", name)
			}
			for _, c := range r.commands {
				if strings.Contains(c, " add ") || strings.Contains(c, " set ") || strings.Contains(c, " remove ") {
					t.Fatalf("a refused request still wrote to the router: %s", c)
				}
			}
		})
	}
}

func TestIPAddressCreateFailsClosedWhenTheUplinkCannotBeDetermined(t *testing.T) {
	r := addressFixture()
	delete(r.objects, "dhcp_clients") // the fake then reports an unreadable menu
	for key := range r.objects {
		if key == "dhcp_clients" {
			r.objects[key] = nil
		}
	}
	good := quickConfigApplier(&unreadableMenuRunner{networkRunner: r, menu: "/ip dhcp-client"})
	if _, err := good.PreviewNetwork(context.Background(), "biz-1", "dev-1", addr("192.168.50.1/24", "ether3")); err == nil {
		t.Fatal("must refuse when the uplink cannot be determined")
	}
}

// unreadableMenuRunner makes one menu fail to read, like an unsupported or broken router.
type unreadableMenuRunner struct {
	*networkRunner
	menu string
}

func (u *unreadableMenuRunner) Run(ctx context.Context, cmd string) (string, error) {
	if strings.Contains(cmd, ":foreach id in=["+u.menu+" find") {
		return "", context.DeadlineExceeded
	}
	return u.networkRunner.Run(ctx, cmd)
}

func TestIPAddressEditRules(t *testing.T) {
	edit := func(id, address string) NetworkRequest {
		return NetworkRequest{Kind: "ip_address", ID: id, Values: map[string]string{"address": address}}
	}
	refused := map[string]NetworkRequest{
		"address that serves customers (bridge with DHCP)": edit("*10", "192.168.89.1/24"),
		"hub tunnel address":                               edit("*11", "10.99.0.8/32"),
		"dynamic address on the uplink":                    edit("*12", "192.168.71.1/24"),
		"dynamic address on a free port":                   edit("*15", "192.168.72.1/24"),
		"vlan with a customer address but overlapping":     edit("*14", "192.168.60.9/24"),
		"moving to another port":                           {Kind: "ip_address", ID: "*13", Values: map[string]string{"address": "192.168.61.1/24", "interface": "ether3"}},
		"overlapping another network":                      edit("*13", "192.168.88.9/24"),
		"public address":                                   edit("*13", "8.8.8.1/24"),
		"inside the Hub range":                             edit("*13", "10.77.0.50/24"),
		"missing record":                                   edit("*99", "192.168.61.1/24"),
		"no settings":                                      {Kind: "ip_address", ID: "*13", Values: map[string]string{}},
	}
	for name, req := range refused {
		t.Run("refuse "+name, func(t *testing.T) {
			r := addressFixture()
			if _, err := preview(t, r, req); err == nil {
				t.Fatalf("%s must be refused", name)
			}
		})
	}
	t.Run("a free address can change its own network and host", func(t *testing.T) {
		r := addressFixture()
		applyOK(t, r, edit("*13", "192.168.61.1/24"))
		if r.objects["addresses"]["*13"]["address"] != "192.168.61.1/24" {
			t.Fatal("address not changed")
		}
		// Same network as itself is not an overlap with itself.
		r2 := addressFixture()
		applyOK(t, r2, edit("*13", "192.168.60.2/24"))
		if r2.objects["addresses"]["*13"]["address"] != "192.168.60.2/24" {
			t.Fatal("changing the host within its own network was refused")
		}
	})
	t.Run("a free address can be disabled and re-enabled", func(t *testing.T) {
		r := addressFixture()
		applyOK(t, r, NetworkRequest{Kind: "ip_address", ID: "*13", Values: map[string]string{"disabled": "yes"}})
		if r.objects["addresses"]["*13"]["disabled"] != "yes" {
			t.Fatal("address not disabled")
		}
	})
	t.Run("an unchanged request is reported, not applied", func(t *testing.T) {
		r := addressFixture()
		if _, err := preview(t, r, edit("*13", "192.168.60.1/24")); err == nil || !strings.Contains(err.Error(), "already applied") {
			t.Fatalf("got %v", err)
		}
	})
}

func vlan(name, parent, id string) NetworkRequest {
	return NetworkRequest{Kind: "vlan", Values: map[string]string{"name": name, "interface": parent, "vlan-id": id, "disabled": "no"}}
}

func TestVLANCreateAndRefusals(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		r := addressFixture()
		applyOK(t, r, vlan("guests100", "ether3", "100"))
		found := false
		for _, row := range r.objects["vlans"] {
			if row["name"] == "guests100" && row["vlan-id"] == "100" && row["interface"] == "ether3" {
				found = true
			}
		}
		if !found {
			t.Fatal("VLAN was not created")
		}
	})
	refused := map[string]NetworkRequest{
		"id zero":                   vlan("v0", "ether3", "0"),
		"id too large":              vlan("v4095", "ether3", "4095"),
		"id not a number":           vlan("vx", "ether3", "ten"),
		"empty id":                  vlan("ve", "ether3", ""),
		"name already an interface": vlan("ether4", "ether3", "100"),
		"name of an existing vlan":  vlan("spare20", "ether3", "100"),
		"same id on same port":      vlan("again20", "ether4", "20"),
		"bad name":                  vlan("bad name!", "ether3", "100"),
		"injection in name":         vlan(`x"; /system reset-configuration; "`, "ether3", "100"),
		"vlan on a vlan":            vlan("nested", "vlan10", "100"),
		"vlan on the hub tunnel":    vlan("ontunnel", "mmh-abc", "100"),
		"vlan on loopback":          vlan("onlo", "lo", "100"),
		"unknown parent":            vlan("noparent", "ether9", "100"),
		"unexpected field":          {Kind: "vlan", Values: map[string]string{"name": "x1", "interface": "ether3", "vlan-id": "100", "mtu": "9000"}},
	}
	for name, req := range refused {
		t.Run("refuse "+name, func(t *testing.T) {
			r := addressFixture()
			if _, err := preview(t, r, req); err == nil {
				t.Fatalf("%s must be refused", name)
			}
		})
	}
	t.Run("the same id is fine on another port", func(t *testing.T) {
		r := addressFixture()
		applyOK(t, r, vlan("v20b", "ether3", "20"))
	})
}

func TestVLANEditRules(t *testing.T) {
	t.Run("an unused VLAN can be reshaped", func(t *testing.T) {
		r := addressFixture()
		applyOK(t, r, NetworkRequest{Kind: "vlan", ID: "*51", Values: map[string]string{"vlan-id": "21"}})
		if r.objects["vlans"]["*51"]["vlan-id"] != "21" {
			t.Fatal("VLAN id not changed")
		}
	})
	t.Run("an unused VLAN can be disabled", func(t *testing.T) {
		r := addressFixture()
		applyOK(t, r, NetworkRequest{Kind: "vlan", ID: "*51", Values: map[string]string{"disabled": "yes"}})
		if r.objects["vlans"]["*51"]["disabled"] != "yes" {
			t.Fatal("VLAN not disabled")
		}
	})
	t.Run("a VLAN in use can be reshaped only with a strong warning", func(t *testing.T) {
		r := addressFixture() // vlan10 carries 172.20.0.1/24
		review, err := preview(t, r, NetworkRequest{Kind: "vlan", ID: "*50", Values: map[string]string{"vlan-id": "11"}})
		if err != nil || !strings.Contains(strings.Join(review.Warnings, " "), "reverts by itself in 5 minutes") {
			t.Fatalf("review=%+v err=%v", review, err)
		}
	})
	t.Run("disabling a VLAN in use warns about what stops", func(t *testing.T) {
		r := addressFixture()
		review, err := preview(t, r, NetworkRequest{Kind: "vlan", ID: "*50", Values: map[string]string{"disabled": "yes"}})
		if err != nil || !strings.Contains(strings.Join(review.Warnings, " "), "Disabling it stops those services") {
			t.Fatalf("review=%+v err=%v", review, err)
		}
	})
	t.Run("a VLAN that carries the internet uplink is protected", func(t *testing.T) {
		r := addressFixture()
		r.objects["dhcp_clients"]["*21"] = map[string]string{"interface": "spare20", "gateway": "", "add-default-route": "yes", "disabled": "no"}
		if _, err := preview(t, r, NetworkRequest{Kind: "vlan", ID: "*51", Values: map[string]string{"disabled": "yes"}}); err == nil {
			t.Fatal("an uplink VLAN must be protected")
		}
	})
	t.Run("renaming and collisions are refused", func(t *testing.T) {
		r := addressFixture()
		if _, err := preview(t, r, NetworkRequest{Kind: "vlan", ID: "*51", Values: map[string]string{"name": "renamed"}}); err == nil {
			t.Fatal("renaming must be refused")
		}
		if _, err := preview(t, r, NetworkRequest{Kind: "vlan", ID: "*51", Values: map[string]string{"vlan-id": "10"}}); err == nil {
			t.Fatal("reusing an id on the same port must be refused")
		}
	})
}

func TestUplinkIsFoundFromRoutesPppoeAndDhcp(t *testing.T) {
	r := addressFixture()
	r.objects["pppoe_clients"]["*60"] = map[string]string{"name": "isp-pppoe", "interface": "ether3", "user": "u", "password": "p", "add-default-route": "yes", "default-route-distance": "1", "use-peer-dns": "yes", "disabled": "no"}
	// A static default route through an address on ether4 makes ether4 an uplink too.
	r.objects["routes"]["*61"] = map[string]string{"dst-address": "0.0.0.0/0", "gateway": "192.168.60.254", "dynamic": "no", "distance": "5", "check-gateway": "", "scope": "", "target-scope": "", "routing-table": "main", "comment": "", "disabled": "no"}
	up, err := networkUplinkInterfaces(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ether1", "ether3", "isp-pppoe", "ether4"} {
		if !up[want] {
			t.Fatalf("%s should be treated as an uplink: %v", want, up)
		}
	}
	for _, not := range []string{"bridge", "mmh-abc"} {
		if up[not] {
			t.Fatalf("%s must not be an uplink", not)
		}
	}
}

// The Hub range must be refused on its own merits, not only because the tunnel's
// own address happens to be on the router already.
func TestHubRangeIsRefusedEvenBeforeTheTunnelExists(t *testing.T) {
	for name, req := range map[string]NetworkRequest{
		"inside the range":   addr("10.77.0.9/24", "ether3"),
		"a wider network":    addr("10.0.0.1/8", "ether3"),
		"a different subnet": addr("10.77.0.129/25", "ether3"),
	} {
		t.Run(name, func(t *testing.T) {
			r := addressFixture()
			delete(r.objects["addresses"], "*11") // no tunnel address to overlap with
			if _, err := preview(t, r, req); err == nil || !strings.Contains(err.Error(), "Magic Hub") {
				t.Fatalf("the Hub range must be refused explicitly, got %v", err)
			}
		})
	}
	r := addressFixture()
	delete(r.objects["addresses"], "*11")
	if _, err := preview(t, r, NetworkRequest{Kind: "ip_address", ID: "*13", Values: map[string]string{"address": "10.77.0.50/24"}}); err == nil || !strings.Contains(err.Error(), "Magic Hub") {
		t.Fatalf("editing into the Hub range must be refused explicitly, got %v", err)
	}
}

func clearPending(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		networkPending.Delete("biz-1:dev-1")
		networkGuardUntil.Delete("biz-1:dev-1")
	})
}

func applyHeld(t *testing.T, r *networkRunner, req NetworkRequest) NetworkResult {
	t.Helper()
	clearPending(t)
	return applyOK(t, r, req)
}

func reshapeInUse() NetworkRequest {
	return NetworkRequest{Kind: "vlan", ID: "*50", Values: map[string]string{"vlan-id": "11"}}
}

func TestReshapingAVLANInUseWaitsForConfirmation(t *testing.T) {
	t.Run("apply verifies, keeps the timer armed and reports the deadline", func(t *testing.T) {
		r := addressFixture()
		result := applyHeld(t, r, reshapeInUse())
		if !result.PendingConfirm || result.RevertAt == "" {
			t.Fatalf("expected a pending confirmation: %+v", result)
		}
		if !r.guard {
			t.Fatal("the router-side timer must stay armed until a person keeps the change")
		}
		if r.objects["vlans"]["*50"]["vlan-id"] != "11" {
			t.Fatal("the change must be on the router while it waits")
		}
		if !strings.Contains(strings.Join(r.commands, "\n"), ":delay 5m;") {
			t.Fatal("a held change needs the longer five minute window")
		}
	})
	t.Run("keeping it cancels the timer and verifies that", func(t *testing.T) {
		r := addressFixture()
		applyHeld(t, r, reshapeInUse())
		if err := quickConfigApplier(r).ConfirmNetwork(context.Background(), "biz-1", "dev-1"); err != nil {
			t.Fatal(err)
		}
		if r.guard || r.objects["vlans"]["*50"]["vlan-id"] != "11" {
			t.Fatal("confirm must cancel the timer and keep the change")
		}
		if _, ok := networkPending.Load("biz-1:dev-1"); ok {
			t.Fatal("nothing should be pending after confirming")
		}
		if err := quickConfigApplier(r).ConfirmNetwork(context.Background(), "biz-1", "dev-1"); err == nil {
			t.Fatal("confirming twice must be refused")
		}
	})
	t.Run("reverting restores the old values and cancels the timer", func(t *testing.T) {
		r := addressFixture()
		applyHeld(t, r, reshapeInUse())
		result, err := quickConfigApplier(r).RevertNetwork(context.Background(), "biz-1", "dev-1")
		if err != nil || !result.RolledBack {
			t.Fatalf("%+v %v", result, err)
		}
		if r.objects["vlans"]["*50"]["vlan-id"] != "10" || r.guard {
			t.Fatal("revert must restore the VLAN id and cancel the timer")
		}
	})
	t.Run("once the router timer has run, keeping it is refused honestly", func(t *testing.T) {
		r := addressFixture()
		applyHeld(t, r, reshapeInUse())
		r.guard = false // the timer fired on the router
		err := quickConfigApplier(r).ConfirmNetwork(context.Background(), "biz-1", "dev-1")
		if err == nil || !strings.Contains(err.Error(), "time ran out") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("confirm is refused if the router no longer matches the change", func(t *testing.T) {
		r := addressFixture()
		applyHeld(t, r, reshapeInUse())
		r.objects["vlans"]["*50"]["vlan-id"] = "12" // someone changed it meanwhile
		if err := quickConfigApplier(r).ConfirmNetwork(context.Background(), "biz-1", "dev-1"); err == nil {
			t.Fatal("a drifted router must not be confirmed")
		}
		if !r.guard {
			t.Fatal("the timer must stay armed when confirmation is refused")
		}
	})
	t.Run("ordinary changes are not held", func(t *testing.T) {
		r := addressFixture()
		clearPending(t)
		result := applyOK(t, r, NetworkRequest{Kind: "vlan", ID: "*51", Values: map[string]string{"vlan-id": "21"}})
		if result.PendingConfirm || r.guard {
			t.Fatalf("an unused VLAN change must finish at once: %+v", result)
		}
	})
	t.Run("a VLAN that carries the uplink is still refused", func(t *testing.T) {
		r := addressFixture()
		r.objects["dhcp_clients"]["*21"] = map[string]string{"interface": "vlan10", "gateway": "", "add-default-route": "yes", "disabled": "no"}
		if _, err := preview(t, r, reshapeInUse()); err == nil {
			t.Fatal("an uplink VLAN must never be reshaped")
		}
	})
	t.Run("another change is blocked while one waits", func(t *testing.T) {
		r := addressFixture()
		applyHeld(t, r, reshapeInUse())
		a := quickConfigApplier(r)
		other := NetworkRequest{Kind: "vlan", ID: "*51", Values: map[string]string{"vlan-id": "22"}}
		review, err := a.PreviewNetwork(context.Background(), "biz-1", "dev-1", other)
		if err != nil {
			t.Fatal(err)
		}
		other.Confirmation = review.Confirmation
		if _, err = a.ApplyNetwork(context.Background(), "biz-1", "dev-1", other); err == nil {
			t.Fatal("a second write must wait for the first to be kept or reverted")
		}
	})
}

func removal(kind, id string) NetworkRequest { return NetworkRequest{Kind: kind, ID: id} }

func TestDeletingOnlyWhatTheAppCreated(t *testing.T) {
	mark := func(r *networkRunner, menu, id string) { r.objects[menu][id]["comment"] = networkOwnedMark }
	t.Run("an address the app created is deleted and verified", func(t *testing.T) {
		r := addressFixture() // *13 is 192.168.60.1/24 on ether4 with the app's comment
		review, err := preview(t, r, removal("remove_ip_address", "*13"))
		if err != nil || !strings.Contains(strings.Join(review.Steps, " "), "will be deleted") {
			t.Fatalf("%+v %v", review, err)
		}
		applyOK(t, r, removal("remove_ip_address", "*13"))
		if _, still := r.objects["addresses"]["*13"]; still || r.guard {
			t.Fatal("address must be gone and the timer cancelled")
		}
	})
	t.Run("an address someone else created is refused", func(t *testing.T) {
		r := addressFixture()
		r.objects["addresses"]["*16"] = map[string]string{"address": "10.9.0.1/24", "interface": "bond1", "comment": "", "disabled": "no", "dynamic": "no"} // otherwise unprotected
		if _, err := preview(t, r, removal("remove_ip_address", "*16")); err == nil {
			t.Fatal("must refuse an address without the app's mark")
		}
	})
	t.Run("protected addresses stay even if marked", func(t *testing.T) {
		for _, tc := range []struct{ name, id string }{{"Hub tunnel", "*11"}, {"dynamic", "*15"}, {"customer bridge", "*10"}} {
			r := addressFixture()
			mark(r, "addresses", tc.id)
			if _, err := preview(t, r, removal("remove_ip_address", tc.id)); err == nil {
				t.Fatalf("%s address must never be deleted", tc.name)
			}
		}
		r := addressFixture()
		mark(r, "addresses", "*12") // dynamic uplink address
		if _, err := preview(t, r, removal("remove_ip_address", "*12")); err == nil {
			t.Fatal("uplink address must never be deleted")
		}
	})
	t.Run("an unused marked VLAN is deleted", func(t *testing.T) {
		r := addressFixture()
		mark(r, "vlans", "*51")
		applyOK(t, r, removal("remove_vlan", "*51"))
		if _, still := r.objects["vlans"]["*51"]; still {
			t.Fatal("VLAN must be gone")
		}
	})
	t.Run("a VLAN in use, unmarked, or carrying the uplink is refused", func(t *testing.T) {
		r := addressFixture()
		mark(r, "vlans", "*50") // vlan10 carries an address
		if _, err := preview(t, r, removal("remove_vlan", "*50")); err == nil || !strings.Contains(err.Error(), "in use") {
			t.Fatalf("in-use VLAN: %v", err)
		}
		r = addressFixture()
		if _, err := preview(t, r, removal("remove_vlan", "*51")); err == nil {
			t.Fatal("unmarked VLAN must be refused")
		}
		r = addressFixture()
		mark(r, "vlans", "*51")
		r.objects["dhcp_clients"]["*21"] = map[string]string{"interface": "spare20", "gateway": "", "add-default-route": "yes", "disabled": "no"}
		if _, err := preview(t, r, removal("remove_vlan", "*51")); err == nil {
			t.Fatal("uplink VLAN must be refused")
		}
	})
	t.Run("a new VLAN carries the mark so it can be deleted later", func(t *testing.T) {
		r := addressFixture()
		applyOK(t, r, vlan("v30", "ether3", "30"))
		found := false
		for _, row := range r.objects["vlans"] {
			if row["name"] == "v30" && row["comment"] == networkOwnedMark {
				found = true
			}
		}
		if !found {
			t.Fatal("created VLAN must be marked")
		}
	})
	t.Run("extra input or no record is refused", func(t *testing.T) {
		r := addressFixture()
		if _, err := preview(t, r, NetworkRequest{Kind: "remove_vlan", ID: "*51", Values: map[string]string{"x": "y"}}); err == nil {
			t.Fatal("values are not accepted")
		}
		if _, err := preview(t, r, removal("remove_vlan", "")); err == nil {
			t.Fatal("a record is required")
		}
	})
}

func TestDeleteUndoRestoresTheSameRecord(t *testing.T) {
	r := addressFixture()
	r.objects["vlans"]["*51"]["comment"] = networkOwnedMark
	step := networkStep{Key: "vlans", ID: "*51", Remove: true, SelectorField: "name", SelectorValue: "spare20",
		Before: map[string]string{"name": "spare20", "interface": "ether4", "vlan-id": "20", "comment": networkOwnedMark, "disabled": "no"}}
	ctx := context.Background()
	if err := networkWrite(ctx, r, step, false); err != nil || networkVerify(ctx, r, step, false) != nil {
		t.Fatal("removal must write and verify")
	}
	// Undoing twice (the guard may run after a rollback) must not duplicate it.
	for i := 0; i < 2; i++ {
		if err := networkWrite(ctx, r, step, true); err != nil {
			t.Fatal(err)
		}
	}
	if err := networkVerify(ctx, r, step, true); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, row := range r.objects["vlans"] {
		if row["name"] == "spare20" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one restored VLAN, got %d", count)
	}
	if !strings.Contains(networkWriteCommand(step, true), ":if ([:len [/interface vlan find where name=") {
		t.Fatal("the router-side undo must be conditional")
	}
}

// A customer network exactly as Create customer network builds it, on a VLAN.
func shopFixture() *networkRunner {
	r := addressFixture()
	r.ifaces["mm-shop-vlan"] = "vlan"
	r.objects["vlans"]["*70"] = map[string]string{"name": "mm-shop-vlan", "vlan-id": "300", "interface": "ether3", "comment": "", "disabled": "no"}
	r.objects["addresses"]["*71"] = map[string]string{"address": "192.168.99.1/24", "interface": "mm-shop-vlan", "comment": "mm-shop", "disabled": "no", "dynamic": "no"}
	r.objects["pools"]["*72"] = map[string]string{"name": "mm-shop", "ranges": "192.168.99.10-192.168.99.200"}
	r.objects["dhcp_networks"]["*73"] = map[string]string{"address": "192.168.99.0/24", "gateway": "192.168.99.1", "dns-server": "1.1.1.1,8.8.8.8", "comment": "mm-shop"}
	r.objects["dhcp"]["*74"] = map[string]string{"name": "mm-shop", "interface": "mm-shop-vlan", "address-pool": "mm-shop", "lease-time": "1h", "disabled": "no"}
	r.objects["nat"]["*75"] = map[string]string{"chain": "srcnat", "action": "masquerade", "src-address": "192.168.99.0/24", "out-interface": "ether1", "comment": "mm-shop", "disabled": "no"}
	return r
}

func snapshotRows(r *networkRunner) map[string][]string {
	out := map[string][]string{}
	for key, rows := range r.objects {
		for _, row := range rows {
			keys := make([]string, 0, len(row))
			for k, v := range row {
				if (k == "dynamic" && v == "no") || v == "" {
					continue // the router sets these itself on a new record
				}
				keys = append(keys, k)
			}
			sort.Strings(keys)
			line := ""
			for _, k := range keys {
				line += k + "=" + row[k] + ";"
			}
			out[key] = append(out[key], line)
		}
		sort.Strings(out[key])
	}
	return out
}

func TestRemovingACustomerNetwork(t *testing.T) {
	req := removal("remove_network", "*74")
	t.Run("lists every piece in reverse creation order and removes them all", func(t *testing.T) {
		r := shopFixture()
		review, err := preview(t, r, req)
		if err != nil || len(review.Steps) != 6 {
			t.Fatalf("%+v %v", review, err)
		}
		joined := strings.Join(review.Steps, "|")
		order := []string{"masquerade", "DHCP service", "client gateway", "address pool", "gateway address", "VLAN mm-shop-vlan"}
		last := -1
		for _, want := range order {
			i := strings.Index(joined, want)
			if i <= last {
				t.Fatalf("%q is out of order or missing in %s", want, joined)
			}
			last = i
		}
		applyOK(t, r, req)
		for _, id := range []string{"*70", "*71"} {
			if _, still := r.objects["vlans"][id]; still {
				t.Fatalf("%s still there", id)
			}
		}
		if len(r.objects["dhcp"]) != 1 || len(r.objects["nat"]) != 0 || len(r.objects["pools"]) != 1 || len(r.objects["dhcp_networks"]) != 0 || r.guard {
			t.Fatalf("not fully removed: %v", snapshotRows(r))
		}
	})
	t.Run("a leftover setup guard is removed too", func(t *testing.T) {
		r := shopFixture()
		r.objects["filters"]["*76"] = map[string]string{"chain": "forward", "action": "drop", "in-interface": "mm-shop-vlan", "comment": "mm-shop-setup", "disabled": "no"}
		applyOK(t, r, req)
		if len(r.objects["filters"]) != 0 {
			t.Fatal("the setup guard must go")
		}
	})
	t.Run("the router-side undo puts back exactly what was removed", func(t *testing.T) {
		r := shopFixture()
		before := snapshotRows(r)
		steps, _, err := buildNetworkPlan(context.Background(), r, req)
		if err != nil {
			t.Fatal(err)
		}
		applyOK(t, r, req)
		ctx := context.Background()
		for i := len(steps) - 1; i >= 0; i-- { // the guard script runs in this order
			if err := networkWrite(ctx, r, steps[i], true); err != nil {
				t.Fatalf("restore %s: %v", steps[i].Key, err)
			}
			if err := networkVerify(ctx, r, steps[i], true); err != nil {
				t.Fatalf("verify %s: %v", steps[i].Key, err)
			}
		}
		after := snapshotRows(r)
		for key, rows := range before {
			if strings.Join(rows, "\n") != strings.Join(after[key], "\n") {
				t.Fatalf("%s differs after undo:\nbefore %v\nafter  %v", key, rows, after[key])
			}
		}
	})
	t.Run("refusals", func(t *testing.T) {
		for name, mutate := range map[string]func(r *networkRunner){
			"a Hotspot": func(r *networkRunner) {
				r.objects["hotspots"]["*80"] = map[string]string{"name": "mm-shop", "interface": "mm-shop-vlan", "profile": "mm-shop", "address-pool": "mm-shop", "disabled": "no"}
			},
			"a Hotspot only on the port": func(r *networkRunner) {
				r.objects["hotspots"]["*87"] = map[string]string{"name": "portal", "interface": "mm-shop-vlan", "profile": "p", "address-pool": "x", "disabled": "no"}
			},
			"a RADIUS entry": func(r *networkRunner) { r.radius = true },
			"a static reservation": func(r *networkRunner) {
				r.objects["leases"]["*81"] = map[string]string{"address": "192.168.99.50", "mac-address": "AA:BB:CC:DD:EE:FF", "server": "mm-shop", "comment": "", "disabled": "no", "dynamic": "no"}
			},
			"another address on port": func(r *networkRunner) {
				r.objects["addresses"]["*82"] = map[string]string{"address": "10.55.0.1/24", "interface": "mm-shop-vlan", "comment": "", "disabled": "no", "dynamic": "no"}
			},
			"the internet uplink": func(r *networkRunner) {
				r.objects["dhcp_clients"]["*83"] = map[string]string{"interface": "mm-shop-vlan", "gateway": "", "add-default-route": "yes", "disabled": "no"}
			},
			"a second DHCP service": func(r *networkRunner) {
				r.objects["dhcp"]["*84"] = map[string]string{"name": "other", "interface": "mm-shop-vlan", "address-pool": "x", "lease-time": "1h", "disabled": "no"}
			},
			"a duplicated NAT rule": func(r *networkRunner) {
				r.objects["nat"]["*85"] = map[string]string{"chain": "srcnat", "action": "masquerade", "src-address": "192.168.99.0/24", "out-interface": "ether1", "comment": "mm-shop", "disabled": "no"}
			},
			"a missing address": func(r *networkRunner) { delete(r.objects["addresses"], "*71") },
			"a shared pool": func(r *networkRunner) {
				r.objects["dhcp"]["*86"] = map[string]string{"name": "x", "interface": "ether4", "address-pool": "mm-shop", "lease-time": "1h", "disabled": "no"}
			},
		} {
			r := shopFixture()
			mutate(r)
			if _, err := preview(t, r, req); err == nil {
				t.Errorf("must refuse a network with %s", name)
			}
		}
	})
	t.Run("a DHCP service the wizard did not create is never removed", func(t *testing.T) {
		r := shopFixture()
		if _, err := preview(t, r, removal("remove_network", "*40")); err == nil { // dhcp1 on the customer bridge
			t.Fatal("must refuse a service that is not mm-named")
		}
	})
	t.Run("needs a record and takes no other input", func(t *testing.T) {
		r := shopFixture()
		if _, err := preview(t, r, removal("remove_network", "")); err == nil {
			t.Fatal("a record is required")
		}
		if _, err := preview(t, r, NetworkRequest{Kind: "remove_network", ID: "*74", Values: map[string]string{"x": "y"}}); err == nil {
			t.Fatal("values are not accepted")
		}
	})
}

func edit(values map[string]string) NetworkRequest {
	return NetworkRequest{Kind: "edit_network", ID: "*74", Values: values}
}

func TestEditingACustomerNetworkAsOneUnit(t *testing.T) {
	t.Run("changes range, DNS, lease and state together and touches nothing else", func(t *testing.T) {
		r := shopFixture()
		before := snapshotRows(r)
		values := map[string]string{"pool_start": "192.168.99.20", "pool_end": "192.168.99.100", "dns": "9.9.9.9, 1.1.1.1", "lease-time": "30m", "disabled": "no"}
		review, err := preview(t, r, edit(values))
		if err != nil || len(review.Steps) != 3 {
			t.Fatalf("%+v %v", review, err)
		}
		applyOK(t, r, edit(values))
		if r.objects["pools"]["*72"]["ranges"] != "192.168.99.20-192.168.99.100" || r.objects["dhcp_networks"]["*73"]["dns-server"] != "9.9.9.9,1.1.1.1" || r.objects["dhcp"]["*74"]["lease-time"] != "30m" {
			t.Fatalf("not applied: %v", snapshotRows(r))
		}
		after := snapshotRows(r)
		for _, key := range []string{"addresses", "nat", "vlans", "hotspots", "filters"} {
			if strings.Join(before[key], "\n") != strings.Join(after[key], "\n") {
				t.Fatalf("%s must not change", key)
			}
		}
		if r.guard {
			t.Fatal("the timer must be cancelled after a verified change")
		}
	})
	t.Run("a single field is enough and an unchanged form is reported as already applied", func(t *testing.T) {
		r := shopFixture()
		review, err := preview(t, r, edit(map[string]string{"lease-time": "2h"}))
		if err != nil || len(review.Steps) != 1 {
			t.Fatalf("%+v %v", review, err)
		}
		if _, err = preview(t, r, edit(map[string]string{"lease-time": "1h", "disabled": "no", "dns": "1.1.1.1,8.8.8.8", "pool_start": "192.168.99.10", "pool_end": "192.168.99.200"})); err == nil || !strings.Contains(err.Error(), "already applied") {
			t.Fatalf("unchanged form: %v", err)
		}
	})
	t.Run("the undo restores the previous values", func(t *testing.T) {
		r := shopFixture()
		req := edit(map[string]string{"pool_start": "192.168.99.30", "dns": "9.9.9.9", "lease-time": "5m", "disabled": "yes"})
		steps, _, err := buildNetworkPlan(context.Background(), r, req)
		if err != nil {
			t.Fatal(err)
		}
		before := snapshotRows(r)
		applyOK(t, r, req)
		for i := len(steps) - 1; i >= 0; i-- {
			if err := networkWrite(context.Background(), r, steps[i], true); err != nil {
				t.Fatal(err)
			}
			if err := networkVerify(context.Background(), r, steps[i], true); err != nil {
				t.Fatal(err)
			}
		}
		after := snapshotRows(r)
		for key, rows := range before {
			if strings.Join(rows, "\n") != strings.Join(after[key], "\n") {
				t.Fatalf("%s differs after undo", key)
			}
		}
	})
	t.Run("a network with a Hotspot can be edited, with a warning when DHCP is turned off", func(t *testing.T) {
		r := shopFixture()
		r.objects["hotspots"]["*80"] = map[string]string{"name": "mm-shop", "interface": "mm-shop-vlan", "profile": "mm-shop", "address-pool": "mm-shop", "disabled": "no"}
		review, err := preview(t, r, edit(map[string]string{"disabled": "yes"}))
		if err != nil || !strings.Contains(strings.Join(review.Warnings, " "), "Hotspot") {
			t.Fatalf("%+v %v", review, err)
		}
	})
	t.Run("refuses a bad range, DNS, lease or state", func(t *testing.T) {
		for name, values := range map[string]map[string]string{
			"outside the subnet":     {"pool_end": "192.168.100.5"},
			"includes the gateway":   {"pool_start": "192.168.99.1"},
			"network address":        {"pool_start": "192.168.99.0"},
			"broadcast address":      {"pool_end": "192.168.99.255"},
			"reversed":               {"pool_start": "192.168.99.150", "pool_end": "192.168.99.20"},
			"not an address":         {"pool_start": "abc"},
			"bad DNS":                {"dns": "1.1.1.1, nope"},
			"too many DNS":           {"dns": "1.1.1.1,2.2.2.2,3.3.3.3,4.4.4.4,5.5.5.5"},
			"multicast DNS":          {"dns": "224.0.0.1"},
			"lease too short":        {"lease-time": "10s"},
			"lease too long":         {"lease-time": "30d"},
			"lease nonsense":         {"lease-time": "soon"},
			"bad state":              {"disabled": "maybe"},
			"unexpected input":       {"address": "10.0.0.1/24"},
			"subnet change not here": {"gateway": "192.168.99.2"},
		} {
			r := shopFixture()
			if _, err := preview(t, r, edit(values)); err == nil {
				t.Errorf("must refuse %s", name)
			}
		}
	})
	t.Run("refuses networks it cannot edit exactly", func(t *testing.T) {
		for name, mutate := range map[string]func(r *networkRunner){
			"several ranges": func(r *networkRunner) {
				r.objects["pools"]["*72"]["ranges"] = "192.168.99.10-192.168.99.50,192.168.99.60-192.168.99.90"
			},
			"a shared pool": func(r *networkRunner) {
				r.objects["dhcp"]["*86"] = map[string]string{"name": "x", "interface": "ether4", "address-pool": "mm-shop", "lease-time": "1h", "disabled": "no"}
			},
			"the internet uplink": func(r *networkRunner) {
				r.objects["dhcp_clients"]["*83"] = map[string]string{"interface": "mm-shop-vlan", "gateway": "", "add-default-route": "yes", "disabled": "no"}
			},
			"a missing pool":  func(r *networkRunner) { delete(r.objects["pools"], "*72") },
			"a missing scope": func(r *networkRunner) { delete(r.objects["dhcp_networks"], "*73") },
			"a duplicated scope": func(r *networkRunner) {
				r.objects["dhcp_networks"]["*88"] = map[string]string{"address": "192.168.99.0/24", "gateway": "192.168.99.1", "dns-server": "1.1.1.1", "comment": "mm-shop"}
			},
		} {
			r := shopFixture()
			mutate(r)
			if _, err := preview(t, r, edit(map[string]string{"lease-time": "2h", "pool_start": "192.168.99.20"})); err == nil {
				t.Errorf("must refuse %s", name)
			}
		}
		r := shopFixture()
		if _, err := preview(t, r, NetworkRequest{Kind: "edit_network", ID: "*40", Values: map[string]string{"lease-time": "2h"}}); err == nil {
			t.Fatal("a DHCP service the wizard did not create must never be edited here")
		}
		if _, err := preview(t, r, NetworkRequest{Kind: "edit_network", ID: "", Values: map[string]string{"lease-time": "2h"}}); err == nil {
			t.Fatal("a record is required")
		}
	})
}

func move(values map[string]string) NetworkRequest {
	return NetworkRequest{Kind: "move_network", ID: "*74", Values: values}
}

var newSubnet = map[string]string{"gateway_cidr": "10.20.0.1/24", "pool_start": "10.20.0.10", "pool_end": "10.20.0.200"}

func TestMovingACustomerNetworkIsHeldUntilKept(t *testing.T) {
	t.Run("moves address, range, client settings and NAT together and waits for Keep", func(t *testing.T) {
		r := shopFixture()
		review, err := preview(t, r, move(newSubnet))
		if err != nil || len(review.Steps) != 4 || !strings.Contains(strings.Join(review.Warnings, " "), "reverts by itself in 5 minutes") {
			t.Fatalf("%+v %v", review, err)
		}
		result := applyHeld(t, r, move(newSubnet))
		if !result.PendingConfirm || !r.guard {
			t.Fatalf("a subnet move must wait for confirmation: %+v", result)
		}
		if r.objects["addresses"]["*71"]["address"] != "10.20.0.1/24" || r.objects["pools"]["*72"]["ranges"] != "10.20.0.10-10.20.0.200" ||
			r.objects["dhcp_networks"]["*73"]["address"] != "10.20.0.0/24" || r.objects["dhcp_networks"]["*73"]["gateway"] != "10.20.0.1" || r.objects["nat"]["*75"]["src-address"] != "10.20.0.0/24" {
			t.Fatalf("not moved: %v", snapshotRows(r))
		}
		if err := quickConfigApplier(r).ConfirmNetwork(context.Background(), "biz-1", "dev-1"); err != nil || r.guard {
			t.Fatalf("keep: %v guard=%v", err, r.guard)
		}
	})
	t.Run("reverting, or the router timer, puts every piece back exactly", func(t *testing.T) {
		r := shopFixture()
		before := snapshotRows(r)
		applyHeld(t, r, move(newSubnet))
		if result, err := quickConfigApplier(r).RevertNetwork(context.Background(), "biz-1", "dev-1"); err != nil || !result.RolledBack {
			t.Fatalf("%+v %v", result, err)
		}
		after := snapshotRows(r)
		for key, rows := range before {
			if strings.Join(rows, "\n") != strings.Join(after[key], "\n") {
				t.Fatalf("%s differs after revert", key)
			}
		}
	})
	t.Run("a network without a NAT rule can still be moved", func(t *testing.T) {
		r := shopFixture()
		delete(r.objects["nat"], "*75")
		review, err := preview(t, r, move(newSubnet))
		if err != nil || len(review.Steps) != 3 {
			t.Fatalf("%+v %v", review, err)
		}
	})
	t.Run("an unchanged request is reported as already applied", func(t *testing.T) {
		r := shopFixture()
		same := map[string]string{"gateway_cidr": "192.168.99.1/24", "pool_start": "192.168.99.10", "pool_end": "192.168.99.200"}
		if _, err := preview(t, r, move(same)); err == nil || !strings.Contains(err.Error(), "already applied") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("refuses a new subnet that is invalid or overlaps something", func(t *testing.T) {
		for name, values := range map[string]map[string]string{
			"overlaps the bridge network": {"gateway_cidr": "192.168.88.1/24", "pool_start": "192.168.88.10", "pool_end": "192.168.88.200"},
			"overlaps the Hub range":      {"gateway_cidr": "10.77.0.1/24", "pool_start": "10.77.0.10", "pool_end": "10.77.0.200"},
			"overlaps a customer network": {"gateway_cidr": "172.20.0.1/24", "pool_start": "172.20.0.10", "pool_end": "172.20.0.200"},
			"is a public network":         {"gateway_cidr": "8.8.8.1/24", "pool_start": "8.8.8.10", "pool_end": "8.8.8.200"},
			"is too small a prefix":       {"gateway_cidr": "10.20.0.1/30", "pool_start": "10.20.0.2", "pool_end": "10.20.0.2"},
			"range outside the subnet":    {"gateway_cidr": "10.20.0.1/24", "pool_start": "10.21.0.10", "pool_end": "10.21.0.20"},
			"range includes the gateway":  {"gateway_cidr": "10.20.0.1/24", "pool_start": "10.20.0.1", "pool_end": "10.20.0.50"},
			"range reversed":              {"gateway_cidr": "10.20.0.1/24", "pool_start": "10.20.0.100", "pool_end": "10.20.0.20"},
			"missing range":               {"gateway_cidr": "10.20.0.1/24"},
			"not an address":              {"gateway_cidr": "gateway", "pool_start": "a", "pool_end": "b"},
			"unexpected input":            {"gateway_cidr": "10.20.0.1/24", "pool_start": "10.20.0.10", "pool_end": "10.20.0.200", "dns": "1.1.1.1"},
		} {
			r := shopFixture()
			if _, err := preview(t, r, move(values)); err == nil {
				t.Errorf("must refuse a subnet that %s", name)
			}
		}
	})
	t.Run("refuses networks it cannot move exactly", func(t *testing.T) {
		for name, mutate := range map[string]func(r *networkRunner){
			"a Hotspot": func(r *networkRunner) {
				r.objects["hotspots"]["*80"] = map[string]string{"name": "mm-shop", "interface": "mm-shop-vlan", "profile": "mm-shop", "address-pool": "mm-shop", "disabled": "no"}
			},
			"a Hotspot only on the port": func(r *networkRunner) {
				r.objects["hotspots"]["*87"] = map[string]string{"name": "portal", "interface": "mm-shop-vlan", "profile": "p", "address-pool": "x", "disabled": "no"}
			},
			"a RADIUS entry": func(r *networkRunner) { r.radius = true },
			"a static reservation": func(r *networkRunner) {
				r.objects["leases"]["*81"] = map[string]string{"address": "192.168.99.50", "mac-address": "AA:BB:CC:DD:EE:FF", "server": "mm-shop", "comment": "", "disabled": "no", "dynamic": "no"}
			},
			"another address on the port": func(r *networkRunner) {
				r.objects["addresses"]["*82"] = map[string]string{"address": "10.55.0.1/24", "interface": "mm-shop-vlan", "comment": "", "disabled": "no", "dynamic": "no"}
			},
			"the internet uplink": func(r *networkRunner) {
				r.objects["dhcp_clients"]["*83"] = map[string]string{"interface": "mm-shop-vlan", "gateway": "", "add-default-route": "yes", "disabled": "no"}
			},
			"a second DHCP service": func(r *networkRunner) {
				r.objects["dhcp"]["*84"] = map[string]string{"name": "other", "interface": "mm-shop-vlan", "address-pool": "x", "lease-time": "1h", "disabled": "no"}
			},
			"a shared pool": func(r *networkRunner) {
				r.objects["dhcp"]["*86"] = map[string]string{"name": "x", "interface": "ether4", "address-pool": "mm-shop", "lease-time": "1h", "disabled": "no"}
			},
			"a limit on the old subnet": func(r *networkRunner) {
				r.objects["queues"]["*90"] = map[string]string{"name": "shop-limit", "target": "192.168.99.0/24", "max-limit": "10M/10M", "queue": "", "parent": "", "comment": "", "disabled": "no"}
			},
			"a limit on half of it": func(r *networkRunner) {
				r.objects["queues"]["*93"] = map[string]string{"name": "half", "target": "192.168.99.128/25", "max-limit": "5M/5M", "queue": "", "parent": "", "comment": "", "disabled": "no"}
			},
			"a limit on a list with it": func(r *networkRunner) {
				r.objects["queues"]["*94"] = map[string]string{"name": "mixed", "target": "10.9.9.0/24,192.168.99.77", "max-limit": "5M/5M", "queue": "", "parent": "", "comment": "", "disabled": "no"}
			},
			"a limit on a shop device": func(r *networkRunner) {
				r.objects["queues"]["*91"] = map[string]string{"name": "till", "target": "192.168.99.50/32", "max-limit": "2M/2M", "queue": "", "parent": "", "comment": "", "disabled": "no"}
			},
			"another NAT rule on it": func(r *networkRunner) {
				r.objects["nat"]["*92"] = map[string]string{"chain": "srcnat", "action": "masquerade", "src-address": "192.168.99.0/24", "out-interface": "ether1", "comment": "mine", "disabled": "no"}
			},
			"a missing address": func(r *networkRunner) { delete(r.objects["addresses"], "*71") },
			"a duplicated NAT rule": func(r *networkRunner) {
				r.objects["nat"]["*85"] = map[string]string{"chain": "srcnat", "action": "masquerade", "src-address": "192.168.99.0/24", "out-interface": "ether1", "comment": "mm-shop", "disabled": "no"}
			},
		} {
			r := shopFixture()
			mutate(r)
			if _, err := preview(t, r, move(newSubnet)); err == nil {
				t.Errorf("must refuse a network with %s", name)
			}
		}
		r := shopFixture()
		if _, err := preview(t, r, NetworkRequest{Kind: "move_network", ID: "*40", Values: newSubnet}); err == nil {
			t.Fatal("a DHCP service the wizard did not create must never be moved")
		}
	})
}

func TestMovingNeverUsesTheHubRangeEvenWithoutATunnelAddress(t *testing.T) {
	r := shopFixture()
	delete(r.objects["addresses"], "*11") // the Hub tunnel address, which would also cause an overlap
	if _, err := preview(t, r, move(map[string]string{"gateway_cidr": "10.77.0.1/24", "pool_start": "10.77.0.10", "pool_end": "10.77.0.200"})); err == nil {
		t.Fatal("the Hub management range must be refused on its own")
	}
	r = shopFixture()
	r.objects["queues"]["*95"] = map[string]string{"name": "other-net", "target": "10.9.9.0/24", "max-limit": "5M/5M", "queue": "", "parent": "", "comment": "", "disabled": "no"}
	r.objects["queues"]["*96"] = map[string]string{"name": "by-interface", "target": "ether4", "max-limit": "5M/5M", "queue": "", "parent": "", "comment": "", "disabled": "no"}
	if _, err := preview(t, r, move(newSubnet)); err != nil {
		t.Fatalf("limits that do not touch the old subnet must not block the move: %v", err)
	}
}
