# Guided network setup — 2026-10-03

Follow-up: [Network controls](NETWORK-CONTROLS.md) records the newly implemented
WAN/PPPoE/existing-network editors, two-CHR acceptance and separately held Wi-Fi
rollout. It supersedes the missing-implementation list below, not the recorded
deployment or broader outstanding hardware/guest tests.

The Network setup navigation item opens a task-focused customer-network form.
Gateways remains device registration, Magic Hub connection, health and service
discovery. Its name/DNS dialog links to this form instead of embedding it.
Captive portal remains the guest design/deploy workflow; vouchers and revenue
remain separate business tools. Read-only capability dialogs are not editors.

## Actual behavior

- Select a registered main gateway. Read interfaces, assigned IPv4 prefixes and
  routed destinations through the existing tenant/device-scoped transport.
- Suggest an unused private /24, gateway .1 and DHCP .10–.250; prefer
  192.168.50.0/24, then bounded private alternatives outside the Hub range.
  Any overlapping address or non-default route reserves a candidate, including
  inactive routes. Unreadable/malformed inventory or exhausted choices fails closed.
- Leave physical uplink/customer roles unassigned. Running state or interface
  names do not establish a safe role. The customer must select the correct ports.
- Keep suggested address fields editable under Address settings. Select optional
  VLAN and Hotspot; optionally connect an existing RADIUS server.
- Preview concrete changes, invalidate preview on edits, explicitly confirm
  apply. Apply rereads address/route inventory before any writes, then uses the
  existing device backup, verified readback and owned-object rollback flow.
- No existing uplink, default route, management address or bridge membership is
  replaced. A guest connectivity test and firewall review are still required.

RouterOS routes include connected and configured destinations; the default
route is the catch-all and is excluded from subnet reservations. See
[MikroTik IP Routing](https://help.mikrotik.com/docs/spaces/ROS/pages/328084/IP%20Routing).

## Explicit remaining product work

This first slice does **not** complete the user's requested network-management
suite. Real controls still need implementation and disposable-router acceptance:

- Multi-WAN load balancing and monitored WAN failover: discover ISP links and
  routing tables, validate probes, preserve management traffic, preview routes/
  policy changes, verify failover and recovery, and retain rollback.
- PPPoE client uplinks and subscriber/server provisioning: distinguish these
  tasks, protect secrets, validate pools/profiles/RADIUS, deploy and read back.
- Wi-Fi SSID/security editor for supported modern/legacy radios and CAPsMAN;
  CHR without radios must target actual external APs/controllers.
- Existing LAN/Hotspot edits, VLAN/bridge management, DHCP reservations,
  bandwidth/queues and access policies rather than only initial LAN creation.
- Private authenticated cloud WebFig access (not just service enablement).

These are not inert buttons labeled as supported functions. No guessed port
roles, global routing rewrites, router resets, public management exposure or
automatic live-router deployment is authorized by this UI work.

## Validation boundary

Go race tests cover the RouterOS provisioner, handlers and server. Synthetic
Chromium acceptance covers dedicated navigation, address prefill, explicit port
selection, preview invalidation, verified mock apply and 390px layout. Backend
tests cover address/route collisions, malformed/exhausted suggestions and a route
added between preview and apply. These do not certify a physical guest network
or execute writes on the production CHR.

Read-only compatibility on the registered CHR (RouterOS 7.15.3) found that
`print value-list detail` produces columns for addresses and key=value routes.
Discovery therefore uses fixed `:foreach` plus scalar `get` commands, yielding
one prefix per record. These commands passed against the pinned lab router,
including both management/LAN prefixes and its default route. The read-only
`guest-router-check -device UUID -network-inventory` mode reproduces this check
and refuses devices not named as the CHR lab. No router write is involved.
Interface discovery uses scalar name/type/running reads too, verified against
ether1, loopback and the management WireGuard interface. Loopback is excluded
from uplink choices; loopback and tunnel types are excluded from customer LAN
choices. The live lab has only one Ethernet interface, so it must gain a separate
customer-facing interface/bridge/VLAN before this initial-LAN workflow can be
certified with real guests. Do not invent additional physical ports in the UI.

Deployment: `a0cd08e` on main, API/frontend published to mikromagic.com. Signed-in
live read-only acceptance returned actual interface options and suggested
192.168.50.1/24; protected anonymous defaults request returned 401. Health,
landing and console returned 200. No production router preview or apply ran.
