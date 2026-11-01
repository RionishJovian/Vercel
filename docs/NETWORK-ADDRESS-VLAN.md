# IP address and VLAN editing

## Status (2026-11-01): enabled in production (`MM_NETWORK_ADDRESS_CONTROLS_ENABLED=true`)
Verified on a real hAP ax3 (the owner's friend's router, with their agreement), each
change previewed, reviewed and approved, then checked by an independent re-read of
the router against a baseline taken before the test:
- 22 previews: every protection fired with a clear reason on real data (uplink, Hub tunnel `wg`, bridge member, serving ports, dynamic addresses, overlap, Hub range, public, prefix, duplicate/in-use/nested VLANs).
- 6 real applies, all verified, none rolled back: create VLAN, change its ID, enable it, add an address on it, change that address, disable the VLAN. The only router differences were the intended ones (plus the automatic connected route while the VLAN was enabled).
- Backup before each apply, restore timer set and cancelled, readback verified.

**Not yet proven on real hardware:** rollback after a real failure, and the restore timer firing when the connection drops (both covered only by the simulated-router tests). Test these on the lab hAP ax2 when it is powered.

**Left on that router by the test:** disabled VLAN `mm-test-4001` (ID 4002, ether3), address `192.168.123.2/24` on it, and 6 sets of `mm-snap-...-net-20261101-...` backup/export files. The app has no delete yet.

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
- a port that serves customers (DHCP server, Hotspot, PPPoE service): its address cannot be changed or disabled
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

## Changing the ID or port of a VLAN in use (confirm or revert)

A VLAN that carries an address or a service can have its ID or parent port changed.
Its addresses and services follow it, but everything on it is cut off unless the
switch or access point carries the new ID on the connecting port, so the change is
never final on its own:

1. Preview shows a strong warning. Apply writes the change and checks it back.
2. The restore timer **on the router** stays armed for 5 minutes instead of being
   cancelled. The page shows a countdown with **Keep these changes** and **Revert now**.
3. Test with a real client. **Keep** cancels the timer on the router and checks it is
   gone; **Revert now** writes the old values back and checks them. If nobody presses
   anything, the router reverts by itself. If the API restarts meanwhile, the router
   still reverts: the safe default.
4. Keep is refused with the reason if the timer already ran (the router has returned
   to the old settings) or if the router no longer matches the change. Keep and Revert
   are never blocked by the write switches, so a pending change can always be resolved.
5. Another change is refused while one is waiting. A VLAN that carries the internet
   uplink is still refused, and the audit log records applied, kept and reverted.

Not yet proven on real hardware: the router firing the timer by itself. Plan: test it on
the disabled test VLAN with the owner's approval.

## Deleting what the app created

An IP address or a VLAN can be deleted from its record page (**Delete this record**),
with the same preview, consent and backup as any change. Only records carrying the
app's `mega-magic` comment qualify: new addresses and, from this release, new VLANs
are marked when created. Addresses and VLANs created earlier by a person, or by this
app before VLANs were marked, are not deleted here (the server refuses them); remove
those where they were made.

Refused even when marked: automatic (dynamic) addresses, the Hub tunnel, addresses
on a port that serves customers or carries the internet uplink, and a VLAN that is in
use (it names what uses it). The router keeps a restore timer that re-creates the
record, only if it is missing, with the same fields; the apply verifies the record is
gone and a failed apply is rolled back and verified the same way.

Still to come: removing a whole customer network in dependency order, and tidying the
app's own old `mm-snap-*` backup files on the router.
