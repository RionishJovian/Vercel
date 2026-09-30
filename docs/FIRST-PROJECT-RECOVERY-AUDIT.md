# MikroTik Magic first-project recovery audit

**Reference source:** `/Users/aces/Nish/mikromagic/app`  
**Current source:** `/Users/aces/Magic Development/mega-magic-project`  
**Audit date:** 2026-09-30  
**Safety rule:** the first project is read-only reference material. Do not replace the
current database, authentication, tenant boundaries, or production deployment with a
bulk copy.

## What the evidence says

The first project is not merely a UI prototype. Its stable-baseline document identifies
a production release from 2026-09-11, its compiled `.output/` is present, and its source
contains focused server implementations and tests for RouterOS, captive portal,
connectors, vouchers, topology, security, finance, and tenant isolation.

The current project is also not disposable. It has a smaller, security-oriented Go
control plane with JWT authentication, tenant-scoped database access, hardware-pinned
SSH credentials, RouterOS snapshot/apply/verify/rollback primitives, database migration
tools, a public portal API, and a deployed frontend. Recovery should combine these
strengths rather than restart either application.

No feature below is declared production-working solely because a file or historical test
exists. Each recovered slice must be validated against the current database and a
disposable RouterOS target before production enablement.

## Capability comparison

| Capability | First project evidence | Current state | Recovery decision |
| --- | --- | --- | --- |
| Authentication and tenant roles | Supabase auth, RLS, role and tenant tests | JWT auth, tenant transactions, role middleware | Keep current security core; port missing UX only |
| Connector / Cloud Remote | Pair, heartbeat, jobs, results, version/download endpoints; Windows/macOS/Linux installers; MNDP discovery and RouterOS transport | Router connection records and cloud status, but no equivalent complete agent lifecycle in the current UI/API | Restore first, behind current tenant/auth model |
| Captive portal publishing | Versioned bundle, fingerprint, probe, staged deploy, rollback, directory pruning, RouterOS trial setup | Cloud config, preview, guest purchase and voucher APIs; no router file publish | Port engine concepts into Go and certify on CHR |
| Voucher plans and enforcement | Plan/profile sync, RouterOS users, generation, print layouts, expiry and quota tests | Voucher CRUD, portal plans, print layout, quota primitives; some hardware enforcement exists but workflow is incomplete | Reconcile one end-to-end voucher contract |
| Live users and controls | List active users, kick user, ban MAC, MCP tools | No equivalent operator workflow exposed | Restore after connector transport |
| Quick setup / provisioning | Gateway bootstrap, quick config audit, Multi-WAN, hotspot SSID, apply traces | Device registration, WireGuard provisioning pieces, limited operational UI | Port validated planners, not legacy secrets |
| Terminal and scripts | Executable router operations, templates, support grants, audited history | Template CRUD and read-only terminal history | Restore guarded execution using current credential resolver |
| Topology and sites | Site workspace, map, port-level diagram, topology pools and probes | Sites CRUD and current topology map | Keep current API; recover richer workspace incrementally |
| Fleet telemetry and syslog | Live telemetry, health hooks, paired syslog ingest, alerts | Telemetry, fleet health, incidents and syslog read endpoint | Wire ingestion and operational views end-to-end |
| Backups and restore | Backup workflows and DB backup hook | Device backup/change timeline and guarded restore | Keep current implementation; compare UX and coverage |
| Security | Captive-portal security, target guards, production security tests | RouterOS posture, pinned host keys, target CIDR guard | Preserve current guards; port missing audits |
| Business operations | Orders, receipts, revenue, services, reseller operations, daily close | Billing requests, reseller repository, coins and platform administration; incomplete product surfaces | Restore only after network money path works |
| Magic Dude | Grounded retrieval, tenant context, Gemini plus fallback | OpenRouter diagnostic chat and usage events | Reconcile safely; never expose credentials/configs |
| Localization and accessibility | Burmese/Chinese dictionaries, overrides and i18n audit | Overrides and partial portal Burmese text | Port dictionaries and audit tooling |
| Easy/mobile operation | Easy mode, bottom navigation, mobile tests | Responsive static dashboard, no comparable guided mode | Recover after core workflows |

## Recovery order

### Phase 0 — preserve and define contracts

1. Keep both source trees unchanged and backed up.
2. Inventory first-project database tables against the completed Go migration map.
3. Extract behavioral contracts from the old tests; do not copy implementations blindly.
4. Establish one disposable CHR certification environment and one authenticated staging
   tenant.
5. Add a recovery checklist to every slice: tenant isolation, permission gate, audit
   event, rollback, responsive UI, and explicit failure state.

### Phase 1 — restore the real router path

1. Connector pairing and heartbeat.
2. Per-device job dispatch/result collection.
3. Router reachability and identity verification.
4. Live-user list, kick, and MAC ban.
5. Guarded terminal/script execution with redacted audit history.

Completion criterion: an operator action reaches one explicitly selected CHR, is read
back, is tenant-scoped, and has an auditable failure path.

### Phase 2 — restore the captive-portal money path

1. Save portal configuration reliably.
2. Select a RouterOS target and HotSpot profile explicitly.
3. Build a versioned portal bundle.
4. Snapshot, publish, switch, probe, and roll back on failure.
5. Synchronize voucher profiles/users and HotSpot trial access.
6. Prove voucher redemption creates actual RouterOS access—not only a database session.
7. Verify expiry, data caps, and disconnect behavior on CHR.

Completion criterion: a test phone joins the lab HotSpot, sees the saved branding, logs in
with a voucher, receives the correct time/data allowance, and loses access on expiry.

### Phase 3 — operational visibility

Restore live telemetry, syslog ingestion, alerting, richer topology, configuration
backup/restore, and device support tooling. Reuse current Go repositories and security
boundaries where they are stronger.

### Phase 4 — business operations and product polish

Restore receipt review, revenue, orders, services, reseller/daily-close workflows,
notifications, Easy mode, mobile navigation, and full localization after the router and
portal paths are verified.

## Immediate rules

- Do not delete or overwrite `/Users/aces/Nish/mikromagic`.
- Do not deploy the old Supabase application over the current production site.
- Do not import legacy secrets or ciphertext into the current credential store.
- Do not label a screen "working" unless its write reaches the intended backend and its
  resulting state is read back.
- Do not enable RouterOS production writes until the CHR certification passes for that
  specific workflow.
- Do not target the RB5009 until Netinstall is complete, the device is registered, and
  its SSH host-key fingerprint is verified out of band.

## First implementation target

Recover the first project's captive-portal deploy/probe/rollback behavior into the
current Go control plane. It directly addresses the present branding issue and provides
the transport foundation needed by voucher login. The existing first-project files under
`src/lib/portal/` and its portal safety/probe/trial tests are the behavioral reference;
the current encrypted device target resolver and CIDR guard remain authoritative.

## Product-experience restoration baseline

The first project's visual identity and workflows are now the reference for these
current-product areas:

1. Site topology workspace, diagram, inventory and properties panels.
2. Magic Dude chat placement, grounded product knowledge and conversational UX.
3. Tenant, developer and operational admin panels.
4. Dense fleet monitoring, health, probe and alert views.
5. Revenue, orders, receipt review and daily-close dashboards.
6. Myanmar/Chinese language settings, dictionaries and translation auditing.
7. RouterBOARD settings, Quick Config, Multi-WAN, SSID, access-point and script panels.
8. WebFig access and status experience.

This is a frontend/workflow restoration, not a backend rollback. Recovered components
must consume explicit Go API contracts. They must not carry over direct Supabase feature
writes, duplicate RouterOS transports, browser-held router credentials, legacy encrypted
secrets, or optimistic success messages without device read-back.

## Visible recovery ledger

This ledger is the product-development checklist. A feature moves to **recovered** only
after its current Go API path and browser behavior are both tested.

| Product area requested by the owner | Current visible state | Next recovery contract |
| --- | --- | --- |
| Original Day/Night identity | Recovered foundation: one semantic token system, persistent theme, old-product density and glass accents | Continue selector-by-selector page polish without splitting the themes |
| Captive portal | Recovered visual design and live preview; working plan/branding persistence and voucher entry | RouterOS bundle publish, probe and rollback still require CHR certification |
| Magic Dude | Recovered as a floating, non-navigating chat window; free OpenRouter route and read-only fleet grounding | Expand grounded product help after the operational APIs mature |
| Revenue dashboard | Recovered on 2026-09-30 from current tenant billing records: totals, trend, funnel, plan performance, ledger, CSV and approve/reject readback | Add site attribution only after purchase records carry a stable site key |
| Fleet | Real current health endpoint is visible, but presentation is below the first project's operational density | Restore router cards, telemetry and alert drill-down against current endpoints |
| Topology | Real current map, relationship integrity and guarded reboot are visible | Restore the old workspace/canvas, inventory and properties interaction |
| Administrator panels | Team, capacity and DEVELOPER platform panels are real but fragmented | Consolidate tenant administration while preserving exact role gates |
| i18n language settings | Real override CRUD exists; Myanmar/Chinese dictionaries are incomplete | Port dictionaries, user locale switching and translation audit |
| RouterBOARD settings/config | Router records and encrypted connection management exist; Quick Config is missing | Add read-only snapshot first, then planned/apply/verify/rollback on CHR |
| WebFig | Not recovered; the feature flag and legacy launcher exist but no current secure launch contract does | Build a short-lived, tenant-scoped proxy/session; never expose stored router credentials |
| Store / Services | Old catalogue and data were audited; pricing, purchases, entitlements, bank accounts, coins and keys survived migration, but the current checkout/activation engine is incomplete | Build a dedicated transactional Store API before exposing Buy controls |

The sidebar must never advertise a placeholder as a finished feature. Missing areas are
added only when their read path is real; mutation controls require authoritative readback.
