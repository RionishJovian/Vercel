package routeros

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Strict command interpreter: unknown commands fail, mutations have actual
// state and readback, and a transport failure can happen after committing.
type networkRunner struct {
	objects   map[string]map[string]map[string]string
	commands  []string
	next      int
	failWrite bool
	failPing  bool
	guard     bool
}

var networkTestValues = regexp.MustCompile(`([a-zA-Z0-9_.-]+)="([^"\\]*)"`)

func TestNetworkProbeLabelledScalarOnly(t *testing.T) {
	for _, tc := range []struct {
		out   string
		count int
		valid bool
	}{
		{"Columns: SEQ, HOST\n0 1.1.1.1 56\n1 1.1.1.1 56\nMM-PROBE-RESULT:2\r\n", 2, true},
		{"MM-PROBE-RESULT:0\n", 0, true},
		{"0 1.1.1.1 56\n2\n", 0, false},
		{"MM-PROBE-RESULT:2\nMM-PROBE-RESULT:0\n", 0, false},
		{"MM-PROBE-RESULT:3\n", 0, false},
		{"MM-PROBE-RESULT:-1\n", 0, false},
	} {
		r := commandRunnerFunc(func(context.Context, string) (string, error) { return tc.out, nil })
		got, err := networkProbe(context.Background(), r, "1.1.1.1")
		if (err == nil) != tc.valid || got != tc.count {
			t.Fatalf("probe result %q: %d %v", tc.out, got, err)
		}
	}
}

func TestNetworkDurationScalarAndStrictInput(t *testing.T) {
	for _, pair := range [][2]string{{"01:00:00", "1h"}, {"1d00:00:00", "24h"}, {"1w00:00:00", "7d"}, {"00:02:00", "2m"}} {
		if !networkEqual("lease-time", pair[0], pair[1]) {
			t.Fatalf("canonical duration mismatch: %v", pair)
		}
	}
	for _, value := range []string{"ignored1h", "1hignored", "1h2h", "1s1h", "99:99:99", "999999999999h", "8d"} {
		if _, ok := networkDurationSeconds(value); ok {
			t.Fatalf("invalid duration accepted: %q", value)
		}
	}
}

func (r *networkRunner) Run(_ context.Context, cmd string) (string, error) {
	r.commands = append(r.commands, cmd)
	if strings.HasPrefix(cmd, "{ ") {
		body := strings.TrimSuffix(strings.TrimPrefix(cmd, "{ "), " }")
		quoted, escaped := false, false
		start := 0
		for i, c := range body {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '"' {
				quoted = !quoted
			}
			if c == ';' && !quoted {
				if _, e := r.Run(context.Background(), strings.TrimSpace(body[start:i])); e != nil {
					return "", e
				}
				start = i + 1
			}
		}
		return r.Run(context.Background(), strings.TrimSpace(body[start:]))
	}
	if strings.HasPrefix(cmd, ":local received [/ping ") {
		if r.failPing {
			return "Columns: SEQ, HOST\n0 1.1.1.1 timeout\nMM-PROBE-RESULT:0\n", nil
		}
		return "Columns: SEQ, HOST\n0 1.1.1.1\n1 1.1.1.1\nMM-PROBE-RESULT:2\n", nil
	}
	if cmd == gatewayInterfacesCommand {
		return "name: ether1\ntype: ether\nrunning: true\nname: ether2\ntype: ether\nrunning: true\nname: ether3\ntype: ether\nrunning: true\n", nil
	}
	for _, source := range []struct{ command, key, field, prefix string }{
		{gatewayAddressesCommand, "addresses", "address", "address: "},
		{gatewayRoutesCommand, "routes", "dst-address", "dst-address: "},
		{gatewayPoolsCommand, "pools", "ranges", "ranges: "},
	} {
		if cmd == source.command {
			out := ""
			for _, row := range r.objects[source.key] {
				out += source.prefix + row[source.field] + "\n"
			}
			return out, nil
		}
	}
	if strings.HasPrefix(cmd, "/interface bridge port print count-only where interface=") {
		return "0", nil
	}
	if strings.HasPrefix(cmd, ":put [/interface get") {
		return "ether\n", nil
	}
	if strings.HasPrefix(cmd, "/interface print count-only") {
		return "1", nil
	}
	if strings.HasPrefix(cmd, "/export ") || strings.HasPrefix(cmd, "/system backup ") {
		return "", nil
	}
	if strings.HasPrefix(cmd, ":put [:execute ") {
		r.guard = true
		return "*FFE\n", nil
	}
	if cmd == "/system script job remove *FFE" {
		r.guard = false
		return "", nil
	}
	if strings.HasPrefix(cmd, "/system script job print count-only") {
		if strings.Contains(cmd, " where .id=") {
			if r.guard {
				return "1", nil
			}
			return "0", nil
		}
		if r.guard {
			return "2", nil
		}
		return "1", nil
	}
	for key, menu := range networkMenus {
		rows := r.objects[key]
		if strings.HasPrefix(cmd, `:foreach id in=[`+menu.path+` find where .id=`) {
			id := strings.Split(strings.TrimPrefix(cmd, `:foreach id in=[`+menu.path+` find where .id=`), "]")[0]
			if rows[id] == nil {
				return "", nil
			}
			out := "id: " + id + "\n"
			for _, field := range menu.fields {
				if strings.Contains(cmd, ` get $id `+field+`]]`) {
					out += field + ": " + rows[id][field] + "\n"
				}
			}
			return out, nil
		}
		if cmd == networkReadCommand(menu) {
			ids := []string{}
			for id := range rows {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			out := ""
			for _, id := range ids {
				out += "id: " + id + "\n"
				for _, field := range menu.fields {
					out += field + ": " + rows[id][field] + "\n"
				}
			}
			return out, nil
		}
		if strings.HasPrefix(cmd, `:put [:tostr [`+menu.path+` get `) {
			fields := strings.Fields(strings.TrimSuffix(strings.TrimPrefix(cmd, `:put [:tostr [`+menu.path+` get `), "]]"))
			if len(fields) != 2 || rows[fields[0]] == nil {
				return "", fmt.Errorf("missing record")
			}
			return rows[fields[0]][fields[1]] + "\n", nil
		}
		if strings.HasPrefix(cmd, `:put [:len [`+menu.path+` find where .id=`) {
			id := strings.TrimSuffix(strings.TrimPrefix(cmd, `:put [:len [`+menu.path+` find where .id=`), "]]")
			if rows[id] != nil {
				return "1", nil
			}
			return "0", nil
		}
		selector := func() []string {
			values := map[string]string{}
			for _, m := range networkTestValues.FindAllStringSubmatch(cmd, -1) {
				values[m[1]] = m[2]
			}
			ids := []string{}
			for id, row := range rows {
				match := true
				for field, value := range values {
					if row[field] != value {
						match = false
					}
				}
				if match {
					ids = append(ids, id)
				}
			}
			sort.Strings(ids)
			return ids
		}
		if strings.HasPrefix(cmd, menu.path+" print count-only") {
			return fmt.Sprint(len(selector())), nil
		}
		if strings.HasPrefix(cmd, `:put [`+menu.path+` find where `) {
			return strings.Join(selector(), ";") + "\n", nil
		}
		if strings.HasPrefix(cmd, menu.path+" remove ") {
			for _, id := range selector() {
				delete(rows, id)
			}
			return "", nil
		}
		if strings.HasPrefix(cmd, menu.path+" add ") || strings.HasPrefix(cmd, menu.path+" set ") {
			if rows == nil {
				rows = map[string]map[string]string{}
				r.objects[key] = rows
			}
			var id string
			if strings.HasPrefix(cmd, menu.path+" add ") {
				r.next++
				id = fmt.Sprintf("*%X", r.next)
				rows[id] = map[string]string{}
			} else {
				id = strings.Fields(strings.TrimPrefix(cmd, menu.path+" set "))[0]
				if rows[id] == nil {
					return "", fmt.Errorf("missing set record")
				}
			}
			for _, m := range networkTestValues.FindAllStringSubmatch(cmd, -1) {
				rows[id][m[1]] = m[2]
			}
			if r.failWrite {
				r.failWrite = false
				return "", fmt.Errorf("transport failed after write with secret output withheld")
			}
			return "", nil
		}
	}
	return "", fmt.Errorf("unrecognized command")
}
func networkFixture() *networkRunner {
	r := &networkRunner{objects: map[string]map[string]map[string]string{}, next: 100}
	for key := range networkMenus {
		r.objects[key] = map[string]map[string]string{}
	}
	r.objects["ppp_profiles"]["*1"] = map[string]string{"name": "business", "local-address": "192.168.70.1", "remote-address": "customers", "dns-server": "1.1.1.1", "rate-limit": "5M/10M", "only-one": "yes"}
	r.objects["pools"]["*2"] = map[string]string{"name": "customers", "ranges": "192.168.70.10-192.168.70.200"}
	r.objects["subscribers"]["*3"] = map[string]string{"name": "alice", "password": "old-secret", "service": "pppoe", "profile": "business", "disabled": "no"}
	r.objects["addresses"]["*4"] = map[string]string{"address": "192.0.2.2/24", "interface": "ether1", "disabled": "no", "dynamic": "no"}
	r.objects["addresses"]["*5"] = map[string]string{"address": "198.51.100.2/24", "interface": "ether2", "disabled": "no", "dynamic": "no"}
	r.objects["wireless"]["*6"] = map[string]string{"name": "wlan1", "ssid": "old", "country": "myanmar", "mode": "ap-bridge", "security-profile": "default", "disabled": "no"}
	return r
}
func TestNetworkInventoryNeverReturnsCredentials(t *testing.T) {
	r := networkFixture()
	a := quickConfigApplier(r)
	data, e := a.ReadNetwork(context.Background(), "biz-1", "dev-1")
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(data)
	if strings.Contains(string(raw), "old-secret") || strings.Contains(string(raw), `"password"`) {
		t.Fatal("credential exposed")
	}
	if data.Objects["subscribers"][0].Values["has_secret"] != "yes" {
		t.Fatal("secret state missing")
	}
}

func TestNetworkPPPoEServiceAvoidsUnroutedPools(t *testing.T) {
	r := networkFixture()
	a := quickConfigApplier(r)
	req := NetworkRequest{Kind: "pppoe_server", Values: map[string]string{"name": "customers", "interface": "ether3", "gateway_cidr": "192.168.70.1/24", "pool_start": "192.168.70.10", "pool_end": "192.168.70.200", "dns": "1.1.1.1", "rate_limit": "10M/20M"}}
	if _, err := a.PreviewNetwork(context.Background(), "biz-1", "dev-1", req); err == nil {
		t.Fatal("existing unrouted subscriber pool reused")
	}
	req.Values["gateway_cidr"] = "192.168.72.1/24"
	req.Values["pool_start"] = "192.168.72.10"
	req.Values["pool_end"] = "192.168.72.200"
	review, err := a.PreviewNetwork(context.Background(), "biz-1", "dev-1", req)
	if err != nil {
		t.Fatal(err)
	}
	req.Confirmation = review.Confirmation
	result, err := a.ApplyNetwork(context.Background(), "biz-1", "dev-1", req)
	if err != nil || !result.Verified {
		t.Fatalf("service configuration failed: %+v %v", result, err)
	}
}
func TestNetworkReviewApplyReplayAndRollback(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			r := networkFixture()
			a := quickConfigApplier(r)
			req := NetworkRequest{Kind: "subscriber", ID: "*3", Values: map[string]string{"name": "alice", "password": "new-secret", "profile": "business", "disabled": "yes"}}
			review, e := a.PreviewNetwork(context.Background(), "biz-1", "dev-1", req)
			if e != nil {
				t.Fatal(e)
			}
			for _, cmd := range r.commands {
				if strings.Contains(cmd, " set ") || strings.Contains(cmd, " add ") || strings.Contains(cmd, ":execute") {
					t.Fatal("preview mutation")
				}
			}
			raw, _ := json.Marshal(review)
			if strings.Contains(string(raw), "secret") {
				t.Fatal("secret in review")
			}
			req.Confirmation = review.Confirmation
			r.failWrite = fail
			result, e := a.ApplyNetwork(context.Background(), "biz-1", "dev-1", req)
			if fail {
				if e == nil || !result.RolledBack || r.objects["subscribers"]["*3"]["password"] != "old-secret" {
					t.Fatalf("rollback failed: %+v %v", result, e)
				}
			} else {
				if e != nil || !result.Verified || r.guard {
					t.Fatalf("apply failed: %+v %v", result, e)
				}
			}
			if _, e = a.ApplyNetwork(context.Background(), "biz-1", "dev-1", req); e == nil {
				t.Fatal("approval replay accepted")
			}
		})
	}
}
func TestNetworkReviewTenantInputAndRouterStateBound(t *testing.T) {
	for _, test := range []string{"tenant", "input", "router"} {
		t.Run(test, func(t *testing.T) {
			r := networkFixture()
			a := quickConfigApplier(r)
			req := NetworkRequest{Kind: "ppp_profile", ID: "*1", Values: map[string]string{"rate-limit": "10M/20M"}}
			review, e := a.PreviewNetwork(context.Background(), "biz-1", "dev-1", req)
			if e != nil {
				t.Fatal(e)
			}
			req.Confirmation = review.Confirmation
			biz := "biz-1"
			switch test {
			case "tenant":
				biz = "other"
			case "input":
				req.Values["rate-limit"] = "20M/40M"
			case "router":
				r.objects["ppp_profiles"]["*1"]["rate-limit"] = "15M/30M"
			}
			r.commands = nil
			if _, e = a.ApplyNetwork(context.Background(), biz, "dev-1", req); e == nil {
				t.Fatal("changed review accepted")
			}
			for _, cmd := range r.commands {
				if strings.Contains(cmd, " set ") || strings.Contains(cmd, " add ") || strings.Contains(cmd, ":execute") {
					t.Fatal("invalid preview mutated")
				}
			}
		})
	}
}
func TestNetworkWANAndWirelessPlans(t *testing.T) {
	r := networkFixture()
	req := NetworkRequest{Kind: "wan", Values: map[string]string{"mode": "failover", "wan1": "ether1", "wan2": "ether2", "gateway1": "192.0.2.1", "gateway2": "198.51.100.1"}}
	steps, _, e := buildNetworkPlan(context.Background(), r, req)
	if e != nil {
		t.Fatal(e)
	}
	if len(steps) != 6 {
		t.Fatalf("WAN steps=%d", len(steps))
	}
	if steps[4].Values["distance"] != "2" {
		t.Fatal("backup priority missing")
	}
	raw, _ := json.Marshal(steps)
	if strings.Contains(string(raw), "per-connection-classifier") || strings.Contains(string(raw), "routing-mark") {
		t.Fatal("Hotspot-incompatible PCC")
	}
	wifi := NetworkRequest{Kind: "wireless", ID: "*6", Values: map[string]string{"ssid": "Business", "country": "myanmar", "password": "test-wifi-secret", "disabled": "no"}}
	steps, _, e = buildNetworkPlan(context.Background(), r, wifi)
	if e != nil || len(steps) != 2 {
		t.Fatalf("wireless plan %v", e)
	}
}
func TestNetworkRejectsUntrustedOperations(t *testing.T) {
	for _, req := range []NetworkRequest{{Kind: "raw", Values: map[string]string{"command": "/system reboot"}}, {Kind: "queue", Values: map[string]string{"name": "x"}}, {Kind: "ppp_profile", ID: "*1; /system reboot", Values: map[string]string{"rate-limit": "10M/20M"}}, {Kind: "pool", ID: "*2", Values: map[string]string{"ranges": "not-ip"}}} {
		if _, _, e := buildNetworkPlan(context.Background(), networkFixture(), req); e == nil {
			t.Fatalf("unsafe request accepted %+v", req)
		}
	}
}

func TestNetworkReadbackCanonicalForms(t *testing.T) {
	for _, v := range []struct{ field, a, b string }{{"authentication", "chap;mschap2", "mschap2,chap"}, {"max-limit", "5000000/10000000", "5M/10M"}, {"dns-server", "1.1.1.1;8.8.8.8", "8.8.8.8,1.1.1.1"}} {
		if !networkEqual(v.field, v.a, v.b) {
			t.Fatalf("canonical mismatch: %s", v.field)
		}
	}
	if networkEqual("max-limit", "5000000/10000000", "10M/20M") {
		t.Fatal("different rates accepted")
	}
}

func TestNetworkWANAtomicApplyUpdateAndHealthRollback(t *testing.T) {
	r := networkFixture()
	a := quickConfigApplier(r)
	req := NetworkRequest{Kind: "wan", Values: map[string]string{"mode": "failover", "wan1": "ether1", "wan2": "ether2", "gateway1": "192.0.2.1", "gateway2": "198.51.100.1"}}
	apply := func() (NetworkResult, error) {
		review, e := a.PreviewNetwork(context.Background(), "biz-1", "dev-1", req)
		if e != nil {
			return NetworkResult{}, e
		}
		req.Confirmation = review.Confirmation
		return a.ApplyNetwork(context.Background(), "biz-1", "dev-1", req)
	}
	if result, e := apply(); e != nil || !result.Verified {
		t.Fatalf("WAN apply %+v %v", result, e)
	}
	found := false
	for _, cmd := range r.commands {
		if strings.HasPrefix(cmd, "{ ") {
			found = true
		}
	}
	if !found {
		t.Fatal("WAN changes were not staged together")
	}
	req.Values["mode"] = "balance"
	if result, e := apply(); e != nil || !result.Verified {
		t.Fatalf("WAN update %+v %v", result, e)
	}
	before := networkHash(r.objects)
	req.Values["mode"] = "failover"
	r.failPing = true
	if result, e := apply(); e == nil || !result.RolledBack || networkHash(r.objects) != before {
		t.Fatalf("failed WAN probe did not restore prior state: %+v %v", result, e)
	}
}
