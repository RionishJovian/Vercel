# Feature gaps — ranked, verified against source 2026-10-04

Product goal: **one place, one click, one script.** A user who buys a voucher
should never need to understand WireGuard, RADIUS, MTU or NAT.

Every gap below was verified by grep against the live source, not assumed.

---

## CRITICAL — blocks revenue or enrollment

### C1. No RADIUS server exists
`internal/` writes `/radius address=… service=hotspot` to the router
(`gateway_setup.go:180`) and sets `use-radius=yes`, but **nothing in the
codebase listens on 1812**. Verified: 0 refs to `1812`.

Result: HotSpot is configured to authenticate against a server that isn't
there. **Every guest login fails.**

Fix: either build a RADIUS server (go.mod already carries FreeRADIUS deps) or
stop writing `use-radius=yes` and use RouterOS local hotspot users.

### C2. Provisioned routers get no NTP or clock
Nothing in `internal/` sets `/system/ntp/client` or `/system/clock`.
`internal/hub/client.go:80` signs requests with `time.Now()`, and the Hub
verifies the timestamp.

Result: a fresh AX2 with no WAN sits at a stale date; its signed enrollment
request is rejected. **This is the most likely remaining enrollment blocker.**

Fix: the one-paste script must set NTP and correct the clock before enrolling.

### C3. Connection tracking never cleared after NAT changes
Manual flags this a `danger`: rules "may seem to be not functioning correctly
until the connection entry expires." Verified: 0 refs to connection tracking.

Fix: run `/ip/firewall/connection tracking remove` after `network_control.go`
applies any `nat` record.

### C4. Open resolver risk
`quick_config.go:124` sets `allow-remote-requests=yes`. No WAN drop rule for
port 53 anywhere. Verified: 0 refs to `dst-port=53`.

RouterOS's default firewall covers this today, but the app writes its own
firewall rules — one custom input rule could expose it.

Fix: emit the two WAN-drop rules whenever remote DNS is enabled.

---

## HIGH — user-visible friction

### H1. `login-by` not pinned at profile creation
`network_plan.go:583` correctly *rejects* anything but `http-chap,mac` /
`http-chap` when a customer edits a profile. But `gateway_setup.go:187`
creates the profile without forcing the value.

Fix: pin `login-by=http-chap,mac` on creation.

### H2. Single subnet per gateway, no segmentation
One `192.168.{50+i}` per business. No `/interface/vlan` anywhere.

Not urgent — it is a clean design — but it caps the platform at ~256
businesses and means staff and guests share one network.

### H3. No self-service recovery path
Enrollment, DNS, clock and RADIUS fixes all require someone who knows RouterOS.
That contradicts "one-click" directly. Every gap above becomes a product
defect until the app can apply it remotely.

---

## MEDIUM — missing capability

### M1. No router log capture
Zero real `/log` reads. When a tunnel drops or an interface flaps, the evidence
sits on the router and is never collected. Incidents have no router-side detail.

### M2. No per-voucher data cap
Vouchers control **speed** (queue rules), not **volume**. `Mikrotik-Recv-Limit`
/ `Xmit-Limit` (vendor 14988) deliver per-session data caps on Access-Accept.

Customers will ask "how much data did I use". There is no answer today.

### M3. Jellyfin reachability undefined
LAN-only media is the confirmed architecture, but no walled-garden or firewall
rule lets guest devices reach Jellyfin on the owner's PC. The path is
designed and not built.

---

## LOW — housekeeping

### L1. MikroTik Cloud left enabled
`ddns-enabled` publishes the router IP to MikroTik and phones home. We have our
own Hub; this is a second, unneeded dependency.

### L2. MNDP broadcasts identity
Fine to leave on, but it announces board model and RouterOS version to every
device on the segment.

---

## Correctly absent — do NOT add

These conflict with the architecture (one RouterBOARD, one WAN, one flat guest
subnet, SSH-provisioned). Their absence is a decision, not a gap.

| Feature | Why not |
|---|---|
| VLAN / `vlan-filtering` | Single segment is correct at current scale |
| Bonding, VRRP, MCLAG | Standalone customer-premises units |
| Load balancing / failover | One WAN by design |
| rsync / IPsec on routers | Portal files ship over SSH; no daemon needed |
| DLNA | Contradicts the portal model |
| MikroTik containers | No workloads on customer routers |
| detect-internet | Silently rewrites routes and DHCP |
| REST/API transport | Plaintext password flow; SSH is stronger |

---

## Definition of done for "one-click"

A new customer should be able to: generate a script → paste it into WinBox →
click one button in the portal → get internet. No router knowledge at any step.

C1–C4 are prerequisites. H3 is the goal.