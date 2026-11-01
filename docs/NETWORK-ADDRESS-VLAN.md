# IP address and VLAN editing

Two typed operations on the Router tab, under Internet setup: **IP and DHCP →
IP addresses** and **VLANs**. They use the same guarded path as every other
network change (preview, router-side backup, a restore timer that undoes the
change within three minutes if the connection is lost, per-step readback and
rollback). They are **off by default** and must be switched on after a router
test.

## What they can and cannot do
Add or change an IP address, and enable or disable it. Create a VLAN, and enable,
disable or reshape one that is unused. Nothing is deleted.

Refused, by design (each has a test that fails if the rule is removed):
- the Magic Hub tunnel, loopback, and any interface that is not a port, bridge, bond or VLAN
- an internet uplink: DHCP client, PPPoE client, or the interface a default route leaves through; if this cannot be determined the change is refused
- a port that serves customers (DHCP server, Hotspot, PPPoE service): its address cannot be changed or disabled; a VLAN in use keeps its ID and port
- automatically assigned (dynamic) addresses, moving an address to another port, renaming a VLAN
- anything outside private ranges (10/8, 172.16/12, 192.168/16), prefixes outside /8 to /30, the network and broadcast addresses, anything overlapping an existing address or static route, and the whole Hub range 10.77.0.0/24
- a port that is a bridge member (address the bridge instead), a VLAN on a VLAN or on a tunnel, a duplicate VLAN ID on the same port, and a VLAN name already used by any interface

## The switch: `MM_NETWORK_ADDRESS_CONTROLS_ENABLED`
Set in `.env.production`, then recreate the API. `MM_NETWORK_CONTROLS_ENABLED=true` must also be set (it is).

| Value | Preview (read the router, show the plan) | Apply (change the router) | What the page shows |
|---|---|---|---|
| unset / anything else | refused (503) | refused (503) | form locked with the reason |
| `preview` | allowed | **always refused (503)** | form usable, "Preview only" banner |
| `true` | allowed | allowed | form usable |

`preview` exists to check the plans against a real router with no possible change:
the server refuses every apply in that mode (a test fails if that ever stops being
true). Only the exact values `preview` and `true` have any effect.

Router test plan (lab hAP ax2, one step at a time, using Preview first):
1. Add `192.168.50.1/24` to a spare port. Check the readback, then `/ip address print` on the router.
2. Edit it to `192.168.50.2/24`, disable it, enable it.
3. Try each refusal: the uplink port, the tunnel, an overlapping network, `10.77.0.9/24`, a port with a DHCP server.
4. Create VLAN 100 on a spare port, disable it, change its ID, then give it an address.
5. Pull the connection mid-apply (or use a deliberately failing step) and confirm the restore timer undoes the change.

## Also fixed in the same change
The existing network forms posted fields the server rejects ("unexpected network
input"): creation suggestions and a record's other properties. Forms now send only
the fields they show, and `contract.test.ts` checks every form against the server's
allowed-field list read from `network_plan.go`.
