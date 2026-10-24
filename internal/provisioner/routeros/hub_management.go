package routeros

import (
	"context"
	"fmt"
	"net"
	"strings"
)

// Health is a bounded projection, never an export or credential-bearing log.
func readHubHealth(ctx context.Context, r CommandRunner) map[string]string {
	fields := []string{"cpu-load", "free-memory", "total-memory", "uptime", "version", "board-name"}
	command := ""
	for _, f := range fields {
		command += `:put (` + routerOSQuoted(f+": ") + ` . [:tostr [/system resource get ` + f + `]]); `
	}
	out, err := r.Run(ctx, command)
	if err != nil || len(out) > 4096 {
		return nil
	}
	result := map[string]string{}
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		if line == "" {
			continue
		}
		k, v, ok := strings.Cut(line, ": ")
		valid := false
		for _, f := range fields {
			if k == f {
				valid = true
			}
		}
		if !ok || !valid || len(v) > 128 {
			return nil
		}
		if _, duplicate := result[k]; duplicate {
			return nil
		}
		result[k] = v
	}
	if len(result) != len(fields) {
		return nil
	}
	return result
}

// These operations deliberately own a small, explicit scope. "Shield" removes
// one reviewed HotSpot exemption, not MAC spoofing protection or a paid-login
// bypass. No default firewall, WAN, management or voucher rule is adopted.
func buildHubManagementPlan(ctx context.Context, r CommandRunner, req NetworkRequest) ([]networkStep, []string, error) {
	v := req.Values
	if v["disabled"] != "yes" && v["disabled"] != "no" {
		return nil, nil, fmt.Errorf("choose enabled or disabled")
	}
	warnings := []string{"Configuration readback is not a client connectivity test. Existing guest traffic and router management must be checked after applying."}
	steps := []networkStep{}
	read := func(key string) ([]NetworkObject, error) { return readNetworkObjects(ctx, r, key) }
	edit := func(key, id, label string, values map[string]string) error {
		before, e := networkReadOne(ctx, r, key, id, values)
		if e != nil {
			return e
		}
		same := true
		for k, val := range values {
			if !networkEqual(k, before[k], val) {
				same = false
			}
		}
		if !same {
			steps = append(steps, networkStep{Key: key, ID: id, Label: label, Values: values, Before: before})
		}
		return nil
	}
	add := func(key, field, name, label string, values map[string]string, first bool) error {
		n, e := networkCount(ctx, r, networkMenus[key].path+" print count-only where "+field+"="+routerOSQuoted(name))
		if e != nil || n != 0 {
			return fmt.Errorf("reserved object already exists or cannot be checked; no foreign object will be adopted")
		}
		empty := false
		if first {
			n, e := networkCount(ctx, r, networkMenus[key].path+" print count-only")
			if e != nil {
				return e
			}
			empty = n == 0
		}
		steps = append(steps, networkStep{Key: key, SelectorField: field, SelectorValue: name, Label: label, Values: values, First: first, EmptyTable: empty})
		return nil
	}
	switch req.Kind {
	case "firewall":
		if e := networkRequireInterface(ctx, r, v["interface"], false); e != nil {
			return nil, nil, e
		}
		// An input-only invalid-state drop does not disable the rest of a firewall.
		marker := "mm-protection-invalid-" + v["interface"]
		desired := map[string]string{"chain": "input", "action": "drop", "connection-state": "invalid", "in-interface": v["interface"], "comment": marker, "disabled": v["disabled"]}
		if req.ID == "" {
			if v["disabled"] == "yes" {
				return nil, nil, fmt.Errorf("select an installed protection rule to turn it off")
			}
			if e := add("filters", "comment", marker, "Install invalid-connection protection for the selected router port", desired, true); e != nil {
				return nil, nil, e
			}
		} else {
			fields := map[string]string{}
			for k := range desired {
				fields[k] = ""
			}
			fields["dynamic"] = ""
			old, e := networkReadOne(ctx, r, "filters", req.ID, fields)
			if e != nil {
				return nil, nil, e
			}
			for k, val := range desired {
				if k != "disabled" && !networkEqual(k, old[k], val) {
					return nil, nil, fmt.Errorf("protection ownership or configuration changed")
				}
			}
			if networkBool(old["dynamic"]) != "no" {
				return nil, nil, fmt.Errorf("dynamic rules cannot be changed")
			}
			if v["disabled"] == "no" {
				first, e := r.Run(ctx, `:put [:pick [/ip firewall filter find] 0]`)
				if e != nil || strings.TrimSpace(first) != req.ID {
					return nil, nil, fmt.Errorf("protection rule is no longer first; review rule ordering separately")
				}
			}
			if e := edit("filters", req.ID, "Change this Mega Magic invalid-connection rule only", map[string]string{"disabled": v["disabled"]}); e != nil {
				return nil, nil, e
			}
		}
		warnings = append(warnings, "Asymmetric routing can classify legitimate packets as invalid; review the routing design first. This drops invalid IPv4 input on one port only. It is not a complete firewall, IPv6 policy, or an internet access switch. Turning it off preserves all other rules.")
	case "shield":
		// Only disable a selected bypass; never create/re-enable an exemption.
		if req.ID == "" || v["disabled"] != "yes" {
			return nil, nil, fmt.Errorf("select one active bypass to disable; creating or enabling bypasses is not supported")
		}
		old, e := networkReadOne(ctx, r, "bindings", req.ID, map[string]string{"type": "", "comment": "", "disabled": "", "mac-address": "", "address": "", "server": ""})
		if e != nil {
			return nil, nil, e
		}
		if old["type"] != "bypassed" || networkBool(old["disabled"]) != "no" {
			return nil, nil, fmt.Errorf("select an active bypass binding")
		}
		if strings.HasPrefix(strings.ToLower(old["comment"]), "mm-") {
			return nil, nil, fmt.Errorf("managed exemptions require their owning workflow; this control cannot change them")
		}
		old["disabled"] = "yes"
		if e := edit("bindings", req.ID, "Disable the selected HotSpot bypass; the client must use normal portal login", old); e != nil {
			return nil, nil, e
		}
		warnings = append(warnings, "This client may lose its exempt internet access. Active sessions may continue until reauthentication. Other bindings, walled-garden rules and managed grace access are unchanged; this does not prove the entire portal is bypass-proof.")
	case "fairshare":
		name := v["name"]
		if !gatewayNamePattern.MatchString(name) {
			return nil, nil, fmt.Errorf("provide a short fair-share name")
		}
		ip, target, e := net.ParseCIDR(v["target"])
		if e != nil || ip.To4() == nil || !ip.Equal(target.IP) {
			return nil, nil, fmt.Errorf("use a canonical IPv4 customer subnet")
		}
		ones, bits := target.Mask.Size()
		if bits != 32 || ones < 16 || ones > 30 || target.Contains(net.ParseIP("10.77.0.1")) {
			return nil, nil, fmt.Errorf("choose a customer /16 through /30 outside the Hub network")
		}
		if !networkRatePattern.MatchString(v["max-limit"]) {
			return nil, nil, fmt.Errorf("provide total upload/download rates, for example 10M/50M")
		}
		addresses, e := read("addresses")
		if e != nil {
			return nil, nil, e
		}
		interfaces, e := gatewayInterfaces(ctx, r)
		if e != nil {
			return nil, nil, e
		}
		connected := false
		for _, a := range addresses {
			_, prefix, e := net.ParseCIDR(a.Values["address"])
			if e != nil || prefix.String() != target.String() || networkBool(a.Values["disabled"]) != "no" {
				continue
			}
			for _, i := range interfaces {
				if i.Name == a.Values["interface"] && (i.Type == "ether" || i.Type == "bridge" || i.Type == "vlan") {
					connected = true
				}
			}
		}
		if !connected {
			return nil, nil, fmt.Errorf("fair sharing needs the exact subnet of a connected customer LAN")
		}
		filters, e := read("filters")
		if e != nil {
			return nil, nil, e
		}
		for _, f := range filters {
			if f.Values["action"] == "fasttrack-connection" && networkBool(f.Values["disabled"]) != "yes" {
				return nil, nil, fmt.Errorf("active FastTrack can bypass simple queues; review its scope before enabling fair sharing")
			}
		}
		queues, e := read("queues")
		if e != nil {
			return nil, nil, e
		}
		for _, q := range queues {
			if q.ID == req.ID || networkBool(q.Values["disabled"]) == "yes" {
				continue
			}
			if strings.TrimSpace(q.Values["target"]) == "" {
				return nil, nil, fmt.Errorf("another active queue has an unspecified target; review it first")
			}
			for _, raw := range strings.FieldsFunc(q.Values["target"], func(r rune) bool { return r == ',' || r == ';' }) {
				_, other, e := net.ParseCIDR(raw)
				if e != nil || target.Contains(other.IP) || other.Contains(target.IP) {
					return nil, nil, fmt.Errorf("another active queue has an overlapping or unreadable target; review it first")
				}
			}
		}
		base := "mm-fair-" + name
		marker := "mm-fairshare-v1:" + name
		types := []struct{ name, classifier string }{{base + "-up", "src-address"}, {base + "-down", "dst-address"}}
		if req.ID == "" {
			if v["disabled"] == "yes" {
				return nil, nil, fmt.Errorf("select an existing fair-share policy to disable")
			}
			for _, typ := range types {
				if e := add("queue_types", "name", typ.name, "Create dedicated PCQ "+typ.classifier+" sharing", map[string]string{"name": typ.name, "kind": "pcq", "pcq-rate": "0", "pcq-classifier": typ.classifier}, false); e != nil {
					return nil, nil, e
				}
			}
			desired := map[string]string{"name": base, "target": target.String(), "max-limit": v["max-limit"], "queue": types[0].name + "/" + types[1].name, "parent": "none", "comment": marker, "disabled": "no"}
			if e := add("queues", "name", base, "Share total bandwidth fairly between customer IP addresses", desired, false); e != nil {
				return nil, nil, e
			}
		} else {
			old, e := networkReadOne(ctx, r, "queues", req.ID, map[string]string{"name": "", "target": "", "queue": "", "parent": "", "comment": ""})
			if e != nil {
				return nil, nil, e
			}
			if old["name"] != base || old["target"] != target.String() || old["queue"] != types[0].name+"/"+types[1].name || old["parent"] != "none" || old["comment"] != marker {
				return nil, nil, fmt.Errorf("policy identity or scope changed; foreign queues cannot be adopted")
			}
			records, e := read("queue_types")
			if e != nil {
				return nil, nil, e
			}
			for _, typ := range types {
				matches := 0
				for _, o := range records {
					if o.Values["name"] == typ.name {
						matches++
						if o.Values["kind"] != "pcq" || o.Values["pcq-rate"] != "0" || o.Values["pcq-classifier"] != typ.classifier {
							return nil, nil, fmt.Errorf("PCQ definition changed")
						}
					}
				}
				if matches != 1 {
					return nil, nil, fmt.Errorf("owned PCQ type is missing or ambiguous")
				}
			}
			if e := edit("queues", req.ID, "Update this fair-share policy's total bandwidth and state", map[string]string{"max-limit": v["max-limit"], "disabled": v["disabled"]}); e != nil {
				return nil, nil, e
			}
		}
		warnings = append(warnings, "Speeds are total upload/download bandwidth shared per IP, not per person. Use measured ISP capacity. FastTrack and overlapping queue policies must be resolved first. IPv6 is outside this IPv4 policy.")
	default:
		return nil, nil, fmt.Errorf("unsupported Hub operation")
	}
	if len(steps) == 0 {
		return nil, nil, fmt.Errorf("selected settings are already applied")
	}
	return steps, warnings, nil
}

// Overview deliberately avoids the full configuration inventory: two bounded
// reads over the saved tenant-bound connection are enough for the health card.
func (a *DeviceApplier) ReadRouterOverview(ctx context.Context, businessID, deviceID string) (NetworkInventory, error) {
	r, e := runnerForDevice(ctx, a.src, a.factory, a.timeout, businessID, deviceID)
	if e != nil {
		return NetworkInventory{}, e
	}
	interfaces, e := gatewayInterfaces(ctx, r)
	if e != nil {
		return NetworkInventory{}, e
	}
	return NetworkInventory{Interfaces: interfaces, Health: readHubHealth(ctx, r)}, nil
}
