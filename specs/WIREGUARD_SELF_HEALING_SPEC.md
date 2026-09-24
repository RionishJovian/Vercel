# Mega Magic — Feature Spec: WireGuard Tunnel Self-Healing

**Status:** Draft for approval — not yet a build target
**Author:** Hermes lead agent, on behalf of the Kingdom
**Sources:** Live CCR2004 outage (Sep 2026) · official RouterOS WireGuard/diagnostics docs · MKController/RemoteWinBox competitive review
**Related:** `IMPLEMENTATION_SPEC.md` (parent spec — telemetry/alerting sections)

---

## 1 | Problem statement

A fleet router behind CGNAT/NAT silently stops answering its WireGuard tunnel. The hub keeps
dialing the peer's cached endpoint; the peer never re-dials. Result: manager plane (WebFig/SSH/API)
unreachable, and **no automated recovery** — only a site visit or manual peer reset fixes it.

Proven live: a CCR2004 peer sat at 24h+ stale handshake while the hub was healthy and actively
sending. Control-plane looked fine; data-plane was dead. This is the #1 recurring cost and the
defining reliability gap of the product.

---

## 2 | Grounding — what the docs actually say

- WireGuard is **UDP-only**; the initiator's NAT pinhole expires in ~30s without keepalive.
  → `PersistentKeepalive = 25s` on every peer, or tunnels die silently.
- A *stale conntrack pinhole* can outlive reality (WAN/CGNAT re-map): replies route nowhere.
  → Busted by **disabling the peer > UDP timeout** or **rotating the WireGuard listen-port**.
- RouterOS management doors (WebFig/SSH/API/REST) all proxy over the same tunnel → an offline
  tunnel takes every remote door down with it. Offline is the *common* state, not the error case.
- Health lives on two planes: **control-plane** (`wg show` handshake age, rx/tx) and **data-plane**
  (reachability/ping/latency). A healthy control-plane does NOT imply a reachable device.

Failure mode → recovery mapping (the playbook, condensed):
| Fault | Evidence | Auto-recovery action |
|---|---|---|
| CGNAT pinhole expired | keepalive missing, no inbound reply | ensure keepalive; re-send handshake |
| Stale conntrack pinhole | keepalive present, no reply in | disable peer > UDP timeout, re-enable |
| Listener wedged | hub sends, peer silent | rotate WG listen-port |
| Peer/router offline | no handshake, no ping | alert + escalate (site/Netwatch) |

---

## 3 | Scope

**In:** automatic peer recovery on the hub/probe service · provisioning keepalive enforcement ·
graceful WebFig-offline degradation · the acceptance tests that prove a stuck peer recovers without
site access.

**Out (deliberately):** hardware-level watchdog/reboot, MTU/path-MTU tuning, policy routing
steering (future work).

---

## 4 | Design

### 4.1 Provisioning change (do FIRST — cheap, prevents most cases)
The bootstrap/provisioning artifact (`provisioning/*.rsc` → `provisioner-service`) must set on every
peer: `PersistentKeepalive = 25s`, and verify `chain=input protocol=udp dst-port=<listen>` accept
rule sits **above** any drop rule on the router firewall. Enforced at `provision.verify`.

### 4.2 Peer self-healing (the core feature)

Repeat loop, driven by the existing 25s heartbeat + 60s telemetry pipeline:

1. **Detect** — from `peer.handshake` / telemetry `tunnel_state`:
   - handshake age `> T_down` (default 180s) **and** data-plane unreachable → candidate.
   - Distinguish *stale-pinhole* (keepalive online, no reply) from *peer-offline* (no handshake,
     no ping) — **they get different actions.**
2. **Escalate severity:** 3 missed heartbeats = `stale` (amber → red at threshold); log started.
3. **Recover (in order, each followed by verify):**
   - a) Send fresh handshake / re-probe (cheap, clears most transient).
   - b) **Disable peer > UDP timeout, then re-enable** (busts stale conntrack pinhole).
   - c) **Rotate WireGuard listen-port** on the hub listener for that peer path (busts wedged
     listener) — only when b fails.
4. **Verify** — handshake age resets AND data-plane reachable. Record time-to-recover.
5. **Escalate** — if a,b,c all fail within `T_escalate` (default 10 min): raise incident, webhook
   (existing notifier), no further auto-attempts without backoff.
6. **Backoff guard** — rate-limit recovery attempts per peer (e.g. max 3 cycles / 15 min) so a
   legitimately-dead offline peer doesn't thrash the hub or burn NATs subjects.

NATS subjects (extends parent spec):
```
{tenant_prefix}peer.recover          # recovery pass trigger
{tenant_prefix}peer.recovery.result  # action taken + verify outcome + time-to-recover
{tenant_prefix}peer.escalate         # all auto-recovery exhausted -> incident
```

### 4.3 Graceful WebFig-offline degradation
When `tunnel_state = down` for a router: WebFig/SSH/API row renders **"Router offline — will
reconnect"** (not hard 404/empty). Proxy allocations for that peer are *dropped* (matches parent
spec: packet-drop > 5% rule). Reconnect is automatic via 4.2.

---

## 5 | Acceptance criteria (the pass/fail for THIS feature)

1. **Stuck-peer recovery:** Given a peer whose conntrack pinhole is stale (keepalive online, no
   reply), the hub recovers it **automatically** (disable→re-enable) and handshake age resets to
   < 30s, **in ≤ 5 min, with zero site access**. ✔/✘
2. **Offline peer does not thrash:** a genuinely dead peer is escalated to an incident and auto-
   recovery backoff engages (no unbounded retry loop). ✔/✘
3. **Provisioning enforcement:** a router provisioned via Mega Magic has keepalive=25s + firewall
   accept-above-drop verified at `provision.verify`, else provisioning FAILS loudly. ✔/✘
4. **Graceful degradation:** WebFig UI shows "offline — will reconnect" (not an error page) when
   the tunnel is down. ✔/✘
5. **Escalation:** when all in-band recovery fails, a `peer.escalate` -> incident + webhook fires
   within `T_escalate`. ✔/✘
6. **Observability:** every recovery pass emits `peer.recovery.result` with action, verify result,
   and time-to-recover (dashboard-able, feeds MRR/health story). ✔/✘

---

## 6 | Test profile (QA — extends parent `Alpha`/`Gamma` style)

**Test Profile Sigma: Stuck Peer Self-Heal**
- Given: hub with a simulated stale conntrack pinhole for `peer X` (keepalive online, no inbound reply).
- When: heartbeat ages past `T_down`.
- Then: (a) disable>re-enable runs, (b) handshake resets < 30s, (c) no incident raised, (d) total
  time-to-recover recorded and ≤ 5 min.

**Test Profile Tau: Offline Peer Escalation**
- Given: peer X fully dead (no handshake, no ping).
- When: recovery attempts exhaust across a,b,c.
- Then: `peer.escalate` → incident, webhook fires, backoff prevents further auto-attempts for the
  backoff window.

---

## 7 | Implementation checklist

- [ ] Provisioning: add keepalive=25s + firewall rule to bootstrap + `provision.verify` check
- [ ] Probe service: classify stale-pinhole vs peer-offline (handshake + data-plane signals)
- [ ] Recovery loop: disable>re-enable, then listen-port rotate, with verify + backoff
- [ ] `peer.recover` / `peer.recovery.result` / `peer.escalate` NATS subjects
- [ ] `T_down`, `T_escalate`, backoff window as config (feature-flag toggleable)
- [ ] Monitoring-service: time-to-recover metric + handshake-age SLA alerting (parent spec)
- [ ] UI: WebFig/SSH/API "offline — will reconnect" state
- [ ] QA profiles Sigma (stuck peer) + Tau (offline escalation) green

---

*Approve before treating as a build target. Lands first in `~/mega-magic-project/specs/` next to
`IMPLEMENTATION_SPEC.md`; the parent spec's telemetry/alerting rows are then reconciled to point
here.*