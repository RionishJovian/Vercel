# Gateway, guest commerce and portal security audit

Audit date: 2026-10-01. Read-only audit; no production settings, router configuration, accounts or payments were changed. This report supersedes historical voucher-readiness scorecards for the maintained Go application.

## Verdict and evidence boundary

The complete paid-hotspot workflow is **not certified production-ready**. Bank-transfer guest quota application exists, but printed voucher redemption is not connected to RouterOS guest authorization. Gateway network configuration is not the same as deploying the branded portal. Revenue is not a complete financial ledger.

Reviewed maintained code under `internal/`, `web/app.html`, and `web/portal.html`. Checked the app VPS over SSH without printing credentials. All eight initially compared workflow source files matched local hashes: server routes, portal/voucher/guest handlers, guest quota applier, gateway setup, portal HTML and app source. Five additional files (provisioner wiring, guest repository, quota repository, voucher semantics, SSH transport) also matched. The installed portal HTML matched. Installed app HTML differed from repository source; directly inspected its revenue function and confirmed the same truncation/partial-source behavior. Do not treat every installed frontend byte as synchronized.

API service was active; public portal returned HTTP 200. This establishes availability, **not** successful guest login or security enforcement.

Tests passed: `go test ./internal/provisioner/routeros ./internal/voucher ./internal/handler ./internal/hub`; `node scripts/test-hub-ui.mjs`; `node scripts/test-network-workspaces.mjs`. Router tests use fake command runners unless explicitly configured otherwise. No disposable PostgreSQL integration database or real guest traffic test was used in this audit. No live router firewall, IPv6, FastTrack, IP-binding, walled-garden or quota-exhaustion configuration was captured.

## Workflow status

- Gateway records → Magic Hub: signed read-only lookup exists. `EnrollPeer` is implemented as a client method but has no maintained application caller beyond tests. Saving a connection or choosing `hub` does not enroll the router or install its tunnel. Live lookup remains gated by the older WebFig/Sovereign entitlement. Sources: `internal/hub/client.go:93`, `internal/handler/vps_handler.go:41`, `internal/handler/hub_lookup.go`.
- Config tools: admin-gated live tools use tenant-resolved targets and management CIDR policy. Gateway setup previews settings, requires a matching confirmation hash, performs live preflight, takes backups, reads back writes and attempts targeted rollback. It creates IPv4 addressing/DHCP/NAT/optional Hotspot, not a complete portal/security deployment. Sources: `internal/server/server.go:543`, `internal/server/provisioner_wiring.go:58`, `internal/provisioner/routeros/gateway_setup.go:262`.
- Guest MMQR: creates a pending purchase, applies a temporary RouterOS user and router-side deadline scheduler, then owner approval changes paid caps and forces reauthentication. Implemented and unit-tested; relevant deployed source matches. Migration state and real-device behavior remain unverified here.
- Printed vouchers: database redemption and business WebFig entitlement are implemented. Guest internet provisioning is missing. Do not call this migrated/deployed/verified hotspot quota enforcement merely because those database handlers are deployed.
- Portal: branding/preferences and hosted preview are implemented/deployed. Router upload, router login redirect/context bootstrap, walled-garden provisioning and a deployment/readback version check were not found in maintained code. Historical migrations/old frontend functionality are not evidence of current executable wiring.
- Revenue: aggregates approved requests from two different purchase pipelines. Deployed renderer inspected; completeness and accounting semantics are defective below.

## Findings, in repair order

### P1 — Viewers can perform monetary and portal mutations

Protected routes have a VIEWER baseline. Guest approve/reject (`internal/server/server.go:748`), portal configuration PUT (`:729`), and voucher create/update/delete (`:608`) have no stronger role guard; their handlers check tenant identity but not a higher user role. By contrast, legacy billing explicitly requires OPERATOR. Hiding sidebar buttons is not authorization.

Impact: a tenant viewer can approve unpaid guest access, edit bank/QR destination or plans, and mutate vouchers. Voucher status PUT accepts `active` for a used record (`internal/handler/voucher_handler.go:247`, `internal/database/voucher_repo.go:516`), permitting repeated redemption while expiry is still valid. An atomic claim does not prevent this deliberate status reset.

Required: explicit role policy for each mutation, deny viewer requests server-side, immutable redemption transitions or a separately audited reversal operation. Add route-level negative tests for every role. Code-path finding; no production mutation attempted.

### P1 — Printed WiFi vouchers do not grant WiFi

`internal/voucher/voucher.go:100` resolves only `sovereign`, which extends the business WebFig pass. Shared redemption at `internal/handler/voucher_handler.go:413` applies that entitlement. `internal/handler/portal_handler.go:167` writes a PortalSession but never creates/authenticates a Hotspot user or applies guest time/data limits. `web/portal.html:323` nevertheless announces the device is connected.

Required: distinct guest-voucher plan/quota model, immutable device/profile binding, router/RADIUS authorization with readback and failure reconciliation. Do not consume a code and claim internet success when provisioning has not succeeded. Decide whether validity starts on issuance or first use: current `ComputeExpiry` starts on issuance.

### P1 — Portal deployment and bypass hardening are incomplete

Gateway setup sets `html-directory=hotspot` and `login-by=http-chap,mac` (`internal/provisioner/routeros/gateway_setup.go:178`). No maintained upload of branded login files, cloud-portal redirect/bootstrap, or walled-garden commands was found. Saving staff portal configuration is database-only.

The setup guard protects forwarding during setup and is removed after readback. It is not a permanent guest isolation policy, IPv6 deny policy, or bypass-rule audit. Management hardening disables/restricts management services; it does not certify captive portal protection.

MikroTik documents Hotspot as IPv4-only and IP bindings as capable of bypassing authentication: <https://manual.mikrotik.com/docs/authentication-authorization-accounting/hotspot-captive-portal/>. Consequently, an IPv6-enabled guest network needs a separately verified restriction; **an exploitable IPv6 bypass on the present gateway has not been demonstrated**. Also inspect FastTrack, bridge/VLAN escape routes, RADIUS fallback, cookies and existing bypass bindings on the actual router.

Required: explicit portal deployment job with backup/readback/rollback; minimum required HTTPS portal/payment destinations; prohibit broad wildcard/CDN allow rules; verify unpaid access is blocked from a real guest device over IPv4 and IPv6. Reference for custom router login pages: <https://manual.mikrotik.com/docs/authentication-authorization-accounting/hotspot-captive-portal/hotspot-customisation/>.

### P1 — Revenue totals are incomplete and mix different products

`internal/handler/guest_wifi_handler.go:647` requests only 50 rows; repository SQL applies that limit. `web/app.html:3455` presents their sum as “All time,” also using them for 7/30-day totals and the funnel. More than 50 requests silently loses older sales, including recent approved sales behind newer pending requests.

The renderer concatenates legacy billing and guest WiFi. Legacy billing resolves the same business WebFig entitlement rather than guest internet (`internal/handler/billing_handler.go:311`). Its amount is anonymous guest input (`:126`), not authoritative server pricing. “Verified approved sales” means manual approval, not bank-settlement verification. A failed source is silently replaced by an empty array while the remaining source still displays totals. Browser-local dates, rather than the site's timezone, determine daily buckets.

Required: server-side full-period aggregates, authoritative prices, separate product/source ledgers, explicit partial-data warnings and Asia/Yangon/site-timezone boundaries. Export CSV quoting does not neutralize spreadsheet formulas in untrusted bank references; protect formula-leading cells.

### P2 — Paid purchase duration can change after checkout

`internal/handler/guest_wifi_handler.go:568` re-reads current portal plan minutes during approval, while amount/data are stored on the request. Owner edits or deletion between checkout and approval can change the purchased duration or prevent approval. Snapshot all commercial terms/version at purchase creation.

### P2 — Router writes and database approval are not one atomic operation

Guest decision holds a database transaction/row lock while executing multiple router commands (`internal/handler/guest_wifi_handler.go:588`). Router success followed by DB commit failure can leave access paid on-router while the database remains pending. Router idempotency via the paid-request comment is a useful retry protection; it does not automatically reconcile failures or update revenue. Partial failures during reauthentication/scheduler removal also need recovery.

Required: durable desired/applied states or an outbox/state machine, bounded lock time, retry/reconciliation and an operator-visible failed/pending-apply status. Preserve exactly-once quota additions.

### P2 — Concurrent pass extensions can lose purchased time

`internal/database/quota_repo.go:58` reads existing quota without `FOR UPDATE`; `internal/handler/voucher_handler.go:431` computes a new expiry then writes it. Two different codes/legacy purchases can read the same expiry and overwrite each other's extension despite each claim being atomic. Use an atomic database expression or serialize the entitlement row, including first-row creation. PostgreSQL concurrency test still required.

### P2 — Public portal controls need stronger binding and abuse limits

Public routes have no dedicated rate limiter in `internal/server/server.go:490`; the bounded auth limiter is attached only to `/auth`. Public status looks up MAC globally (`internal/handler/portal_handler.go:218`), not a signed guest session bound to tenant/device. Voucher redemption accepts caller MAC without RouterOS host verification. MMQR grace does check the live router host table—good—but MAC presence is not proof the HTTP caller owns that client.

Receipt attachment authorizes using request ID and grace window only (`internal/handler/guest_wifi_handler.go:254`); it does not bind a secret receipt token to the submitting guest. Upload/DB failures are swallowed yet `receipt_attached:true` is returned. Last-six/phone shape is not server-validated here. Receipt files are private tenant-scoped `.bin` files, not executable HTML, which is a useful protection.

Required: short-lived signed device/client context, receipt submission token, scoped status, strict input/body limits and bounded per-client/device/IP abuse protection. Grace lookup is a **24-hour** per-MAC window, not a lifetime lock; MAC changes can evade that business policy. Do not represent MAC login as strong person authentication.

### P2 — Consent setting is not enforced

Config stores `ConsentRequired`; redemption creates `ConsentGiven:false` regardless (`internal/handler/portal_handler.go:167`). Portal displays terms but has no required acceptance control. Enforce consent server-side and record the accepted terms version before granting access.

### P2 — Injection defenses are partial; security headers absent

Portal business text, plan labels, errors and HTML attributes use `esc`; branding permits literal hex colors; RouterOS names/interfaces and MACs are constrained and commands quote string values. These are meaningful defenses; no working stored-HTML/script injection was demonstrated.

MMQR image URL has no server scheme/origin allowlist, permitting arbitrary third-party image loading/tracking. Public `portal.html` response had no CSP, HSTS, X-Content-Type-Options or Referrer-Policy headers in this check. Escaping alone is not a complete policy. Add validated HTTPS assets and a CSP compatible with the intended same-origin preview embedding rather than blindly blocking every frame.

SSH transport includes full command text in command errors (`internal/provisioner/routeros/transport.go:89`). Secret-bearing commands can expose credentials through error propagation (temporary user's MAC-auth password; other commands may contain shared secrets). Redact command arguments and provider output before returning or auditing errors. No secret was deliberately triggered or captured.

### P2 — Gateway “verified” is settings readback, not service acceptance

`ApplyGateway` verifies requested settings and removes its setup guard; it does not test a DHCP client, DNS, guest internet, portal fetch, payment callbacks, quota exhaustion or the restored management path under realistic traffic. Preview hash is settings confirmation, not an expiring device-bound approval token. Scope its UI wording accordingly; keep the stronger real-device acceptance checklist separate.

### P3 — Voucher lookup route/parameter mismatch

GET `/vouchers/:id` routes to `GetVoucherByCode`, which reads `c.Param("code")`. Align the contract and cover it with an HTTP route test.

## Quota assessment

MMQR grace code validates a live Hotspot host, validates MAC login, sets decimal-MB upload+download total and connected-time caps, installs/readbacks a wall-clock deadline scheduler, and does not reset counters on retry. Paid conversion adds purchased caps to consumed counters, binds a request comment and removes active sessions for reauthentication. Unit tests cover these protections.

Paid minutes mean **connected uptime**, not elapsed validity: the paid path removes the grace scheduler and sets uptime limits without a replacement paid wall-clock expiry. If plans promise “valid for 24 hours after purchase,” that promise is not implemented. A time-only plan has no data cap, a data-only plan has no time cap. That is intentional code behavior, but must match product labels. Docs: MikroTik Hotspot user limits at the official manual linked above.

Verdict: implementation is plausible and test-backed for MMQR caps; **real RouterOS enforcement is unverified**, and printed-voucher quotas are not implemented through the current redemption path.

## Required acceptance checks before paid rollout

1. Viewer/operator/admin/owner role matrix; cross-tenant device, voucher and payment IDs; public device suspension policy; RLS integration with restricted DB role. Runtime superuser/BYPASSRLS remains a deployment risk even when tenant transactions switch role correctly.
2. Enroll a real gateway through Magic Hub, validate fresh tunnel handshake and pinned identity, then verify tenant-scoped config tools across that path. No invented “online” state from a saved record.
3. Deploy the branded portal and exact walled garden; connect a real unpaid client; test discovery/redirect, portal assets, DNS, QR/payment reachability and blocked unrelated destinations.
4. Test voucher first-use, code reuse/status reset, expiry, upload+download quota, uptime, reconnect, router restart, API outage and concurrent different-code purchases.
5. Test MMQR grace deadline offline/API-down, both quota thresholds, slow owner approval, paid cap conversion and retries after router/DB/network failures. Verify timezone and scheduler privileges on supported RouterOS releases.
6. Test IPv6 and static-IP bypass, MAC changes/spoofing, FastTrack, VLAN/bridge escape, user profiles, RADIUS/cookies and unintended IP bindings; confirm no guest access to management/LAN services.
7. Reconcile >50 requests and both ledgers with authoritative prices; refund/cancel semantics, duplicate bank references, partial backend outage, CSV safety and daily timezone boundaries.
8. Test HTML/attribute/CSS/URL/RouterOS input handling without production payloads; restrictive asset URLs, upload boundaries, origin policy, secrets redaction and compatible browser headers.

## Authorization and phase tracking

This audit authorizes no fixes. Future work must explicitly record **implemented → tested → migrated → deployed → verified** for each workflow, with evidence and remaining checks. A source match is deployed-source evidence, not proof that the current running binary contains it or a gateway has the desired settings.

Router writes, firewall/walled-garden changes, production migration/deployment, paid-access issuance/revocation, account deletion and real-payment actions require authorization and a recoverable plan. The recommended first implementation slice is server-side role/transition enforcement, followed by a genuine guest voucher + portal deployment state machine, then authoritative revenue aggregates and real-router acceptance testing.
