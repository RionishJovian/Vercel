# Router-local broadband release — 2026-11-04

The owner explicitly approved merging and deploying this batch after CI passed.
PR [#99](https://github.com/RionishJovian/Vercel/pull/99) was merged normally.

- Feature commit: `2dcef631324be37138079c6a6c190d2087a4f310`.
- Production release: `a0337d90ec91b70e6fe733e2b6deec08e2fa6ea0`.
- [Production workflow](https://github.com/RionishJovian/Vercel/actions/runs/41750507225): validation, image build and deploy-vps all successful.
- Previous running release: `a87397fc7e65627f5e91f48e797fdc156602c2a9`.
- Database backup and previous-image rollback directory:
  `/root/mm-rollout-backups/ci-a0337d90ec91b70e6fe733e2b6deec08e2fa6ea0-20261104T010500Z`.
- Additive migration `20261104000000_broadband.sql` applied; runtime grants and
  policies rerun by the existing migration pipeline.

The release adds paid PPPoE account installation, first-use validity, renewal,
suspension, router-local expiry, idempotent recovery and separate recorded-payment
reporting. It also fixes Hotspot catch-up expiry. Existing router-write gates were
preserved; `MM_NETWORK_CONTROLS_ENABLED` was already `true` before deployment.
No subscriber was installed and no router configuration was changed by this run.

## Verification

- PR validation, Go and JavaScript/TypeScript CodeQL, and Vercel preview checks
  passed. Local verification included the uncached full Go race suite with
  disposable PostgreSQL, 186 frontend tests, production packaging, 22 critical UI
  checks and 57 migration-runner checks.
- Both running API and web containers report the production release SHA above.
- Public readiness reports database, Redis and NATS all OK.
- Public console HTML and script bytes match the running release container.
  Console HTML SHA256:
  `8038f8f54d1231e914c34e0c41e37cb314773dab5783ea32432f5c20c8ddf2d3`.
- Public CSP retains script restrictions without unsafe inline-script permission.
- Landing, sign-in, pricing and guest portal return HTTP 200; the existing
  public-only smoke script passes its two checks.
- Unauthenticated `/api/v1/me` and `/api/v1/broadband/report` return HTTP 401.
- All three broadband tables have forced RLS, tenant policies and service runtime
  policies; the new migration is recorded exactly once.
- After removing local API fixture interception, the browser's existing signed-in
  session loaded the real Paid broadband accounts and PPPoE recorded-payment
  sections. Broadband report, existing voucher report and router list return 200.
  No script exception or CSP violation was observed during those page checks.
- Store and Jellyfin screens still render. Store addon/purchase reads return 200.
  The media connection endpoint returns 404 in this workspace; live Jellyfin
  connector operation was not verified or changed by this release.

Physical RouterOS script execution, outage/clock/reboot behavior and real subscriber
installation remain untested. Traffic counters are observed samples, not a complete
offline accounting journal. See [implementation and operating boundaries](ROUTER-LOCAL-BROADBAND.md).

No running-container files, account passes, Store purchases, issued vouchers or
media connector records were edited manually. Local notes from the previous
hardening deployment and RouterOS study were preserved and not published.

## Follow-up router trial readiness — 2026-11-04

Read-only checks for the registered hAP ax3 Dev found:

- Magic Hub enrollment capability is available and reports `ready`.
- The peer has `no_recent_handshake`, with no reported handshake timestamp.
- Telemetry reports `reconnecting`, with no successful retained reading.
- Network inventory returns 409 `router_management_required`; quick config
  reports that the device has no SSH target configured.

The existing Connect Magic Hub wizard was opened and its first-step script
prepared for local use. It was not executed, and no enrollment confirmation or
router change was submitted. Local router access is required to establish the
initial trusted connection before physical PPPoE testing can proceed.

## Follow-up UI correction — latest production release

Software-only work continued after the owner confirmed local router access was
unavailable. [PR #100](https://github.com/RionishJovian/Vercel/pull/100) adds explicit
Magic Hub connection guidance, a read-only recheck, disabled new installation until
usable PPP plans load, and separate messaging for failed read-only refreshes.
Existing saved-operation retries remain available when inventory alone fails.

- Feature commit: `298f63fa70866dc9c8c2f7fe59b8c563e319cacb`.
- Latest production release: `e239967680d6b78f91389112b104a88cdc0a4d95`.
- [Release workflow](https://github.com/RionishJovian/Vercel/actions/runs/41754185743): validation, image build and deploy-vps all successful.
- Previous release: `a0337d90ec91b70e6fe733e2b6deec08e2fa6ea0`.
- Rollback directory:
  `/root/mm-rollout-backups/ci-e239967680d6b78f91389112b104a88cdc0a4d95-20261104T013741Z`.
- No migration was added; the deployment runner reported the schema up to date.

Verified 191 frontend tests, production build and lint (the existing hook warning
remains), followed by green CI/CodeQL/preview checks. Local browser fixtures checked
both disconnected and recovered states at mobile/desktop sizes.

Post-deployment checks confirmed both image revisions, public readiness, served
HTML/script hashes, authentication protection and the retained broadband RLS
policies. Public console HTML SHA256:
`a5f6c9cdcd4474bd42c7a047451e38fe7b7c9316878b206efa83711821eef43f`.
The real signed-in page displays the expected connection guidance, its installation
control is disabled, and the connection recheck returns the existing 409 setup-
required response. No overflow within the section at 390px and no observed script
exception or CSP violation during that check. No subscriber, payment or router
changes were submitted. Physical-router testing remains deferred.

## Paid retry identity correction — latest production release

[PR #101](https://github.com/RionishJovian/Vercel/pull/101) preserves an unconfirmed
installation/renewal operation ID when the same details are retried on the current
screen. Confirmation explicitly labels that retry. Only the exact operation's
acknowledgement or definitive rejection clears its identity; other uncertain
payments are retained. Suspend/resume do not reuse cached obsolete actions. State
is held in component memory and cleared on unmount, never browser storage.

- Feature commit: `a090d287c1130b4a4beec12da1ae7d43d2091e17`.
- Latest production release: `a956244d1b1e9c2c5c06081214f94989d20ec7f9`.
- [Release workflow](https://github.com/RionishJovian/Vercel/actions/runs/41759518034): validation, image build and deploy-vps all successful.
- Previous release: `e239967680d6b78f91389112b104a88cdc0a4d95`.
- Rollback directory:
  `/root/mm-rollout-backups/ci-a956244d1b1e9c2c5c06081214f94989d20ec7f9-20261104T022403Z`.
- No migration added; the deployment runner reported the schema up to date.

Verified 198 frontend tests, build/lint and a focused code review. Regressions cover
lost success replies, subsequent purchases, rejected creates, saved recovery,
intervening suspension/resume, separate accounts and delivery order differing from
click order. The local browser fixture confirmed three attempts produce two
distinct payments: one retry reused its ID and the next acknowledged purchase got
a new ID. Those were synthetic requests, not production payment tests.

Post-deployment image revisions, served asset hashes, public readiness,
authentication protection and tenant/runtime policies passed. Console HTML SHA256:
`1d264010cff3dc699a22e3500c221cff3404d2f6ee3559cfe2adeb74be4d6f17`.
The real signed-in broadband page loads and retains its connection guidance with
no observed script exceptions. No live payment, subscriber or router changes were
submitted. Physical-router testing remains deferred.

## PPP-plan label compatibility — latest production release

[PR #102](https://github.com/RionishJovian/Vercel/pull/102) replaces username-style
validation of source PPP-plan labels with a shared literal-label validator in the
API and router adapter. Spaces, punctuation and Unicode are accepted within the
existing 64-character database bound. Control/invalid UTF-8/blank/trimmed labels
remain rejected. Names are not normalized or renamed; username rules, quoting and
source-profile hook protections are unchanged.

- Feature commit: `c46629041e9a312a6285888f319b481723bd677f`.
- Latest production release: `085f5d3242bf27cf2a0551fb11316a99a85a2ef6`.
- [Release workflow](https://github.com/RionishJovian/Vercel/actions/runs/41762442646): validation, build and deploy-vps all successful.
- Previous release: `a956244d1b1e9c2c5c06081214f94989d20ec7f9`.
- Rollback directory:
  `/root/mm-rollout-backups/ci-085f5d3242bf27cf2a0551fb11316a99a85a2ef6-20261104T025127Z`.
- No migration added; the runner reported the schema up to date.

The adapter rejection and API 409 were reproduced before the fix. Full uncached Go
race tests passed against disposable PostgreSQL, production commands compiled and
focused review found no confirmed actionable issue. Tests cover Unicode length,
invalid/control labels, quote/dollar literal lookup and lost-reply recovery with an
exact stored spaced label and one payment.

Post-deployment image revisions, served hashes, public readiness, endpoint
authentication and retained RLS/runtime policies passed. Frontend bytes are
unchanged from the preceding release. The initial signed-in browser view showed
unavailable account data; repeat API reads returned 200 for me/routers/accounts/
broadband report, and a full reload restored the normal page. The cause of the
initial view failure was not confirmed. Network inventory retains the expected 409
management-setup-required state for the disconnected router. No live account,
payment or router changes were submitted. Physical testing remains deferred.

## Account-read recovery — latest production release

[PR #103](https://github.com/RionishJovian/Vercel/pull/103) keeps the broadband
panel mounted when account reads fail. It preserves entered values and uncertain
payment identities, labels cached records stale and locks router actions until a
successful read. Owners and viewers can use **Reload records**, which only reads
saved account records; it does not run router synchronization or record payment.

- Feature commit: `54717a0cd944fb72a7bed1c514e74e2fe6088cbf`.
- Latest production release: `fda730ca25bca77253407d42d244ca0c91b5b626`.
- [Release workflow](https://github.com/RionishJovian/Vercel/actions/runs/41788119216): validation, build and deploy-vps all successful.
- Previous release: `085f5d3242bf27cf2a0551fb11316a99a85a2ef6`.
- Rollback directory:
  `/root/mm-rollout-backups/ci-fda730ca25bca77253407d42d244ca0c91b5b626-20261104T065853Z`.
- No migration added; the deployment runner reported the schema up to date.

The two regression tests failed before implementation, then all 200 frontend
tests passed. Lint/build, 22 existing critical UI tests, two deployment and eight
proxy tests passed; focused review found no actionable issues. The first critical
suite attempt was blocked by the sandbox's loopback-listener restriction; rerun
with the required permission passed. Existing NetworkEditor hook and large-map
bundle warnings remain. A local browser fixture recovered a failed read with two
account GETs total and zero writes; the new panel did not overflow at 390px.

Production API/web revisions match the release. Public readiness, served entry
scripts and the broadband lazy chunk match the release container; endpoint
authentication and three forced-RLS tenant/runtime policy sets remain intact.
Console HTML SHA256:
`7a029364e162011371288a4bccfb771cb7ecc10a012abf017116f729b47344a1`.
The signed-in broadband panel loads and retains the expected Magic Hub connection
guidance for the unavailable router. No live account, payment or router changes
were submitted. Local preview stopped and browser fixtures removed. Physical
router behavior remains untested and deferred by the owner's availability.

## PPPoE Revenue read recovery — latest production release

[PR #104](https://github.com/RionishJovian/Vercel/pull/104) adds a GET-only reload
to the PPPoE Revenue section and preserves last loaded totals with an explicit
warning after refresh failure. An initial unavailable report does not show zero;
a successfully loaded zero remains valid. Report calculations and existing
Hotspot/bank-payment workflows are unchanged.

- Feature commit: `60a95709a356b96c51e762d779dfae3b6c762a73`.
- Latest production release: `174510cf0cfcc274e19a333c0e20f26871292784`.
- [Release workflow](https://github.com/RionishJovian/Vercel/actions/runs/41790288575): validation, build and deploy-vps successful on attempt two.
- Previous release: `fda730ca25bca77253407d42d244ca0c91b5b626`.
- Rollback directory:
  `/root/mm-rollout-backups/ci-174510cf0cfcc274e19a333c0e20f26871292784-20261104T072059Z`.
- No migration added; the deployment runner reported the schema up to date.

Two regression cases failed before implementation, then all 203 frontend tests
passed, including initial failure/recovery, cached totals/pending replacement and
confirmed-zero reporting. Lint/build, 22 critical UI tests, two deployment and
eight proxy tests passed. Focused review independently passed all three new
tests and found no actionable issues. Local browser outage/recovery: three report
GETs, zero writes, retained stale totals, recovered new totals and no panel
overflow at 390px. Preview stopped and browser fixtures removed.

PR and main CI passed. Release attempt one failed in the existing media publishing
test at Chrome startup (12-second timeout, before application checks). The failed
release job was retried once without code/test changes; all validation passed on
the retry. No check was skipped or weakened. Existing NetworkEditor hook and
large-map bundle warnings remain.

Post-deployment API/web revisions, public readiness, authentication and retained
tenant/runtime RLS policies passed. Served entry scripts and Revenue lazy chunk
match the release container. Console HTML SHA256:
`d331bc478b33d0440abf0008f5a28d7c67de7b43beb5cfbe3598b4cea900c961`.
The signed-in Revenue report loads and its reload returned HTTP 200 via GET.
No live account, payment or router changes were submitted. The original software
plan is implemented; physical-router acceptance remains deferred by the owner's
availability, with the accounting and setup limitations in the operating guide.
