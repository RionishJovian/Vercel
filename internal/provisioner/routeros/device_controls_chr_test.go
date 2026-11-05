package routeros

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// Real RouterOS behaviour of the device controls, on the disposable CHR. Needs the network set up by
// TestCHRLabHotspotAndReads (m3 customer LAN, m4 guest in VRF "guest"). A CHR has one Ethernet port, its
// own management link, and no PoE, so turning a port off and restarting power need real hardware.
func TestCHRLabDeviceControls(t *testing.T) {
	target := os.Getenv("MM_CHR_SSH")
	if target == "" {
		t.Skip("set MM_CHR_SSH to run against a disposable CHR")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	r := chrRunner{target}
	l := chrLab{t, r, ctx}
	a := NewDeviceApplier(capabilitySource{}, 60*time.Second, func(SSHConfig) CommandRunner { return r }, DefaultInterface, 0, 0)
	const biz, dev = "biz-1", "dev-1"
	guestMAC := strings.ToUpper(l.put(`:put [/interface get [find name=m4] mac-address]`))
	blockRule := deviceBlockComment(guestMAC)
	apply := func(t *testing.T, kind string, values map[string]string) NetworkResult {
		t.Helper()
		var result NetworkResult
		var err error
		// The lab's broadband guard runs every 15 s, and the app rightly refuses to write during it.
		for attempt := 0; attempt < 5; attempt++ {
			request := NetworkRequest{Kind: kind, Values: values}
			var review NetworkReview
			if review, err = a.PreviewNetwork(ctx, biz, dev, request); err != nil {
				t.Fatalf("preview %s: %v", kind, err)
			}
			t.Logf("%s review: %v", kind, review.Steps)
			request.Confirmation = review.Confirmation
			result, err = a.ApplyNetwork(ctx, biz, dev, request)
			if err == nil || !strings.Contains(err.Error(), "active script job") {
				break
			}
			time.Sleep(4 * time.Second)
		}
		if err != nil || !result.Verified {
			t.Fatalf("apply %s: %+v %v", kind, result, err)
		}
		return result
	}
	pings := func() int {
		// A second lab device in its own VRF: the guest reaches it only through the router's forward chain.
		out, _ := r.Run(ctx, `:local n [/ping 10.99.0.2 vrf=guest count=4 interval=300ms]; :put ("MM-PINGS:" . $n)`)
		var n int
		if i := strings.LastIndex(out, "MM-PINGS:"); i >= 0 {
			fmt.Sscanf(out[i+len("MM-PINGS:"):], "%d", &n)
		}
		return n
	}
	cleanup := func() {
		for _, cmd := range []string{
			`/ip firewall filter remove [find where comment=` + routerOSQuoted(blockRule) + `]`,
			`/ip hotspot active remove [find where mac-address=` + guestMAC + `]`,
			`/ip hotspot user remove [find where name=` + routerOSQuoted(guestMAC) + `]`,
			`/system scheduler remove [find where name~"^mm-(voucher|paid)-"]`,
			`/ip dhcp-client remove [find where interface=m5]`,
			`/ip dhcp-server lease remove [find where comment="lab-lease"]`,
			`/ip dhcp-server lease remove [find where address="10.60.0.123"]`,
			`/ip route remove [find where routing-table=g3]`,
			`/ip address remove [find where address="10.99.0.1/24"]`,
			`/ip address remove [find where address="10.99.0.2/24"]`,
			`/ip vrf remove [find where name=g2]`,
			`/ip vrf remove [find where name=g3]`,
			`/interface macvlan remove [find where name=m5]`,
			`/interface macvlan remove [find where name=m6]`,
			`/interface macvlan remove [find where name=m7]`,
			`/interface set [find where name=ether1] comment=""`,
		} {
			_, _ = r.Run(context.Background(), cmd)
		}
	}
	cleanup()
	t.Cleanup(cleanup)
	for _, cmd := range []string{
		`/interface macvlan add name=m6 interface=ether1 mode=bridge`,
		`/interface macvlan add name=m7 interface=ether1 mode=bridge`,
		`/ip vrf add name=g3 interfaces=m6`,
		`/ip address add address=10.99.0.1/24 interface=m7`,
		`/ip address add address=10.99.0.2/24 interface=m6`,
		`/ip route add dst-address=0.0.0.0/0 gateway=10.99.0.1@g3 routing-table=g3`,
	} {
		l.put(cmd)
	}

	t.Run("observations parse on real output", func(t *testing.T) {
		inv, err := a.ReadObservations(ctx, biz, dev)
		if err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"interfaces", "interface_labels", "device_blocks", "arp", "hotspot_sessions"} {
			if !inv.Available[key] {
				t.Errorf("%s was not readable", key)
			}
		}
		if inv.Available["poe_ports"] && len(inv.Objects["poe_ports"]) != 0 {
			t.Errorf("a CHR has no PoE: %+v", inv.Objects["poe_ports"])
		}
		t.Logf("poe_ports available=%v, interfaces=%d", inv.Available["poe_ports"], len(inv.Objects["interfaces"]))
	})

	t.Run("refusals on a real router", func(t *testing.T) {
		own := strings.ToUpper(l.put(`:put [/interface get [find name=m3] mac-address]`))
		if _, err := a.PreviewNetwork(ctx, biz, dev, NetworkRequest{Kind: "device_block", Values: map[string]string{"mac": own}}); err == nil || !strings.Contains(err.Error(), "router itself") {
			t.Errorf("own MAC: %v", err)
		}
		if _, err := a.PreviewNetwork(ctx, biz, dev, NetworkRequest{Kind: "port_state", Values: map[string]string{"interface": "ether1", "disabled": "yes"}}); err == nil || !strings.Contains(err.Error(), "uplink") {
			t.Fatalf("the management and uplink port must be refused: %v", err)
		}
		if _, err := a.PreviewNetwork(ctx, biz, dev, NetworkRequest{Kind: "port_state", Values: map[string]string{"interface": "lan0", "disabled": "yes"}}); err == nil || !strings.Contains(err.Error(), "Ethernet") {
			t.Errorf("a bridge must be refused: %v", err)
		}
		if _, err := a.PreviewNetwork(ctx, biz, dev, NetworkRequest{Kind: "poe_cycle", Values: map[string]string{"interface": "ether1"}}); err == nil {
			t.Errorf("a CHR has no PoE, so this must be refused")
		}
	})

	t.Run("port label set and cleared", func(t *testing.T) {
		apply(t, "port_label", map[string]string{"interface": "ether1", "comment": "Lab uplink"})
		if got := l.put(`:put [/interface get [find name=ether1] comment]`); got != "Lab uplink" {
			t.Fatalf("label: %q", got)
		}
		inv, err := a.ReadObservations(ctx, biz, dev)
		if err != nil || !inv.Available["interfaces"] {
			t.Fatalf("a label must not break the read: %v %+v", err, inv.Available)
		}
		apply(t, "port_label", map[string]string{"interface": "ether1", "comment": ""})
		if got := l.put(`:put [/interface get [find name=ether1] comment]`); got != "" {
			t.Fatalf("label not cleared: %q", got)
		}
	})

	online := func(t *testing.T) string {
		t.Helper()
		g := GuestVoucherAccess{VoucherID: fmt.Sprintf("dddddddd-dddd-4ddd-8ddd-%012x", time.Now().Unix()), ClientMAC: guestMAC, Minutes: 60, DataMB: 500, ExpiresAt: time.Now().Add(time.Hour)}
		// Each case starts with a new voucher, and the app rightly refuses to reuse another voucher's login.
		_, _ = r.Run(ctx, `/ip hotspot user remove [find where name=`+routerOSQuoted(guestMAC)+`]`)
		_, _ = r.Run(ctx, `/system scheduler remove [find where name~"^mm-(voucher|paid)-"]`)
		var err error
		end := time.Now().Add(60 * time.Second)
		for time.Now().Before(end) {
			_, _ = r.Run(ctx, `/ping 1.1.1.1 vrf=guest count=2`)
			if err = a.ConfigureGuestVoucher(ctx, biz, dev, g); err == nil {
				break
			}
		}
		if err != nil {
			t.Fatal(err)
		}
		for end = time.Now().Add(30 * time.Second); time.Now().Before(end); {
			if l.put(`:put [:len [/ip hotspot active find where mac-address=`+guestMAC+`]]`) != "0" {
				return l.put(`:put [/ip hotspot active find where mac-address=` + guestMAC + `]`)
			}
			_, _ = r.Run(ctx, `/ping 1.1.1.1 vrf=guest count=2`)
		}
		t.Fatal("guest did not get online")
		return ""
	}

	t.Run("block stops forwarded traffic and unblock restores it", func(t *testing.T) {
		online(t)
		before := pings()
		if before == 0 {
			t.Fatal("the guest has no forwarded path before the block, so this run proves nothing")
		}
		// The lab guest is a macvlan on the router, so the app rightly refuses it as the router's own
		// address. The exact same rule is written directly to measure what it does to traffic.
		step := deviceBlockStep(guestMAC, false)
		if err := networkWrite(ctx, r, step, false); err != nil {
			t.Fatal(err)
		}
		if err := networkVerify(ctx, r, step, false); err != nil {
			t.Errorf("the block is not verified as the first static rule: %v", err)
		}
		clearDeviceConnections(ctx, r, guestMAC)
		during := pings()
		inv, err := a.ReadObservations(ctx, biz, dev)
		if err != nil || len(inv.Objects["device_blocks"]) != 1 {
			t.Errorf("the block should show in observations: %v %+v", err, inv.Objects["device_blocks"])
		}
		if err = networkWrite(ctx, r, step, true); err != nil {
			t.Fatal(err)
		}
		after := pings()
		t.Logf("replies before=%d during=%d after=%d", before, during, after)
		if during != 0 {
			t.Errorf("blocked guest still got %d replies", during)
		}
		if after == 0 {
			t.Error("guest did not recover after the block was removed")
		}
	})

	t.Run("block and unblock through review and apply", func(t *testing.T) {
		// A MAC the router does not own, so the full path runs, safety checks included.
		const other = "02:AA:BB:CC:DD:03"
		t.Cleanup(func() {
			_, _ = r.Run(context.Background(), `/ip firewall filter remove [find where comment=`+routerOSQuoted(deviceBlockComment(other))+`]`)
		})
		apply(t, "device_block", map[string]string{"mac": other})
		first := l.put(`:put [:pick [/ip firewall filter find where dynamic=no] 0]`)
		if rule := l.put(`:put [/ip firewall filter find where comment=` + routerOSQuoted(deviceBlockComment(other)) + `]`); rule != first {
			t.Errorf("the block is not the first static rule: %s vs %s", rule, first)
		}
		if _, err := a.PreviewNetwork(ctx, biz, dev, NetworkRequest{Kind: "device_block", Values: map[string]string{"mac": other}}); err == nil || !strings.Contains(err.Error(), "already blocked") {
			t.Errorf("a second block must be refused: %v", err)
		}
		apply(t, "device_unblock", map[string]string{"mac": other})
		if got := l.put(`:put [:len [/ip firewall filter find where comment=` + routerOSQuoted(deviceBlockComment(other)) + `]]`); got != "0" {
			t.Errorf("rule still present: %s", got)
		}
	})

	t.Run("reserve the address of a device with a dynamic lease", func(t *testing.T) {
		for _, cmd := range []string{
			`/interface macvlan add name=m5 interface=ether1 mode=bridge`,
			`/ip vrf add name=g2 interfaces=m5`,
			`/ip dhcp-client add interface=m5 add-default-route=no use-peer-dns=no use-peer-ntp=no`,
		} {
			l.put(cmd)
		}
		mac := strings.ToUpper(l.put(`:put [/interface get [find name=m5] mac-address]`))
		var leased string
		for end := time.Now().Add(60 * time.Second); time.Now().Before(end) && leased == ""; time.Sleep(3 * time.Second) {
			// An offered lease is already the dynamic record this case is about.
			if l.put(`:put [:len [/ip dhcp-server lease find where mac-address=`+mac+` dynamic=yes]]`) != "0" {
				leased = l.put(`:put [/ip dhcp-server lease get [find where mac-address=` + mac + `] address]`)
			}
		}
		if leased == "" {
			t.Fatal("the test client got no lease")
		}
		t.Logf("dynamic lease %s for %s", leased, mac)
		request := map[string]string{"address": leased, "mac-address": mac, "server": "mm-labnet", "disabled": "no"}
		apply(t, "lease", request)
		if got := l.put(`:put [/ip dhcp-server lease get [find where mac-address=` + mac + ` dynamic=no] address]`); got != leased {
			t.Fatalf("reservation address %q", got)
		}
		l.put(`/ip dhcp-server lease remove [find where mac-address=` + mac + `]`)
	})
}
