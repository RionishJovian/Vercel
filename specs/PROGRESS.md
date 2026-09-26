# Mega Magic — Progress Status

**Updated:** 2026-09-26 · **Repo:** `mega-magic-project` (V-language VSL, ~11,600 LOC) · **Tree:** clean, vet+build green

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
| `cf46329` | **feat(portal)** — **portal PRODUCT BUILT**: anonymous device-anchored captive-portal (config/redeem/status) + staff config, on shared RLS-proven redemption core |
| `7238e8a` | **docs(progress)** — portal product built on RLS foundation |
| `d39a553` | **feat(webhook)** — full slice: fix schema drift, enable RLS, tenant-wrap the notifier worker (outbound webhooks survive enforcement) |
| `cf46329`→`49404ca` | **feat(billing)** — device-anchored purchase requests + operator approve-gate (Sovereign grant, single-wind, RLS-scoped) → hardened money path (required+unique bank-ref, plan-driven 30-day duration) |
| `0647664` | **feat(quota)** — enforcement sweep: detect lapsed Sovereign passes, emit quota.pass.expired |
| `ae9893e` | **feat(auth)** — tenant-scope the Me profile read; document user-repo system-level exclusions |
| `e1d61a6` | **test(e2e)** — HTTP boot test (4/4 real-HTTP PASS), fix RLS + billing schema drift via migrations (authenticated grants were missing from persistent DB) |
| `83647b2` | **feat(guest-wifi)** — Option C schema: `business_telegram_bots` + `guest_purchase_requests` (tenant RLS-isolated), portal `guest_plans` + `guest_grace_minutes`, idempotent migration |
| `cf1993f` | **feat(guest-wifi)** — owner bot binding (GET/PUT/test, masked token, tenant-scoped) + per-business bridge supervisor (one getUpdates stream per ENABLED owner, flag-gated off) |
| `b059ae8` | **feat(guest-wifi)** — guest online-banking purchase (grace window + receipt attach, fail-closed) + approve grants wifi extension single-wind / reject grants nothing (handler DB harness, 3 tests, 33→91 total) |
| `98c9589` | **feat(guest-wifi)** — notify owner's own bot on printed-voucher redemption (informational, fail-closed) |

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

- **Remaining empty module stubs:** `ai`, `billing`, `portal`, `topology-module`, `vps-module` (note: `billing`/`topology`/`vps`/`portal` *handlers+repos exist* + are live; the pure-`module` dirs are empty/structural)
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

Remaining product modules: `ai` (most speculative; needs a model contract + token-metering), plus `topology relay/reboot` and `vps apply` (both gated behind the live RouterOS applier). **Billing + quota + portal + topology-map + vps-provisioning + operator Telegram loop + guest-wifi (Option C) are DONE**: the operator loop ships a real Bot-API transport + inbound approve/reply bridge on the SHARED money core (one grant path, HTTP and Telegram identical) — flag-gated OFF by default. **guest-wifi (Option C)** adds a per-owner loop: each business binds its OWN bot token (DB-stored, tenant-scoped, masked on read), guests buy wifi by online banking (free grace window → receipt, fail-closed), the owner approves/rejects on their own bot (approve grants a single-wind wifi extension from the portal plan; reject grants nothing), and printed-voucher redemptions notify the owner's bot — all flag-gated OFF by default (`guest_wifi.enabled`, purchases decided on the web endpoint when off). **vps peer-provisioning** (read-only WireGuard config + deterministic peer block + reachability, Sovereign-gated) is write-free with an explicit ownership guard (404 cross-tenant even with RLS off). **topology relay/reboot** (2026-09-26): explicit owner device action — `POST /api/v1/topology/:deviceId/reboot` (Sovereign-gated, tenant-scoped; queues a `topology.action` intent) driven by a `TopologyActionConsumer` reusing the shared manual approve-gate + OTP (reboot is high-risk) + `ApplyService.ApplyAction` fire-and-validate (deliberately NO config snapshot/rollback — you can't un-reboot). Wired behind the SAME `provisioner.applier.enabled` flag; default **OFF** so no router is ever touched until explicitly enabled.

**Public test URL live (2026-09-26):** `mega.mikromagic.com` serves the Mega Magic API on this VPS — Cloudflare-proxied → nginx vhost (`/etc/nginx/sites-available/mega-magic`) → `127.0.0.1:8080`. Served by a new **systemd unit `mega-magic-api.service`** (enabled, restart-on-failure) running the freshly-built gitignored binary `/home/ubuntu/mega-magic-project/server`. Rebuild+restart: `/usr/bin/go build -o server cmd/server/main.go && sudo systemctl restart mega-magic-api`. Verified live: `/ready` → 200 all-ok over http+https, login → 200 real token. MikroMagic still runs on :3000/:8000 (unlinked from the apex, see flip note below).

**APEX FLIPPED to Mega Magic (2026-09-26):** `mikromagic.com` AND `www.mikromagic.com` now serve the Mega Magic Go API (`/ready` 200, login 200, through Cloudflare). New nginx vhost `mega-magic-apex` (port 80 → 301 https; 443 → :8080, reused LE cert). The MikroMagic apex vhost (`mikromagic-app`) is UNLINKED from sites-enabled, **not deleted** — MikroMagic still runs (:3000 frontend 200, :8000 backend 401-auth, systemd active). Revert: `sudo rm /etc/nginx/sites-enabled/mega-magic-apex.conf && sudo ln -sf /etc/nginx/sites-available/mikromagic-app /etc/nginx/sites-enabled/mikromagic-app && sudo nginx -t && sudo systemctl reload nginx`.

**MIKROMAGIC SHUT DOWN (2026-09-26):** no active users on the old project → `mikromagic-app.service` disabled+inactive; all MikroMagic timers/services (health-monitor, i18n-audit, telegram-webhook-keep, mm-gate, mm-model-watchdog) disabled+inactive; Supabase stack containers stopped. Ports 3000/8000 free. MicroTik Supabase data preserved (not deleted). `mega-magic-api.service` + `chr-lab` + all `mega_magic_*` containers untouched and running. Domain `mikromagic.com` (apex/www/mega) serves only Mega Magic. Reversible: `sudo systemctl enable --now mikromagic-app` + restart Supabase containers + `sudo ln -sf /etc/nginx/sites-available/mikromagic-app /etc/nginx/sites-enabled/ && sudo systemctl reload nginx`.


**AI OPERATOR TELEGRAM ALERT (2026-09-26, f881891):** AI-incident open now DMs the operator. TelegramNotifier (Implements IncidentNotifier): best-effort single-line `AI alert [sev] subject kind: detail (incident <id>) at <UTC>` to the operator chat. Note-1 COMPOSE via Fanout — webhooks + Telegram both fire, nothing replaced. Config ai.notify_telegram_{enabled,bot_token,chat_id}, default OFF; misconfig degrades to webhooks-only w/ warning; bot token never logged (fake Messenger in tests). Peer produced code but timed out on its report step (1800s); Lead verified independently (vet/build 0, 18 pkgs, notify tests green, token-leak scan clean). CLI NOTE: openclaw agent builder runs can exceed default agent timeout; budget briefs so the agent completes its self-check. THE AI CHAIN IS NOW COMPLETE + gated off: analyze→meter→finding→incident→notify (webhook+DM).
**AI OPENROUTER CLASSIFIER (2026-09-26, b647a6a):** `OpenRouterClient` behind the existing `ModelClient` seam — free OpenRouter completion model for telemetry classification. The model was chosen by a LIV●E catalog probe (the originally-requested `nvidia/llama-nemotron-embed-vl-1b-v2:free` does NOT exist on OpenRouter and is an embed/vector model, wrong modality): `nvidia/nemotron-3-super-120b-a12b:free` was the ONLY candidate out of gemma-4 (guardrail-blocked), qwen3.8 (provider error), nemotron-ultra (prose), liquid (parse fail), cohere/north-mini (code-flavored) to return parseable finding JSON with `response_format:json_object`. Gated by `ai.mode: openrouter` (default `auto` stays Ollama+rule-fallback, never calls OpenRouter). API key injected via config/env, NEVER logged/echoed, tests use httptest fakes only. LEAD FIX: server.go wiring originally omitted the 3 OpenRouter fields (would have been empty key → auth fail); completed the pass-through. Verified by Lead: vet/build RC=0, 18 pkgs, key-in-logs scan 0 hits, and a LIVE probe through the compiled client hit the real free model → parseable critical Finding (rule/sev=critical/conf=0.96).
**AI MONEY-PATH DB/RLS PROOF (2026-09-26, `a98afb6`):** `internal/ai/integration_test.go` (peer) — package-internal harness (env-gated MM_TEST_DB_URL, fresh throwaway DB, mirrors internal/database) proving 3 real-DB/RLS cases: quotaMeter.Spend tenant-scoped + dedupe, never-below-zero, and databaseIncidentStore dedupe-then-create + RLS cross-tenant isolation. CAUGHT+FIXED a latent bug: incidents.severity CHECK is ('info','warning','critical') but the AI finding vocabulary says 'warn' -> would violate CreateTXN and silently fail-closed; normalizeSeverity now maps warn/warning->'warning'. Lead verified by RUNNING the proof himself vs live dev DB (localhost:5433), 3/3 PASS with RLS enforced (not a peer green-check).

**AI FINDING→INCIDENT CONSUMER (2026-09-26, `92819c4`):** `internal/ai` second slice (OpenClaw peer). `ai.finding` now publishes a self-describing envelope finding+business/device/subject; the Service opens a real `Incident` per finding mirroring `evaluator.raiseIncident` — DB-only dedupe `HasOpenIncidentBySubjectTXN`, tenant-scoped one-tx create-if-not-open, reuses the evaluator's notifier instance, `Kind=ai_<sanitized>`, nil PolicyID, Detail=Summary, UTC. Open-only/fail-closed/gated OFF. Verified: vet RC=0, build ok, suite 18 pkgs RC=0, 21 ai tests pass, secret-clean, committed. Docs: peer loop (dispatch→audit→commit→ack) via `openclaw agent --deliver` to the session-key. Clawclaw sandbox: NATS joined `mega_magic_dev_network` (was bridge) so the peer reaches the bus; `:8080` stays firewalled (no ufw hole).

**AI SERVICE FIRST SLICE (2026-09-26, `0e0e0aa`):** `internal/ai` built by the OpenClaw peer to a model-contract spec (I scoped defects, peer drafted, I audited). Consumes `telemetry.ingest`, classifies each sample via pluggable `ModelClient` (Ollama local primary, deterministic rule-fallback, mode `auto`), meters `ai_token_balance` spend BEFORE acting (tenant-scoped one-tx read+write via `WithTenant`, never below zero, exhausted=skip), publishes `ai.finding` after a successful spend. Fail-closed: analysis errors + exhausted quota spend/publish nothing. In-memory dedupe `business|device|rule`, cleared by healthy sample. Worker gated OFF (`ai.enabled=false`); `Start()` no-op when disabled. Verified by me on host: vet RC=0, build ok, 18 pkgs pass, 14 new ai unit tests. Follow-ups parked: (1) money-path spend needs real-DB/RLS integration test, (2) `ai.finding` has no consumer yet, (3) `ai.finding` per-authority — spend+publish not transactional together. Peer reachability unblocked 2026-09-26 (NATS joined dev network).

**WEB INCIDENTS VIEW (2026-09-26, fd890e4):** dashboard now has an Incidents view (Operations nav, after Topology) that lists open incidents via the existing `GET /api/v1/incidents` — severity badge (critical/bad, warning/warn, other/unknown), kind, subject label, truncated detail, opened time — with Acknowledge/Resolve buttons hitting `POST /incidents/:id/{acknowledge,resolve}` and re-rendering. This surfaces the AI finding->incident chain output so an operator can see and action detected problems. Design system: added `.btn-sm` + unknown-severity fallback. Inline JS node--check'd; live-verified over the public domain via a real create->list->ack->resolve->gone cycle (200 each, count 1->0). Deployed to /opt/mega-magic-web.
**WEB UI LIVE (2026-09-26):** `mikromagic.com` (apex/www) now serves a real frontend — a framework-less React-free app under `web/` (landing + sign-in + dashboard with Overview/Fleet/Sites/Topology/Vouchers/Portal views), designed to replace the old AI-style TanStack/Supabase `ui-reference/`. Professional design system: warm paper bg, ink text, single deep-teal accent, hairline borders, tabular numerics, thin left rail (Monitor surface); landing is a restrained Decide surface. nginx serves the build from `/opt/mega-magic-web` and proxies `/api/v1` (plus /health+/ready) to the Go API on :8080 (systemd `mega-magic-api`). Token auth: login stores JWT in localStorage (`mm_token`); api.js client hits `/api/v1`. Verified live: landing/css/js 200, login token ok, and every dashboard endpoint (`/me /fleet/health /topology/map /routers /vouchers /billing/requests /portal/config`) returns 200 through the public domain. Rebuild/redep: edit `web/`, `sudo cp -r web/. /opt/mega-magic-web/` then reload nginx. Browser pixel-render not verified (backed browser config locked to real-profile); behavior verified via live HTTP + node syntax-check. Commit `3b99bef`.

### §7.1 · guest-wifi (Option C) — activation checklist

Built but **OFF by default**. Everything below is a deliberate, Lead-gated action to go live; nothing is required until you decide to turn it on.

1. **Feature flag** — set `config.yaml` `guest_wifi.enabled: true` (and tune `bridge_scan_interval`; `receipt_dir` stores guest payment receipts — keep it non-world-readable). Restart the server.
2. **Per-business guest plans (owner + tooling)** — for each business, populate `portal_configs.guest_plans` (JSON `[{key,label,minutes,amount}]`) and set `guest_grace_minutes` (the free payment window, default 10). Guardrails already enforced at runtime: a guest purchase against a business with no plans → 400 `plan not available`; a plan with no/minutes ≤ 0 → approve fails closed ("no duration"); grace ≤ 0 → online purchase refused for that business. No DB writes here until Phase 3 below if you want to dogfood first.
3. **Owner bot binding (self-serve)** — each hotspot owner binds their **own** Telegram bot token under their platform account (`PUT /api/v1/guest-wifi/telegram-bot`, test with `POST .../telegram-bot/test`). Their token is DB-stored, tenant-scoped, masked on read, and used ONLY for their own guest loop — never for your SaaS token, never cross-tenant.
4. **The web path is already live regardless of the flag** — a guest can buy wifi on the portal and the owner can approve/reject on the protected web endpoint (`POST /api/v1/guest-wifi/approve/:id`, `reject/:id`) even with `guest_wifi.enabled: false`. The flag ONLY turns on the extra "second apple":
   - `guest_wifi.enabled: true` → the supervisor starts one bridge per **enabled** owner bot (notify + `approve <id>` / `reject <id>` inbound replies). Web and bot are independent; owners can use either or both — never forced to one.
   - Fail-closed by construction: no binding / disabled / no token / feature off / send error ⇒ no notification and no decision; malformed or unknown bot replies are ignored (never acted on).
5. **Parallel flows confirmed in the build**:
   - Guest **online-banking** purchase: pick plan → free grace window → attach receipt only within the window → owner approve grants the wifi extension single-wind / reject grants nothing.
   - **Printed-voucher** redemption: stays instant; the owner's bot is notified (informational, non-blocking).
6. **Safe-rollback** — flip `guest_wifi.enabled` back to `false`; purchases are then decided on the web endpoint alone. Binding rows remain but are inert.

The **RLS tail resolved via real audit**: `user` (Me read scoped), `heartbeat` (already param-scoped, no live hole), `audit` (unwired — build when needed). Enforcement story complete. **HTTP boot test (`e1d61a6`)** proved the real server serves all four live-HTTP checks and caught RLS/billing schema drift (authenticated grants missing from persistent DB — fixed via idempotent migrations).

Two queued follow-ups (require an explicit decision):
1. **DB integration harness — DONE (`e5604c5`)** — money-path SQL (claim race → 409, rollback) now DB-proven, then reused to prove RLS isolation (`aa9ee10`).
2. **RLS enforcement — locked down through keystone + portal/device + webhook (`aa9ee10`→`d39a553`)** — restricted pool + tenant connection pinning (flag `database.rls_enforce`, default off) applies RLS at the **connection level**, DB-proven cross-business-blocked. **~43 of 67 bare calls converted + real-DB-proven** across money path, alerting, telemetry, portal config/session, device request paths, and webhook (schema-drift fixed + notifier worker tenant-wrapped — outbound webhooks survive enforcement). 4 same-class policy fixes (quota, alert_policies, telemetry, webhook). **System-level by design (NOT converted):** sweep worker / `ListAllPeerKeys` (cross-tenant hub loop), `GetActiveByMAC` (anonymous seam), login-time `FindByEmail`/`UpdateLastLogin` (pre-context). **Remaining slices:** user (register-only), heartbeat (worker pattern), audit (system-appended). Cross-tenant probes return 404 (no existence leak).
3. **Portal — PRODUCT BUILT (`cf46329`)** — the captive-portal surface now exists, built on the RLS-secure foundation. Decision (yours, earlier): **anonymous customer at the portal, MAC-anchored**. Resolution is **device-anchored**: a client's tenant = the business of the AP/router (`device_id`) they're attached to, not the code (codes are only business-unique) and not a session. Public `/api/v1/portal/:deviceId/{config,redeem}` + `/status` (anonymous, no auth) re-use the ONE shared redemption core (owner endpoint + portal both call it) — cross-business redemption impossible by construction, real-DB-proven. Staff `protected./portal/config` GET/PUT. Follow-ups: HTTP-boot integration test (harness proves mechanism, not a booted server); the ~34/67 remaining bare sites if full RLS coverage is wanted.