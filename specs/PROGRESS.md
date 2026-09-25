# Mega Magic — Progress Status

**Updated:** 2026-09-25 · **Repo:** `mega-magic-project` (V-language VSL, ~11,600 LOC) · **Tree:** clean, vet+build green

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
| `05074ba` | **fleet** — per-tenant fleet aggregate + `GET /api/v1/fleet/health` (read-only, worst-state overall) — **first module with unit tests** |
| `6afd3fa`→`7ba635b` | **provisioner** — dry-run → artifact fix → **gated apply path** (approve-gate, verify-before-apply, 60s OTP rollback) |
| `9660801` | **CHR lab target** — containerized-RouterOS throwaway box (sandbox 10.99.0.x) |
| `524f170` | **real Applier + Prober** — SSH/RouterOS transport, prod-guard fail-closed, lab-proven end-to-end |
| `f311cf7` | **wire consumer into server.New** — flag-gated OFF, fail-loud |
| `2089af2` | **RFC 6238 TOTP verifier** — all 6 official vectors pass |
| `b001c65` | **Telegram 2FA approve bridge** — human in the loop, default-deny |
| `ee3eb47` | **docs(progress)** — provisioner arc complete, CCR2004 stood down |
| `28ef5ee` | **voucher** — real redemption: code gen, atomic single-tx claim + Sovereign grant, race-guard |
| `e5604c5` | **test(db)** — voucher money-path DB integration harness (throwaway DB, env-gated) — first real-SQL proof |
| `aa9ee10` | **fix(db)** — `authenticated` role before RLS policies → 10 isolation policies now apply; DB-proven tenant isolation |
| `6927817` | **feat(db)** — `WithTenant` tx-scoped RLS helper + real-DB mechanism proofs (isolates-on / unscoped-off boundary) |
| `5839ba8` | **fix(provisioner)** — enforce RLS on money path: voucher/quota via WithTenant; cross-business read/update BLOCKED, DB-proven + missing quota policy |
| `3db3740` | **feat(database)** — enforce RLS on operational alerting: incident/alert_policy via WithTenant + alert_policies policy (cross-tenant now 404, no leak) |
| `10a9fc7` | **feat(database)** — enforce RLS on telemetry: ingest/list/latest via WithTenant + telemetry policy FOR ALL (was SELECT-only) |
| `c2a9d82` | **feat(database)** — **keystone**: flag-gated restricted pool + tenant connection pinning — RLS applies at the CONNECTION level for the whole request |
| `53161b7` | **feat(database)** — RLS on portal config/session + device request paths (sweep/GetActiveByMAC stay system-level); ~33/67 bare calls cleared |

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

- **Remaining empty module stubs:** `ai`, `billing`, `portal`, `topology`, `vps` (note: `voucher`/`router` *handler+repo* exist; the *module dirs* are empty)
- **Tests now exist** — `probe`, `fleet`, `recovery`, and the provisioner stack (`routeros` transport, `otp`, `telegram` bridge) all carry unit tests; the whole-suite `go test ./...` is green. Growing coverage as each new module lands.
- **Live-write trigger deliberately cold:** the real RouterOS Applier only runs when `provisioner.applier.enabled=true` (default **false**). No prod host/keys/endpoint in the repo. Operator must set `otp_secret` + inject a real Telegram messenger before any live use. Human approve-gate + OTP high-risk gate + 60s auto-rollback are in the path.
- **Recovery apparatus for the CCR2004 is stood down** (2026-09-25): device offline ~52h+; hub-side retries won't change customer-side reality. Not treated as an active alarm; re-check only if the customer re-engages.

---

## 5 · Sizing (honest)

**Overall: ~30–40% of full Mega Magic scope.** The self-healing *brain* is substantially done and proven — but the *arms/legs* (fleet dashboard, provisioner, billing, portal, AI, topology) are unstarted.

Rough module health:
- Self-healing data plane (probe→monitoring→evaluator→sweep→recovery): **~done, verified**
- Tenant/auth/device persistence + HTTP CRUD surface: **~70% scaffolded**
- Feature modules (9 empty dirs): **0%**

---

## 6 · Lead flags (audit, not actions)

1. **Live-write trigger held cold (by design).** The provisioner is lab-proven but the flag defaults OFF. Before ever wiring a real router: inject a real Telegram messenger, set an operator `otp_secret`, and run one deliberate live smoke against an approved target. Capability is complete; usage is a separate, explicit, Lead-gated decision.
2. **Remaining feature surface** is the separate product modules (`ai`, `billing`, `portal`, `topology`, `vps`) — see dependency order in §7.

---

## 7 · Next unit (dependency-based recommendation)

**Voucher redemption is DONE** (`28ef5ee`): real code gen, atomic single-transaction claim + Sovereign tier grant, fail-closed unknown plan, race-guard (409), and a fixed base schema bug. Full gate green (9 modules). **This is the first revenue path in the rewrite** — a paid voucher now actually grants the Sovereign pass.

Remaining product modules by dependency order: **`portal`** (customer self-service redemption — the next integration; depends on the now-live voucher + device repos), then `topology` (network map), then `ai`/`vps`. **Billing/quota enforcement** (sweep checking WebfigPassExpiresAt against actual usage) is queued as follow-up to voucher.

Two queued follow-ups (require an explicit decision):
1. **DB integration harness — DONE (`e5604c5`)** — money-path SQL (claim race → 409, rollback) now DB-proven, then reused to prove RLS isolation (`aa9ee10`).
2. **RLS enforcement — locked down through keystone + portal/device (`aa9ee10`→`53161b7`)** — restricted pool + tenant connection pinning (flag `database.rls_enforce`, default off) applies RLS at the **connection level**, DB-proven cross-business-blocked. **~33 of 67 bare calls converted + real-DB-proven** across money path, alerting, telemetry, portal config/session, device request paths. 3 same-class policy fixes (quota, alert_policies, telemetry). **System-level by design (NOT converted):** sweep worker / `ListAllPeerKeys` (cross-tenant hub loop), `GetActiveByMAC` (anonymous seam), login-time `FindByEmail`/`UpdateLastLogin` (pre-context). **Remaining slices:** user (register-only), heartbeat (worker pattern), webhook (no RLS at all), audit (system-appended), then build the actual portal HTTP surface on the now-isolated repos. Cross-tenant probes return 404 (no existence leak).
3. **Portal redemption — who redeems?** (clawclaw finding #7, decided: **anonymous customer at the captive portal**, MAC-anchored). Determines authz on the redeem endpoint. Ready to scope portal with this baked in.