# Feature gaps — ranked, re-audited against source

Product goal: **one place, one click, one script.** A user who buys a voucher
should never need to understand WireGuard, RADIUS, MTU or NAT.

> **Re-audit note (2026-10-04).** The first pass reported 8 gaps. **Four were
> wrong** — each came from a grep that stopped short of the relevant line. This
> file now records only what was verified by reading the code.

---

## SHIPPED

| ID | Fix | Commit |
|---|---|---|
| C3 | Connection tracking flushed after NAT changes | `1b5f301` |
| C4 | WAN drop rules whenever `allow-remote-requests` is on | `1b5f301` |
| C1 | Unreachable RADIUS refused at preflight | `4d05880` |
| M3 | Per-deployment media host in the walled garden | `6c6041a` |
| M1 | Router log capture for incident evidence | `db8dd23` |

---

## CRITICAL — none outstanding

The original C1 was mis-stated. Vouchers do **not** need RADIUS: the app
creates local hotspot users with `limit-uptime` and `limit-bytes-total`
(`internal/provisioner/routeros/guest_voucher.go:125`), verifies them by
read-back, then enables them.

The real defect was narrower and is now fixed. The setup form offered a
`radius_address` whose own hint said *"this does not install one"*, and
filling it in set `use-radius=yes` with nothing answering — locking out every
guest. Preflight now pings the host from the router and refuses the setup with
an actionable message.

---

## HIGH

### H2. One subnet per gateway
`gateway_defaults.go:107` allocates `192.168.50+` then `10.200.x` — 456
subnets. No `/interface/vlan` anywhere, so staff and guests share one network.

Not urgent. It is a deliberate single-segment design and the limit is far away.

### H3. No self-service recovery path
Enrollment, DNS and RADIUS fixes all require someone who knows RouterOS. Every
fix above becomes a product defect until the app can apply it remotely.
**This is the actual remaining work for "one-click".**

---

## MEDIUM

### M4. Media host is per-deployment, not per-business
`media_host` lives on `PortalDeployment`, so it is set per profile deployment.
It should be stored once per business so redeploying the portal does not
require retyping it.

---

## LOW

### L2. MNDP broadcasts identity
Announces board model and RouterOS version to every device on the segment.
Harmless at current scale.

---

## WITHDRAWN — reported, then disproved

Each of these was in the original list. Each was wrong.

| ID | Claim | Reality |
|---|---|---|
| C2 | No NTP, so enrollment fails | The timestamped HMAC signs **app → Hub**. The router never signs anything to us. WireGuard's replay window is monotonic, not wall-clock. A stale clock is not why enrollment fails. |
| C1(orig) | No RADIUS server exists, so guests cannot log in | Vouchers are local hotspot users. No server needed. See CRITICAL for the narrow real bug. |
| H1 | `login-by` not pinned, PAP possible | Already `login-by: "http-chap,mac"` at profile creation. |
| M2 | No per-voucher data cap | `limit-bytes-total` is set and read back. Time and data caps both work. |
| L1 | MikroTik Cloud enabled | The app never writes `/ip/cloud`. That is the operator's own router setting, not a code gap. |

**Do not act on the withdrawn items.** They were greps that stopped short.

---

## Correctly absent — do NOT add

Conflicting with the architecture: one RouterBOARD, one WAN, one flat guest
subnet, SSH-provisioned.

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

C1–C4 were prerequisites. **H3 is the goal.**

---

## The one remaining blocker

**No real RouterBOARD has ever completed a WireGuard handshake.** The Hub peer
store is empty. Signed enrollment returns the correct Hub public key, but
nothing has ever connected.

Every other item on this list is polish. That handshake is the gap that stops a
customer from actually getting online.