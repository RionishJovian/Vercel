# MikroMagic delivery status — development build 20 (updated)

Latest additions: manual SME-to-platform subscription invoices/payment review and optional authenticator MFA. Earlier increment sections describe historical checkpoints; the build-20 section below supersedes their billing/MFA pending notes. Production integrations remain outstanding.

Code and verification are separate from production readiness. External integrations are not connected. Nothing here proves real router enforcement or paid streaming. Live router configuration and verification are deferred at the user's request; development continues on web-app workflows.

| Requirement | Current implementation | Verification / remaining work |
|---|---|---|
| Three roles | Platform, SME and guest workspaces; scoped API demo sessions | Browser role switching tested; API blocks foreign-tenant payment decisions. Account sign-in and tenant/role authorization implemented for core flows; scoped support delegation and deployment hardening pending. |
| SME revenue | Account-backed cash event ledger; site/date/timezone filters; gross/refunds/net, receipt count/average, pending workload, service/site/method/day totals and CSV export | Isolated tests cover period refunds, midnight boundaries, historical backfill, tenant/site isolation and source snapshots. Costs/profit remain demo-only; reconciliation and full KPI suite remain pending. Authenticated browser QA pending. |
| Manual payment review | Persistent orders and price snapshots; reference/evidence, duplicate warnings; approve/reject/request evidence; refund record and revocation | API tests passed for permission, duplicate decision, reasons, resubmission and revoke. Guest-order JPEG/PNG receipts now upload privately, with authenticated viewing and stale-review checks. Counter-sale image attachments and independent money verification remain pending. |
| External review API | Account-backed owner-delegated service credentials, complete review context/private receipts, context/version checks and idempotent audited decisions; separate demo HMAC implementation | Read-only, tenant/role isolation, token expiry/revoke/owner disable, stale context and approval/request-evidence/reject workflows tested. Durable per-service pull delivery with leases/acknowledgement/retries is now implemented. Real Telegram/Discord identity adapters, signature verification, provider notifications and reconciliation remain pending. |
| Voucher lifecycle | Account-backed issuance, hashed codes, masked inventory, single redemption, configurable 1–365 day redeem deadline, revocation, print UI and redemption metrics | Isolated tests verify retries, concurrent redemption, tenant isolation, expiry and revoke. Counter-sale receipt/refund records are connected in increment 6; browser print QA remains. |
| Internet entitlement | Data/time/both records, elapsed or connected counters, activation, usage simulation | API tests deny exhausted data and preserve counter on reactivation. Actual RouterOS/AAA accounting, sessions and device enforcement unbuilt. |
| Media entitlements | Account-backed timed-library, one-title and hybrid package policies; separate hybrid library/title windows and combined internet/media windows | Policy snapshots propagate through orders/vouchers/passes. Demo tests check expiry/title restrictions; live activation, playback, segment checks and replay-count policy remain pending. |
| Portal configuration | Saved SME drafts for branding, hostname/interface/subnet, payment instructions, allowlist and isolation; preview | Browser preview tested. Router selection, TLS, address-pool validation, diff/apply/backup/rollback unbuilt. |
| Magic Hub onboarding | Platform hub registry and business assignments; SME router plan allocation, immutable snapshots, JSON export and review | Atomic allocation/retry/isolation/overlap/exhaustion tests pass. Hub provisioning, router diff/apply/backup/rollback, capability discovery and trusted handshake/reachability evidence still need adapters and actual VPS/router. |
| WAN controls | Budget/reset/threshold/action/failover/load balancing drafts | Save API exists. Real Starlink estimate collection, cycle rollover, alerts and local policy apply/fault tests unbuilt. |
| Topology | Explicit manual architecture preview; separates management from local traffic | Real discovered graph/link evidence and editing unbuilt. |
| Appearance | Account-backed platform light/dark palette drafts, publication, role scope enforcement and previous-publication restore | Platform-only writes, revision conflicts, publication/restore and 20 contrast pairs tested. Authenticated browser and full accessibility QA remain pending; per-tenant overrides are not implemented. |
| SME guidance | Account-backed Did you know? sidebar page; search/topic filters, source/date, related settings and saved explanations in Magic Dude; platform drafts/edit/publish/withdraw/retire/restore | Role and revision checks, source/date validation and migration preservation tests pass. Three initial documentation cards. Compatibility stays unchecked; no AI-generated explanation or router discovery. |
| Floating Magic Dude | Contextual overlay, input and guided demo answers | Browser open/send/response checked. No model, diagnostic tools or config execution integration. |
| Platform operations | Business/first-SME-owner creation, 24-hour temporary-password handoff and mandatory change; owner reset/session/service-key revocation and reset history; account/fleet/hub readiness and audit | Permission, replay/duplicate, gate, expiry, password/session and reset isolation checks pass. SaaS billing, complete owner/staff lifecycle, recovery/MFA, fleet monitoring and live operational actions remain pending. |
| Media management | SME catalogue drafts/edit/publication/archive/restore; package pricing through Packages; guest browsing and stored media passes | Tenant/role/revision, publication and history tests pass. File upload, artwork, transcoding, storage, per-site source mapping, Jellyfin sync and playback remain unbuilt. |
| UI | User-reference forest/sage and ivory/evergreen; navigation and forms | Desktop browser checked at 1363x936. Responsive CSS exists; mobile browser/device testing remains. |

## Validation performed
- Seven API workflow tests passed, isolated temporary database.
- Frontend production build configured (dist/client/ assets generated; .openai/hosting.json is a production deployment config, deferred per task directives).
- Browser test: guest evidence → pending order → SME review/approval → guest activation/check → revenue update (manual verification).
- Portal preview, palette publish/restore, dark mode and assistant interactions tested.
- No application-origin console errors in checked browser logs. Browser extension metadata errors excluded.
- Python backend: argon2 password hashing, SQLAlchemy model compilation, SQLite migration upgrade/downgrade/upgrade verified on isolated database.
- Type system: Python 3.9 compatibility fixes applied to models.py, schemas.py, media.py (str|None → Optional[str], int|None → Optional[int]).

## Production dependencies
Webapp VPS and hub VPS connection details; test MikroTik and site LAN; local media hardware/Jellyfin; operational domains/TLS; tenant/operator identities; adapter secrets and reviewer accounts; model integration. Never expose the demo role-login API publicly.

## Next implementation sequence
1. Production authentication/tenant RBAC and typed policy validation; authoritative order ledger and evidence storage. *(deferred — live router integration pending)*
2. Magic Hub peer onboarding and router capability discovery, backup/diff/apply/verification. *(deferred — VPS/router adapters pending)*
3. Router-local hotspot/AAA accounting and quota/device enforcement; bypass/IPv6 tests. *(deferred — live router required)*
4. Telegram/Discord adapters and durable event delivery, audited reviewer identity. *(deferred — external service integration)*
5. Local media gateway/Jellyfin with per-segment entitlement checks and offline policy. *(deferred — media gateway pending)*
6. Operational telemetry, WAN fault tests, complete KPIs and deployment acceptance. *(deferred — production infrastructure)*

No requirement has been removed; unfinished items stay explicit here and in BLUEPRINT.md.

## Increment 2 — account/payment backend (6 October 2026)
Added `backend/` FastAPI service with normalized SQLAlchemy records, PostgreSQL driver, frozen Alembic migration, private provisioning CLI and strict request schemas. Passwords use Argon2; opaque sessions are stored as digests, expire after one hour and support logout/account disable. Login failure buckets are persisted. Account role/tenant come from the database. No demo role creation endpoint exists in this service.

Packages/orders/manual decisions/refund records/entitlements/audit/outbox use relational transactions. Order request keys prevent retry duplication; approval uses a conditional pending-state update and unique entitlement constraint. Approved fulfillment stays pending_adapter, never falsely active. Platform accounts cannot automatically review SME guest payments.

Verified: eight new backend tests plus seven prior demo tests; schema compilation/offline migration SQL for PostgreSQL; migration upgrade → downgrade → upgrade on isolated SQLite. PostgreSQL runtime, concurrency under PostgreSQL and deployment still unverified.

At increment 2, the frontend remained on the development API. Increment 3 below connects the core account workflows. Production authentication is partially implemented, not a completed deployment. Frontend TypeScript conversion, guest onboarding, scoped support, evidence image storage, full API parity, accounting enforcement and adapters remain. Existing preview data is preserved; no implicit migration into the new database occurs.

## Increment 3 — authenticated browser workflows
The default entry is account sign-in, with a separate demo exploration button. Account role and tenant are database-derived; no account role selector is present. Session tokens remain in memory; sign-out revokes them and expiry returns to sign-in. Reload requires another sign-in. There are no default accounts.

Connected to `/v1`: SME packages, payment queue/full review, approval/rejection/evidence request/refund recording, revenue summaries, guest purchases with stable retry keys, evidence resubmission, stored passes, platform tenant registry and scoped audit. Fulfillment remains pending_adapter.

At increment 3, account portal settings, vouchers, WAN, routers, topology, media, integrations and palette publication showed pending work. Portal settings are connected in increment 4 below. Their demonstration forms remain separate; demo records are never used as authenticated business data. Core revenue has collections, refunds, order counts and service breakdown; cost tracking and date filters remain demo-only.

Verified: 16 isolated Python workflow tests, two client session/error tests, four frontend worker checks and production build. Browser checked sign-in layout and demo entry/return. Sign-in service proxy health was not independently confirmed in the browser. Authenticated browser journeys, mobile tests and PostgreSQL runtime validation remain pending.

## Increment 4 — SME portal drafts and publication
SME owners can save account-backed portal branding, welcome text, language preference, terms/contact, direct purchase and desired voucher options, payment destination/instructions/methods, hostname, guest interface/subnet, TLS/isolation preferences and explicit pre-login allowed hosts. Draft preview and publication are separate. Revision checks reject stale updates/publications. Changes are audited and isolated by account tenant.

Guests see only published guest-facing fields. Network configuration is excluded from that API. New purchases require a published portal, enabled direct purchases, an offered method and the published revision seen by the guest. Each order snapshots the payment destination/instructions; later portal edits never rewrite that history. Older orders retain null payment context rather than invented historical details. Existing request-key retries still return their original order after purchases close.

Publishing updates web content and purchase availability only. Router selection, DHCP/address pools, DNS/TLS provisioning, apply/diff/backup/verification/rollback, IPv6 behavior and media gateway enforcement remain pending. This initial portal is per business; per-site configuration awaits the site model. Language preference is stored; translated interface strings and recorded terms acceptance are pending. Desired voucher login does not activate account-backed redemption. Guest onboarding/pre-login account resolution remains pending.

Verified: 22 Python tests (including five portal workflow checks and migration preservation/round-trip), six existing JavaScript client/worker checks, component rendering checks for visible editor fields, closed purchasing, escaped untrusted text and no guest network fields, plus production build. PostgreSQL migration SQL compiles, but PostgreSQL runtime remains unverified. The preview service starts; browser inspection of the new editor was blocked by the current browser's unsupported error-page state. A static preview rendered from the actual React component is provided with sample data and disabled controls; it is not an authenticated browser journey.

## Increment 5 — account-backed vouchers
SME owners issue a single-use code against an immutable package snapshot, set a 1–365 day redemption deadline, view masked inventory and issued/redeemed/rate metrics, and revoke unused codes or redeemed passes with an audit reason. The database stores only a SHA-256 digest of a random 128-bit code. Full codes are returned once for copying/printing. Issuance request keys prevent duplicate codes on retry; a lost issuance response requires revoking the masked inventory entry and reissuing.

Guests redeem through the published portal's voucher option. Disabled/unpublished portals reject new redemptions. An atomic voucher claim plus unique entitlement origin grants one stored pass. Same-guest retries return that pass, including its revoked state; another guest cannot claim it. Expired/revoked/foreign-tenant codes are denied. Invalid well-formed codes have a persisted attempt limit per guest account. Stored passes identify their source as order or voucher, with an exactly-one-source database constraint. Redemption does not activate router or media access.

Issuance/redemption do not create payment orders or collections. Voucher counter-sale recording, refunds and ledger/revenue linkage remain pending; redemption metrics are operational counts, not sales. Entitlement activation, usage accounting, live revocation enforcement and voucher notifications still need the adapters. No requirement for external review, WAN, media, hubs, topology, themes or AI was removed.

Migration 003 preserves prior order entitlements and supports empty-voucher rollback; it refuses rollback when voucher records exist to prevent losing them. Verified: 29 isolated Python tests, six JavaScript client/worker tests, portal and voucher component rendering checks and production build. PostgreSQL migration SQL compiles; PostgreSQL runtime and authenticated browser journeys remain pending. Browser QA retains the previous blocked-browser limitation.

## Increment 6 — confirmed voucher counter sales and refunds
SME owners record a confirmed full-price counter payment against a voucher, with received amount, method, receipt/reference, evidence, staff identity and time. Issuance/redemption remain nonfinancial. A unique sale per voucher prevents duplicate collections; identical retries return the original record and different payment details conflict. Expired unused or revoked vouchers cannot receive a new sale record. Discounts/partial payments remain pending.

Full refund recording requires confirmation of funds returned outside the app, plus refund reference and reason. The recorded sale remains in history; voucher and stored pass are revoked in the same transaction. Repeated identical refunds do not subtract twice. Revoking a code alone never implies returning money. No transfer, independent receipt verification or live adapter action is claimed.

Revenue now combines approved guest orders and confirmed voucher sales, with gross, refunds, net, confirmed payment count, service breakdown and separate guest-order/voucher-sale totals. Platform subscription revenue remains separate. Date/site filters, historical period ledger, cash reconciliation/export, payment notifications, taxes and discounts remain pending.

Migration 004 preserves earlier records and refuses rollback when sale records exist. Verified: 35 Python tests, six JavaScript client/worker tests, component checks and production build, including concurrent sale submissions, retry/refund behavior, amount/confirmation validation, isolation and migration accounting preservation. PostgreSQL SQL generation passes; live PostgreSQL and authenticated browser QA remain pending.

## Increment 7 — sites, MikroTik registry and branch portals
SME owners create named sites with validated reporting timezones, register MikroTik model/name/reported RouterOS version at a site, and see their own registry. Platform accounts read the permitted site/router fleet; guests cannot read router metadata. Reported models/versions are explicitly unverified, with no assigned hub or handshake. No credentials, network connection or generated router script is claimed. Hub onboarding/capability discovery are still pending infrastructure work.

Migration 006 adds magic_hubs, hub_assignments and router_plans. Dedicated aligned private /24 pools are unique. Exact router-plan retries reuse allocation; keys and per-router plans are unique. API review records SME acknowledgement without networking side effects.

## Increment 8: Magic Hub onboarding
Platform `Magic Hubs` registers immutable hubs and assigns them to businesses. SME `Routers` creates a management-only structured connection plan with atomic /32 allocation, public keys, endpoint, routes and required application checks. Reviewed plans await the adapter; connectivity stays unverified. Exported JSON is not an executable configuration. Private keys stay on devices; full live addressing/firewall/capability validation, hub provisioning, command application and trusted evidence are pending.

Migration 006 adds magic_hubs, hub_assignments and router_plans. Dedicated aligned private /24 pools are unique. Exact router-plan retries reuse allocation; keys and per-router plans are unique. API review records SME acknowledgement without networking side effects.

The generated Magic-Hub-preview.html uses sample records and disabled controls, rendered from the production components. WireGuard fields follow the [official MikroTik manual](https://manual.mikrotik.com/docs/virtual-private-networks/wireguard/). Configure and test actual device capabilities before generating/applying commands.

## Increment 9: SME revenue reporting
Live router configuration/verification are deferred at the user's request. SME Revenue now supports site/date/timezone filters, gross/refunds/net cash KPIs, receipt averages, payment-review workload, and service/source/site/method/day breakdowns. Refund events belong to their own recording period; prior periods retain the original receipt. Financial events are created transactionally with approval and counter-sale workflows.

/v1/revenue/ledger is SME-only with the same filters and bounded pagination. CSV export includes signed amounts, source identifiers and timestamp basis; 10,000-event export cap requires narrowing larger periods. Operating costs/profit and independent cash reconciliation remain pending.

Migration 007 backfills legacy payments using available audits, otherwise explicitly labelled submission-date estimates. It refuses downgrade with financial events present. PostgreSQL SQL generation passes; local testing uses SQLite.

