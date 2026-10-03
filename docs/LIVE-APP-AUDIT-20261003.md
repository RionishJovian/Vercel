# Live app audit — 2026-10-03

## Verdict and scope

The app is reachable and its API/nginx services are healthy, but it is not yet a fully functional production business workflow. This audit found confirmed display/classification defects and separately identified unavailable product functions.

Read-only inspection of the signed-in live application, the published guest media page, public GET endpoints, VPS service/version state, and deployed-source equivalents. No router configuration, payment, account, production database, feature flag, publication or deployment was changed. Browser navigation and device selection only loaded inventory. Current source commit on the VPS: `5300ac9`. Audit began at 2026-10-03 13:26 UTC / 19:56 Asia/Yangon.

Priority: P1 = material security or core business readiness; P2 = misleading behavior or operational/product gap; P3 = discoverability. A missing feature or intentional safety hold is not automatically a software regression.

## Confirmed defects

### P1 — Security classification does not respect RouterOS release tracks

The live stable-channel CHR on 7.15.3 was shown an upgrade threshold of 7.23.4. Three MikroTrick advisories use a single 7.x upper bound of 7.23.4. The channel argument is discarded, and summary recommendations operate on version numbers only.

Consequently, stable 7.24.0/7.24.1 stop matching those three advisories even though the vendor identifies 7.24.2 as the fixed stable release. This is an advisory-level false negative, not a claim that the overall posture becomes Safe: other advisories can still keep the device affected. The table also explicitly acknowledges false positives for a patched long-term TLS fix.

Evidence: `internal/security/posture.go:115-149`, `:291-304`, `:345`; `internal/handler/security_handler.go:106`. Vendor: [MikroTik September 2026 vulnerability](https://mikrotik.com/supportsec/september-2026-vulnerability/), fixes 7.25 beta 3, 7.24.2, 7.23.4 and 6.49.21. This does not establish internet exposure or exploitation of the audited router.

Repair: represent branch-aware affected ranges and fixes, use the observed channel, and regression-test patched/unpatched stable, long-term and testing versions. Do not treat a numeric minimum as a universal fix.

### P2 — Revenue payment statuses render as Unknown

All six displayed payment requests had Unknown status despite being decided. Revenue passes approved/rejected/pending into the shared connectivity badge renderer, which recognizes only up/stale/offline/unknown. A local read-only execution reproduced Unknown for approved, rejected and pending. Voucher statuses reuse the same helper; unused also reproduced Unknown, although there were no live voucher rows to verify that symptom.

Evidence: `web/app.html:3578`, voucher rendering near `:3036`; `web/assets/js/app.js:4-14`.

Repair: separate payment, voucher and connectivity status renderers, preserve explicit unexpected-state errors, and test all enumerated statuses. The current full-ledger revenue totals are not the historical 50-row truncation bug.

### P2 — Topology confuses management reachability with a verified Hub tunnel

The directly managed lab CHR appeared ONLINE / TUNNEL ACTIVE in Topology. Gateways identified direct management and showed No verified signal. A reachable management target is not WireGuard handshake evidence.

Evidence: live Gateways/Topology comparison; `web/app.html:1857` maps up directly to TUNNEL ACTIVE. Root-edge/path presentation also needs to distinguish inferred relationships from measured tunnel state.

Repair: show Management reachable for direct transports; only show Hub tunnel active with fresh verified handshake evidence. Keep WAN/customer traffic health separate.

### P3 — Product guide does not cover the new network workspaces

The main navigation includes WAN and failover, PPPoE subscribers, Wi-Fi networks and Existing networks, but the 23-entry guide does not explain those workspaces. It also promises voucher creation while the current issuance controls are absent.

Evidence: live navigation/guide and `FEATURE_GUIDE` in `web/app.html`.

Repair: update onboarding and task-specific help to match live capabilities, prerequisites and holds.

## Non-functional or incomplete product workflows

### P1 — Magic Media publishes a design, not a paid streaming service

The published guest page explicitly says purchases and playback are not available yet. Its media-access button is disabled; 1,500 MMK / 24 hours is planned pricing only. The production publication response hardcodes `purchases_enabled:false` and `playback_enabled:false`. Production routes do not supply the complete guest checkout/catalog/player workflow.

Evidence: fresh public-page inspection; `internal/handler/media_publication_handler.go:100`, `internal/server/media_routes.go`, `web/assets/js/media-appearance.js:71-77`.

Repair/acceptance: wire the selected automated payment provider, verified/idempotent payment confirmation, timed entitlements, authenticated owner-PC Jellyfin catalog/playback and revocation/expiry. Keep API keys on the owner's computer. The current uncommitted pass/playback foundations are not deployed functionality. Provider identity and real payment integration remain unresolved; do not substitute a manual proof workflow without approval.

### P2 — Media connector is a short-lived terminal pilot

The live connection panel said Not connected and instructed the owner to keep a terminal open. Sessions last 24 hours; the production token expires after 24 hours and no automatic renewal workflow is present. This is not an unattended, non-technical business deployment.

Evidence: live Connection panel; `internal/media/bridge/postgres.go:56`, `:92`, `:140`; connector instructions in `web/assets/js/media-dashboard.js`.

Repair/acceptance: packaged local connector/service, explicit secure pairing, renewal/reconnect, service restart/offline recovery and clear diagnostics. A disconnected computer alone is not evidence of a server bug.

### P1 — Guest voucher issuance is unavailable and the UI hides why

The voucher page offered layouts/sample printing but no Create guest internet vouchers controls. A live anonymous GET to `/api/v1/guest-vouchers/config` returned 404, consistent with feature-disabled route registration, rather than the authentication response an enabled protected route would return. Source suppresses registration when VoucherAccessEnabled is false. The UI catches configuration failure as null and hides issuance, making disabled, unauthorized and failed states indistinguishable.

Evidence: `internal/server/guest_voucher_routes.go:25`, `web/app.html:3022-3036`; live HTTP 404. No feature flag was enabled and no code was issued or redeemed.

Repair/acceptance: show an explicit capability/error reason; complete schema and real guest internet/quota/retry acceptance before enabling. Printed SAMPLE layouts do not grant internet access.

### P2 — Wi-Fi SSID editing is held, not functional on the live app

The selected-router UI reported physical-radio acceptance pending and offered no SSID editor. Production has network controls enabled but Wi-Fi controls explicitly false. The available CHR also has no onboard radio.

Evidence: live Wi-Fi hold, read-only production drop-in values, `docs/NETWORK-CONTROLS.md` and `web/assets/js/network-control.js`.

Repair/acceptance: approved physical modern/legacy radio tests, package/profile/CAPsMAN handling, reconnection and rollback. Retain the hold until those tests pass; do not simply enable the flag on a CHR.

### P2 — Templates is a catalog without an execution workflow

The live page listed 51 built-in templates and custom add/delete controls, but no target selection, review or run action. Corresponding routes are CRUD; listing a task is not running it.

Evidence: live Templates; `web/app.html:1105-1218`, template routes in `internal/server/server.go`.

Repair: either provide typed, role/tenant-scoped review/apply with backup/readback/audit, or label the page as a reference library and direct users to supported task controls. Do not introduce unrestricted production command execution.

## Healthy and present, with limits

- API and nginx active; VPS `/health` returned 200; deployed source `5300ac9`.
- Overview/Fleet/Topology loaded the one managed router and current availability. No open recovery incidents were shown.
- WAN has primary/backup and equal-cost sharing controls. PPPoE live inventory loaded subscriber service/account/plan tasks and suggested `192.168.50.1/24`, `.10`–`.250` pool and DNS/speed defaults. Existing networks loaded DHCP, pool, DNS/gateway, Hotspot, reservation and bandwidth task choices. No writes were performed.
- Network setup has a guided gateway chooser and links to the dedicated business workspaces. Port roles still require deliberate selection rather than unsafe guessing.
- Captive Portal now exposes gateway/profile/review/deploy controls. This audit did not deploy to a router or certify captive guest access.
- Backups displayed an existing snapshot. A historical WebFig certificate-signing failure was visible; it was not reproduced and is not classified as a new failure.
- Revenue loaded a full ledger. Store/subscription, Team, Translations, AI usage and Platform views loaded. Empty incident/scan/terminal/event lists are not automatically bugs.
- Current browser error-log sample was empty. `node scripts/test-network-workspaces.mjs` passed roles, capabilities, freshness, preview-race and single-map-form regressions; this is local regression evidence, not production transaction acceptance.
- Eight compared key source-file hashes matched the VPS checkout. Installed app HTML contains deployment asset-version rewrites, so byte inequality alone was not classified as a bug.

## Not certified by this audit

Production role/tenant negative cases, paid checkout/webhook settlement, captive login/real guest browsing, voucher redemption/expiry/data exhaustion, physical AP edits, real ISP link-cut WAN failover, throughput, full restore/reboot, and unattended connector renewal were not exercised. The lab CHR has only one discovered interface; it cannot establish physical dual-WAN or Wi-Fi acceptance. Prior isolated-lab acceptance is documented separately, not rerun here.

No conclusion of whole-app completion or absence of security vulnerabilities is warranted from healthy HTTP responses and read-only pages. Local Jellyfin playback and unpublished media foundations are separate from this live production audit.

## Recommended repair order

1. Correct security release-track classification and payment status rendering.
2. Correct misleading topology/capability states and guide coverage.
3. Finish voucher guest acceptance and physical Wi-Fi acceptance before rollout.
4. Finish end-to-end Media payment, owner-PC streaming and unattended connector operation.
5. Add a safe template task workflow or accurately scope the reference library.
6. Run authorized production acceptance across roles and real guest/customer flows, recording implemented → tested → migrated → deployed → verified separately.
