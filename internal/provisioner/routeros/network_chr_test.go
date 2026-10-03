package routeros

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

type networkLabTrace struct {
	CommandRunner
	t *testing.T
}

func (r networkLabTrace) Run(ctx context.Context, command string) (string, error) {
	out, err := r.CommandRunner.Run(ctx, command)
	if strings.HasPrefix(command, ":foreach id in=[/ip route find where .id=") || strings.HasPrefix(command, ":put [/ip route find where ") {
		r.t.Logf("Lab route readback %s: %q error=%v", command, out, err)
	}
	return out, err
}

// Opt-in only: executed inside the trusted lab host against a newly created,
// loopback-published container, never against a registered customer device.
func TestNetworkCHRIsolatedAcceptance(t *testing.T) {
	pin := os.Getenv("MM_NETWORK_LAB_FINGERPRINT")
	if pin == "" {
		t.Skip("isolated CHR required")
	}
	if os.Getenv("MM_NETWORK_LAB_CONFIRM") != "mm-network-independent-20261003" {
		t.Fatal("explicit disposable container confirmation required")
	}
	if ValidateHostKeyFingerprint(pin) != nil {
		t.Fatal("valid lab host pin required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	r := NewSSHTransport(SSHConfig{Addr: "127.0.0.1:3222", User: "admin", HostKeyFingerprint: pin, Timeout: 10 * time.Second})
	// The port is assigned by the isolated acceptance launcher. A non-default
	// identity aborts before any setup writes, preventing accidental re-use.
	identity, e := r.Run(ctx, ":put [/system identity get name]")
	if e != nil || (strings.TrimSpace(identity) != "MikroTik" && strings.TrimSpace(identity) != "mm-network-disposable-acceptance" && strings.TrimSpace(identity) != "mm-background-restored") {
		t.Fatal("not a fresh isolated CHR")
	}
	for _, cmd := range []string{`/system identity set name="mm-network-disposable-acceptance"`, `:if ([:len [/ip address find where comment="mm-acceptance"]] = 0) do={ /ip address add address=198.18.2.16/24 interface=ether2 comment="mm-acceptance" }`} {
		if _, e = r.Run(ctx, cmd); e != nil {
			t.Fatal("isolated fixture setup failed (output withheld)")
		}
	}
	a := NewDeviceApplier(capabilitySource{}, 10*time.Second, func(SSHConfig) CommandRunner { return networkLabTrace{r, t} }, DefaultInterface, 0, 0)
	inventory, e := a.ReadNetwork(ctx, "biz-1", "dev-1")
	if e != nil {
		t.Fatal(e)
	}
	for _, key := range []string{"addresses", "routes", "pools", "dhcp", "pppoe_clients", "pppoe_servers", "ppp_profiles", "subscribers", "queues", "nat"} {
		if !inventory.Available[key] {
			t.Errorf("real RouterOS inventory unavailable: %s", key)
		}
	}
	if t.Failed() {
		t.FailNow()
	}
	for _, cmd := range []string{":put [/ping 1.1.1.1 count=2]", ":put [/ping 8.8.8.8 count=2]", "/ip route print detail without-paging"} {
		out, e := r.Run(ctx, cmd)
		if e != nil {
			t.Fatal("baseline lab routing check failed")
		}
		t.Logf("Baseline %s: %s", cmd, strings.TrimSpace(out))
	}
	apply := func(req NetworkRequest) {
		t.Helper()
		if req.ID != "" {
			key := map[string]string{"dhcp": "dhcp", "pool": "pools", "dhcp_network": "dhcp_networks", "hotspot_profile": "hotspot_profiles", "ppp_profile": "ppp_profiles"}[req.Kind]
			values, err := networkReadOne(ctx, r, key, req.ID, req.Values)
			if err != nil {
				t.Fatal(err)
			}
			same := true
			for field, wanted := range req.Values {
				if !networkEqual(field, values[field], wanted) {
					same = false
				}
			}
			if same {
				t.Logf("Existing disposable edit verified: %s", req.Kind)
				return
			}
		}
		if req.Kind == "wan" {
			rows, err := readNetworkObjects(ctx, r, "routes")
			if err != nil {
				t.Fatal(err)
			}
			wanted := "2"
			if req.Values["mode"] == "balance" {
				wanted = "1"
			}
			for _, row := range rows {
				if row.Values["comment"] == "mm-wan-default-2" && row.Values["distance"] == wanted {
					t.Logf("Existing disposable WAN mode: %s", req.Values["mode"])
					return
				}
			}
		}
		// A failed WAN trial may leave successful unrelated fixture objects.
		// Reuse only the exact names owned by this disposable acceptance test.
		owned := map[string]string{"pppoe_server": "pppoe_servers", "subscriber": "subscribers", "queue": "queues"}[req.Kind]
		if owned != "" {
			name := req.Values["name"]
			if req.Kind != "subscriber" {
				name = "mm-" + name
			}
			for _, obj := range inventory.Objects[owned] {
				if obj.Values["name"] == name || (req.Kind == "pppoe_server" && obj.Values["service-name"] == name) {
					t.Logf("Existing disposable fixture: %s", req.Kind)
					return
				}
			}
		}
		review, e := a.PreviewNetwork(ctx, "biz-1", "dev-1", req)
		if e != nil {
			t.Fatalf("%s review: %v", req.Kind, e)
		}
		req.Confirmation = review.Confirmation
		result, e := a.ApplyNetwork(ctx, "biz-1", "dev-1", req)
		if e != nil || !result.Verified {
			t.Fatalf("%s apply: result=%+v error=%v", req.Kind, result, e)
		}
		t.Logf("PASS configuration/guard readback: %s", req.Kind)
	}
	apply(NetworkRequest{Kind: "pppoe_server", Values: map[string]string{"name": "acceptance", "interface": "ether3", "gateway_cidr": "192.168.72.1/24", "pool_start": "192.168.72.10", "pool_end": "192.168.72.200", "dns": "1.1.1.1", "rate_limit": "10M/20M"}})
	apply(NetworkRequest{Kind: "subscriber", Values: map[string]string{"name": "acceptance-user", "password": "disposable-test-only", "profile": "mm-acceptance", "disabled": "no"}})
	apply(NetworkRequest{Kind: "queue", Values: map[string]string{"name": "acceptance", "target": "192.168.72.50/32", "max-limit": "5M/10M", "disabled": "no"}})
	apply(NetworkRequest{Kind: "wan", Values: map[string]string{"mode": "failover", "wan1": "ether1", "wan2": "ether2", "gateway1": "10.0.2.2", "gateway2": "198.18.2.2"}})
	// Simulate an ISP health-path outage, not a customer port interruption.
	// One router-side block restores the primary before returning its proof;
	// an independent timer restores it even if this SSH session disappears.
	guard, e := r.Run(ctx, `:put [:execute { :delay 30s; /ip route set [find where comment="mm-wan-probe-1"] disabled=no }]`)
	if e != nil || !networkIDPattern.MatchString(strings.TrimSpace(guard)) {
		t.Fatal("failover fixture restoration guard failed")
	}
	out, e := r.Run(ctx, `{ /ip route set [find where comment="mm-wan-probe-1"] disabled=yes; :delay 2s; :if ([/ip route get [find where comment="mm-wan-default-1"] active] = true) do={ :put "PRIMARY-STILL-ACTIVE" } else={ :put "PRIMARY-DOWN" }; :if ([/ip route get [find where comment="mm-wan-default-2"] active] = true) do={ :put "BACKUP-UP" }; :local received [/ping 9.9.9.9 count=2 interval=500ms]; :put ("BACKUP-REPLIES:" . $received); /ip route set [find where comment="mm-wan-probe-1"] disabled=no; :delay 2s; :if ([/ip route get [find where comment="mm-wan-default-1"] active] = true) do={ :put "PRIMARY-RESTORED" } }`)
	if e != nil || !strings.Contains(out, "PRIMARY-DOWN\r\n") || !strings.Contains(out, "BACKUP-UP\r\n") || (!strings.Contains(out, "BACKUP-REPLIES:2") && !strings.Contains(out, "BACKUP-REPLIES:1")) || !strings.Contains(out, "PRIMARY-RESTORED\r\n") {
		t.Fatalf("failover/recovery proof missing: %q error=%v (restoration timer retained)", out, e)
	}
	if _, e = r.Run(ctx, "/system script job remove "+strings.TrimSpace(guard)); e != nil {
		t.Fatal("fixture guard cancellation failed")
	}
	t.Log("PASS primary probe loss: backup default active, internet replies, primary recovered")
	apply(NetworkRequest{Kind: "wan", Values: map[string]string{"mode": "balance", "wan1": "ether1", "wan2": "ether2", "gateway1": "10.0.2.2", "gateway2": "198.18.2.2"}})
	out, e = r.Run(ctx, `:put ("ISP1:" . [/ip route get [find where comment="mm-wan-default-1"] active]); :put ("ISP2:" . [/ip route get [find where comment="mm-wan-default-2"] active])`)
	if e != nil || !strings.Contains(out, "ISP1:true") || !strings.Contains(out, "ISP2:true") {
		t.Fatalf("both balanced paths not active: %q %v", out, e)
	}
	t.Log("PASS both equal-cost WAN defaults active")
	// Create an isolated customer LAN, then exercise edits against discovered
	// IDs rather than fabricated RouterOS records. No real guest is attached.
	pools, e := readNetworkObjects(ctx, r, "pools")
	if e != nil {
		t.Fatal(e)
	}
	lanExists := false
	for _, pool := range pools {
		if pool.Values["name"] == "mm-acceptance-lan" {
			lanExists = true
		}
	}
	if !lanExists {
		g := GatewaySetup{Name: "acceptance-lan", WAN: "ether1", LAN: "ether4", Address: "192.168.75.1/24", PoolStart: "192.168.75.10", PoolEnd: "192.168.75.200", DNS: []string{"1.1.1.1"}, Hotspot: true}
		plan, err := a.PreviewGateway(ctx, "biz-1", "dev-1", g)
		if err != nil {
			t.Fatal(err)
		}
		g.Confirmation = plan.Confirmation
		result, err := a.ApplyGateway(ctx, "biz-1", "dev-1", g)
		if err != nil || !result.Verified {
			t.Fatalf("isolated LAN: %+v %v", result, err)
		}
		t.Log("PASS isolated LAN/DHCP/Hotspot initial configuration readback")
	}
	for _, edit := range []struct {
		key, selector, name, kind string
		values                    map[string]string
	}{
		{"dhcp", "name", "mm-acceptance-lan", "dhcp", map[string]string{"lease-time": "2h"}},
		{"pools", "name", "mm-acceptance-lan", "pool", map[string]string{"ranges": "192.168.75.20-192.168.75.180"}},
		{"dhcp_networks", "comment", "mm-acceptance-lan", "dhcp_network", map[string]string{"gateway": "192.168.75.1", "dns-server": "1.1.1.1,8.8.8.8"}},
		{"hotspot_profiles", "name", "mm-acceptance-lan", "hotspot_profile", map[string]string{"dns-name": "acceptance.test", "login-by": "http-chap,mac", "use-radius": "no"}},
		{"ppp_profiles", "name", "mm-acceptance", "ppp_profile", map[string]string{"rate-limit": "15M/25M"}},
	} {
		rows, err := readNetworkObjects(ctx, r, edit.key)
		if err != nil {
			t.Fatal(err)
		}
		id := ""
		for _, row := range rows {
			if row.Values[edit.selector] == edit.name {
				id = row.ID
			}
		}
		if id == "" {
			t.Fatalf("owned fixture %s missing", edit.key)
		}
		apply(NetworkRequest{Kind: edit.kind, ID: id, Values: edit.values})
	}
	// A separate background job must survive its creating SSH session closing.
	out, e = r.Run(ctx, `:put [:execute { :delay 2s; /system identity set name="mm-background-restored" }]`)
	if e != nil || !networkIDPattern.MatchString(strings.TrimSpace(out)) {
		t.Fatal("background job unavailable")
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		out, e = r.Run(ctx, ":put [/system identity get name]")
		if e == nil && strings.TrimSpace(out) == "mm-background-restored" {
			t.Log("PASS router-side job survives SSH disconnect")
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("background restoration did not execute")
}
