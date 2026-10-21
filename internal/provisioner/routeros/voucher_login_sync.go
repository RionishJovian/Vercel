package routeros

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// VoucherLoginState is the RouterBOARD's own accounting for one issued
// voucher: usage counters, enable state and the live session's client MAC.
// The sync reads it back so the app reports what the router actually served.
type VoucherLoginState struct {
	VoucherID     string
	Exists        bool
	Disabled      bool
	UptimeSeconds int64
	BytesUsed     int64
	ActiveMAC     string
}

const voucherSyncMarker = "__MM_VS__"

// voucherSyncScript emits, for each voucher, a marker line followed by the
// router's own counters. Values are read with /ip hotspot user get so no
// transposed table parsing is involved.
func voucherSyncScript(refs []VoucherLogin) string {
	var b strings.Builder
	b.WriteString("{ ")
	for _, l := range refs {
		code := routerOSQuoted(l.Code)
		id := routerOSQuoted(l.VoucherID)
		b.WriteString(":put \"" + voucherSyncMarker + id + "__start\"; :local u [/ip hotspot user find name=" + code + "]; ")
		b.WriteString(":if ([:len $u] = 0) do={ :put \"absent\"; } else={ ")
		b.WriteString(":put (\"disabled=\" . [/ip hotspot user get $u disabled]); ")
		b.WriteString(":put (\"uptime-used=\" . [/ip hotspot user get $u uptime-used]); ")
		b.WriteString(":put (\"bytes-in=\" . [/ip hotspot user get $u bytes-in]); ")
		b.WriteString(":put (\"bytes-out=\" . [/ip hotspot user get $u bytes-out]); ")
		b.WriteString(":local a [/ip hotspot active find user=" + code + "]; ")
		b.WriteString(":if ([:len $a] = 0) do={ :put \"mac=none\"; } else={ :put (\"mac=\" . [/ip hotspot active get $a mac-address]); } ")
		b.WriteString("}; ")
	}
	b.WriteString("}")
	return b.String()
}

// ReadVoucherLogins reads each voucher's live state from the gateway in one
// bounded script. Vouchers the router never served report Exists=false.
func (a *DeviceApplier) ReadVoucherLogins(ctx context.Context, businessID, deviceID string, refs []VoucherLogin) ([]VoucherLoginState, error) {
	if len(refs) == 0 || len(refs) > 200 {
		return nil, fmt.Errorf("sync between 1 and 200 voucher logins at once")
	}
	r, err := runnerForDevice(ctx, a.src, a.factory, a.timeout, businessID, deviceID)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	return readVoucherLogins(ctx, r, refs)
}

func readVoucherLogins(ctx context.Context, r CommandRunner, refs []VoucherLogin) ([]VoucherLoginState, error) {
	byID := map[string]VoucherLogin{}
	for _, l := range refs {
		byID[l.VoucherID] = l
	}
	raw, err := r.Run(ctx, voucherSyncScript(refs))
	if err != nil {
		return nil, fmt.Errorf("cannot read voucher login state: %w", err)
	}
	states := make([]VoucherLoginState, 0, len(refs))
	current := -1
	var cur *VoucherLoginState
	flush := func() {
		if cur != nil {
			states = append(states, *cur)
			cur = nil
		}
	}
	for _, rawLine := range strings.Split(raw, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(rawLine, "\r"))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, voucherSyncMarker) && strings.HasSuffix(line, "__start") {
			id := strings.TrimSuffix(strings.TrimPrefix(line, voucherSyncMarker), "__start")
			ref, ok := byID[id]
			if !ok {
				return nil, fmt.Errorf("unexpected voucher marker %q", id)
			}
			flush()
			cur = &VoucherLoginState{VoucherID: ref.VoucherID}
			current++
			_ = current
			continue
		}
		if cur == nil {
			return nil, fmt.Errorf("voucher state outside a marker")
		}
		if line == "absent" {
			cur.Exists = false
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("unreadable voucher state line %q", line)
		}
		switch key {
		case "absent":
			cur.Exists = false
		case "disabled":
			cur.Exists = true
			cur.Disabled = value == "yes" || value == "true"
		case "uptime-used":
			cur.UptimeSeconds = int64(parseUptimeSeconds(value))
		case "bytes-in", "bytes-out":
			n, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
			if err != nil || n < 0 {
				return nil, fmt.Errorf("invalid %s %q", key, value)
			}
			cur.BytesUsed += n
		case "mac":
			if value != "none" && value != "" {
				cur.ActiveMAC = strings.ToUpper(value)
			}
		default:
			return nil, fmt.Errorf("unknown voucher state key %q", key)
		}
	}
	flush()
	if len(states) != len(refs) {
		return nil, fmt.Errorf("router reported %d of %d vouchers", len(states), len(refs))
	}
	return states, nil
}
