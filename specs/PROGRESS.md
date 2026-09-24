# Mega Magic — Progress Status

**Updated:** 2026-09-24 · **Repo:** `mega-magic-project` (V-language VSL, ~6,300 LOC) · **Tree:** clean, vet+build green

This file is the canonical, git-tracked status snapshot. Re-run it after any significant milestone. It survives context compaction — treat it as source of truth for "where the project stands."

---

## 1 · Git history (reverse, oldest→newest)

| Commit | Feature |
|---|---|
| `33041dd` | Initial commit: Mega Magic Go backend (scaffold: DB schema, auth, handlers, repos) |
| `4a54662` | **probe** — reachability classifier (up/stale/offline/unknown) |
| `8565df6` | docs: WireGuard self-healing spec |
| `bc467c3` | **monitoring** — evaluate + cache + NATS fan-out |
| `4a85fad` | **peer-state endpoint** — probe/monitoring wired into server |
| `479b9d8` | **evaluator** — stale/offline → deduped `peer_offline` incident |
| `48e56f3` | **sweep** — autonomous 60s push loop (no NATS subscription; polls all peer keys) |
| `e423e04` | **recovery** — a/b/c self-healing ladder + backoff + escalate (subscribes `mm.*.peer.recover`) |

---

## 2 · DONE — the self-healing core (verified live, not just compiled)

All six units verified against the **running binary** with real NATS events (offline peer → incident → recovery ladder → backoff suppression, **zero HTTP requests**).

- **probe** — `ClassifyPeerState()`: up/stale/offline/unknown from handshake-age (tDown) + data-plane tunnel/WAN state
- **monitoring** — `EvaluatePeer`/`StateForPeer` (30s cache), publishes `monitor.peer.state`
- **endpoint** — `GET /api/v1/peers/:peerId/state` (owner-scoped, UUID-shaped 404 guard)
- **evaluator** — `mm.*.monitor.peer.state` → deduped `peer_offline` incident (offline=critical, stale=warning)
- **sweep** — autonomous 60s pass over all peer keys via `ListAllPeerKeys()`, re-evaluating each (no NATS subscription)
- **recovery** — subscribe `peer.recover` → ladder: reprobe → reset_peer → rotate_listen_port, each emits `peer.recovery.result`(action+ok+ttr); RateLimit backoff (max 3 / 15m); `peer.escalate` when exhausted

**Spec ACs met:** #1 stuck-peer recovery · #2 no-thrash backoff · #5 escalation · #6 observability (recovery.result). **Not met:** #3 provisioning/keepalive enforcement, #4 UI offline state.

---

## 3 · SCAFFOLDED (initial-commit backend, code exists, partly live)

- **Persistence (15 repos):** alert_policy, audit, device, incident, nats_subject, peer_heartbeat, portal_config, portal_session, quota, telemetry_sample, user, voucher, webhook (+ scan_helpers, db)
- **HTTP (7 handlers):** auth (register/login/refresh/me), router (CRUD), telemetry (ingest), incident (+alert-policies), webhook (+deliveries), voucher (CRUD+redeem), monitor (peer-state)
- **Live routes:** `/health`, `/ready`, `/version` + 11 protected endpoints
- **DB:** Supabase-style Postgres in Docker (`mega_magic_db_dev`, uuids, RLS-ready tenant columns)

---

## 4 · NOT BUILT / CAVEATS

- **9 of 22 modules empty stubs:** `ai`, `billing`, `fleet`, `portal`, `provisioner`, `router`, `topology`, `voucher`, `vps` (note: `voucher`/`router` *handler+repo* exist; the *module dirs* are empty)
- **Zero tests** — `go test` → `[no test files]` in every module. Most urgent risk as surface grows.
- **No provisioning/remote-exec** — `provisioner` empty; **nothing consumes `peer.recover.cmd`** (recovery worker publishes hub intents into a void; can't yet actually fix a router)
- **No hub transport** — no SSH/wg/exec plumbing anywhere in the codebase

---

## 5 · Sizing (honest)

**Overall: ~30–40% of full Mega Magic scope.** The self-healing *brain* is substantially done and proven — but the *arms/legs* (fleet dashboard, provisioner, billing, portal, AI, topology) are unstarted.

Rough module health:
- Self-healing data plane (probe→monitoring→evaluator→sweep→recovery): **~done, verified**
- Tenant/auth/device persistence + HTTP CRUD surface: **~70% scaffolded**
- Feature modules (9 empty dirs): **0%**

---

## 6 · Lead flags (audit, not actions)

1. **Zero-test gap** — live-only verification won't scale once billing/portal/AI land. Recommended: stand up `go test` harness for pure functions (classifier, recovery backoff) with the next real unit (fleet).
2. **Provisioner = functional bottleneck + highest risk** — it's what makes self-healing actually *fix* a router, but touching the live production CCR2004 needs a safety-first design (dry-run → manual approve-gate → verify-before-apply) before any code.

---

## 7 · Next unit (as of this status)

**`fleet` aggregate + peer-health endpoint** (read-only, reuses probe/monitoring/device_repo, surfaces AC#6 data). Provisioner queued as its own scoped safety-first mini-project.