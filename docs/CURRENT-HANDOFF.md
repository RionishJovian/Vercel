# Current agent handoff

## 2026-10-04 uncommitted paid-media review — source validated, NOT deployed

The bank/MMQR checkout/receipt/owner-bot changes and local streaming foundations
were reviewed against HANDOVER.md. Fixed checkout-key reuse after unpublish/
republish: a key cannot return or rebind a purchase from another publication.
Scoped Go race tests passed for media, handler, server, configuration and
Telegram; scoped vet passed. All bridge PostgreSQL tests passed three consecutive
race runs against a new loopback-only PostgreSQL 16 fixture, including migrations,
approval/rejection, replay, tenant isolation, activation and expiry.
The guest receipt DOM regression passed bank terms, cookie isolation, Telegram
retry, approval refresh and refund. Existing Chromium test assertions were not
rerun in this review. The temporary database contained synthetic data only.

No production migration, deployment, payment, bot message, router/network
mutation or stash operation occurred. Paid sales remain off. The missing
guest catalog/player relay is a real implementation gap, not a request for an
automated payment provider. Do not deploy/enable checkout before that path has
passed acceptance on an owner-local media PC. See MEDIA-PAID-ACCESS.md.

## 2026-10-03 confirmed audit defects — DEPLOYED and verified

Audit-fix code `91f6ae3` was pushed and deployed with `./deploy.sh all`.
Full Go, focused race/vet and audit/network UI regressions passed.
Signed-in checks confirmed payment labels, stable fix target 7.24.2,
management-reachability topology labels, voucher notices and guide coverage.
Backup: `/home/ubuntu/mm-audit-fixes.PUV3xT`; API/nginx active and HTTP checks 200.
No migration, router change, payment activation or Wi-Fi enablement occurred.
Unrelated paid-media foundations remain uncommitted. See [audit repairs](AUDIT-FIXES-20261003.md).

## 2026-10-03 paid Magic Media — bank/MMQR owner review, integration in progress

The user clarified there is NO automated payment gateway. Reuse portal-style
bank/MMQR receipt upload and approval/rejection through each business's own
Telegram bot. The new pass ledger snapshots pricing/duration, serializes settlement
and one-time activation, rejects duplicate payment references, and enforces
refund/expiry/suspension under forced tenant RLS. The owner-PC streaming primitive
closes reads at expiry and on revoked/unavailable authorization. Three isolated
PostgreSQL race runs and full Go/race/vet checks passed. Checkout/status/upload now
call the ledger in source, but playback is NOT connected to the deployed connector;
no production migration or
deployment was performed for it. Local Jellyfin 12.1.0 was started, the owner
signed in, and a synthetic clip/test-only library was created. Native decoded
frames were observed, but a playback-error notification also appeared; sustained
native playback and Magic Media integration remain unverified. The typed local
catalog/ancestor-scoped byte-range adapter passed mocked race tests. No local
Jellyfin API key was read or reset. Media receipt submission/owner decisions and
the shared guarded owner-bot decider now exist in source. Telegram document
receipt delivery passed mock tests, and three isolated PostgreSQL race runs
passed manual review/duplicate proof/rejection/refund/tenant tests. Local media,
Telegram, handler and server race/vet checks passed. Bank/QR editor, guest-bound
checkout/status/upload and Telegram-delivery retry are now wired in source.
Receiving details are immutable per order. Synthetic browser checks passed
publishing/reload/conflict, checkout/receipt/approval/reload, delivery failure,
cookie isolation and unpublishing; three isolated PostgreSQL race runs passed
the new snapshot and guest-binding assertions. Sales default off. The guest
playback relay still needs integration and acceptance before migration/deployment. No real
bot message, payment or production migration occurred. See [paid access](MEDIA-PAID-ACCESS.md).

## 2026-10-03 broader network controls — DEPLOYED, Wi-Fi held

Supersedes the older "missing" implementation status below, not its deployment
record. Four task-first workspaces and typed inventory/review/apply routes now
exist for WAN/failover, PPPoE, Wi-Fi and existing-network settings. Full Go tests,
focused race/vet/static and synthetic browser checks passed. Two disposable,
loopback-only CHRs verified primary-probe failover/recovery, both balanced paths,
PPPoE authentication/internet, DHCP reservation delivery, and actual-record
DHCP/Hotspot/pool/profile edits. No registered customer router was changed.

No migration is needed. Code `7f432ad` was pushed and deployed with the canonical
`./deploy.sh all`; landing/app/health returned 200 and anonymous inventory 401.
Backup: `/home/ubuntu/mm-network-rollout.n96C5f`. The production service drop-in
sets `MM_NETWORK_CONTROLS_ENABLED=true` and
`MM_NETWORK_WIFI_CONTROLS_ENABLED=false`. Signed-in live checks loaded real CHR
interfaces, PPPoE address suggestions and existing-service task choices without
performing router preview/apply. The two new disposable CHRs are stopped with
their containers/disks retained. Wi-Fi needs an approved physical AP before
reconnect/rollback acceptance and enabling writes. Detailed guard/token/rollback
behavior, lab evidence and remaining boundaries:
[Network controls](NETWORK-CONTROLS.md). The staged changes also repair initial
LAN setup's scalar field/duration verification and reserve unrouted pools when
suggesting subnets. Complete-app production acceptance is not claimed.

## 2026-10-03 guided network setup — DEPLOYED, discovery verified

Dedicated Network setup replaces the LAN/DHCP/Hotspot form embedded in the router
name/DNS dialog. GET `/routers/:id/gateway/defaults` suggests a private /24 only
after tenant-scoped reads of assigned addresses and routes. Preview/apply repeat
the conflict checks. Port roles remain explicitly selected, never guessed.
RouterOS/handler/server Go race tests and focused synthetic Chromium acceptance
passed. No migration is required and no live router was mutated. App deployment
verification: code commit `a0cd08e` pushed to main and deployed with the canonical
`./deploy.sh all`; landing/app/health 200 and anonymous defaults API 401. The
signed-in live page successfully reads the registered CHR's interfaces, suggests
192.168.50.1/24, leaves ports unassigned and hides apply until preview. No live
preview/apply or router mutation was performed. Initial live discovery found
RouterOS 7.15.3 output incompatible with value-list fixtures; fixed scalar prefix
reads were verified against the pinned CHR lab without mutations and the local
race/browser tests reran successfully. Diagnostic mode:
`cmd/guest-router-check -device UUID -network-inventory`.

Multi-WAN load balancing, WAN failover, PPPoE and Wi-Fi SSID editors remain missing;
this slice does not certify the whole app production-complete. Scope and next
acceptance boundaries: [Network setup workflow](NETWORK-SETUP-WORKFLOW.md).

## 2026-10-02 subscription-only policy — DEPLOYED

Supersedes the older legacy plan-gate status below. The user hit the still-live
Sovereign restriction after Hub rollout. The complete monthly/annual policy is
now active, with one 14-day new-business trial and one 14-day transition trial
for existing unpaid businesses. Migration reruns never extend trials. Existing
paid periods and ledgers are preserved. Shared subscription middleware covers
tenant APIs and separate media owner routes; expiry retains renewal/account
recovery routes but not a role or tenant bypass. Legacy Sovereign quota sweep is
disabled in this mode. New registration writes trial under tenant RLS.

Production config defaults now bind environment-only subscription keys, tested
without YAML keys. Cookie-based browser login responses omit reusable tokens.
Hub guide has four visible steps, spaced controls, an unenrolled-gateway hint,
and console navigation now preserves /app.html on refresh. Live read-only
preparation script retrieval succeeded with the existing signed-in annual account;
Hub lookup no longer returns Sovereign denial. The CHR record still has no
verified Hub enrollment, so no live router success is claimed.

Staged full Go/race suites and all three disposable subscription DB/HTTP tests
passed without skips. Store, navigation, layout and Hub UI tests passed.
Canonical deployment and SSH/capacity/store/session preflights passed.
API active, landing/app/health 200; anonymous Hub and subscription APIs 401.
Production was selectively patched, preserving unrelated dirty baseline work.
Guest recovery, LAN rollout and physical MikroTik verification remain separate.

Protected backup: /home/ubuntu/deploy-backups/subscription-20261002/.
Policy drop-in: /etc/systemd/system/mega-magic-api.service.d/98-subscription-policy.conf.
API binary SHA256: 3efa7f90a0325985177758c87ca4728431cc2e780692852573538e20351817bc.
DB backup was verified with pg_restore --list before applying migration.
Do not restore a production database just to roll back app code: preserve issued
trial dates. See docs/SUBSCRIPTION-ONLY-ROLLOUT.md.

## 2026-10-02 Hub enrollment + management authentication — DEPLOYED

Supersedes the older LOCAL ONLY enrollment/script notes below. User explicitly
authorized live deployment/GitHub update and skipping physical verification.
After a safety-review pause the user separately approved persistent forwarding:
only app 10.77.0.9 -> wg0 router subnet 10.77.0.0/24, TCP/22. UFW rule is installed;
no public management port or customer-to-customer/general LAN forwarding opened.

Live Gateways UI now provides Connect new gateway -> router-local public-key
preparation -> create-only signed Hub enrollment -> WinBox connection/disconnect
scripts -> Check Magic Hub -> pinned SSH credential verification and secure save.
Owner/admin management permission, tenant gateway ownership and existing plan
gates remain enforced. A record or Hub allocation is not presented as a verified
management connection. SSH account/firewall configuration on the customer's
router still requires the owner; no router settings were changed remotely.

Management POST /api/v1/vps/peers/:deviceId/authenticate derives the IP exclusively
from the signed Hub allocation and requires a recent live handshake, private /32,
configured outbound CIDR policy, owner-confirmed SHA256 SSH host fingerprint and
fixed read-only RouterOS resource verification. Password is never returned;
encrypted storage uses a short RLS-bound tenant transaction after the network
probe. Existing host-key changes require separate repair instead of silent TOFU.
The deployed forwarding supports SSH/22; other ports need separate policy review.

Hub create-only POST rejects foreign/key/port/state conflicts, reserves app/live
WireGuard allocations outside the JSON store, and refuses live key/address
collisions before sync. Durable enrolling intent precedes live writes. Same-key
retry recovers interrupted enrollment or missing live state after WG reset without
new addresses. Normal verified retries do not write. Handshake status no longer
falls back to stale stored JSON. New router AllowedIPs include Hub/app private
/32 return routes, not a default route. No existing peers changed at deployment.

Production was patched selectively against its dirty source baseline. Unrelated
subscription/guest-recovery/LAN activation remains untouched, not silently rolled
out by this release. In particular temporary LAN UI/routes and traffic analytics
are NOT claimed complete; existing legacy plan gates are still live until the
separate monthly/annual/transition-trial policy rollout is completed.

Proof: local and staged-production Go suites passed; Hub signed-route simulation,
address reservations/crash retry, UI consent/escaping/copy guards, network workspace
and layout tests passed. Canonical deploy checked SSH/capacity/store/auth schemas.
Live API/Hub services active, landing/app/health 200; bootstrap/enroll/authenticate
anonymous 401; unsigned Hub POST 401. Protected-environment enrollment config
preflight passed. Live signed HTTPS existing-peer inspection and cross-tenant 404
passed with no writes. Physical RouterOS script execution, actual gateway SSH,
customer configuration/apply and LAN reachability are SKIPPED/unverified.
Read-only production management preflight also passed: configured cipher
round-trip, RLS tenant role/context, UPDATE privileges for credential columns and
zero device visibility for an unrelated empty tenant. No customer row was changed.

App rollback: /home/ubuntu/deploy-backups/hub-management.XKWv4d.
Hub rollback: /opt/mikromagic-hub/rollback-enrollment.A7VcmN.
API binary SHA256: 1e12adfd3742ceb923a77ce28284e58c481f20ef1231bb1bf5ac91e7fcf4b521.
Hub service SHA256: 0fc66bc29c970469fe3cdff9f13c6234692e01680f0ec2a718d760487a6b76f6.
Hub helper SHA256: f15623cb9f8fa93418d46de41b822addf7af914b592d1bb61b1ea97497122936.
Live module SHA256: b39f03166eee300a5c5871c3d2839b4241ee9ac210ac552c21640ca8e536f60d.
Enrollment is enabled via 98-magic-hub-enrollment.conf; lookup signer remains in
the existing protected environment file. Never print/copy its secret into Git.

## 2026-10-02 current-project WinBox script flow — historical local implementation

Current Go/static project now implements authenticated GET gateway bootstrap
and adds router_script/disconnect_script to verified create-only enrollment.
internal/hub/router_script.go validates gateway identity, canonical nonzero public
keys, endpoint/port, private IPv4 /32 tunnel address and narrow private /32 routes.
Scripts use a stable gateway-derived dedicated interface and ownership comments.
Bootstrap creates the interface only if absent and prints only its public key;
the private key is generated by RouterOS and never sent to the app. Setup checks
ownership/key/peer/address/route conflicts before writes, only adds missing owned
resources, keeps WAN/LAN/firewall/users/services unchanged and uses 25s keepalive.
Disconnect removes owned tunnel resources, preserves interface/key/Hub allocation.
Conflicting or disabled resources fail instead of being replaced silently.

Gateway UI offers preparation and connection script panels, safe copy/manual-copy
fallback, escaped textareas and a disconnect panel. Connect new gateway creates
the MAIN_GATEWAY record with a duplicate-click guard, preserves workspace during
rerender and opens its enrollment wizard/preparation script automatically.
Bootstrap fails closed when enrollment rollout is disabled. Enrollment rejects
incomplete Hub script data instead of issuing guessed endpoint/key values.

Race tests passed for hub/handler/server. UI tests cover bootstrap duplicate guard,
encoded gateway identity, safe textarea rendering, clipboard fallback, missing
script rejection and old consent/detached-page/error guards. Network workspace
and layout regressions passed. No RouterOS parser/hardware execution is claimed.

Still required: live create-only Hub patch and app feature rollout; management
credential onboarding and pinned-host-key authenticated probe (currently only
cmd/router-register writes encrypted SSH credentials). This script connects the
tunnel, NOT management authentication. No public management ports are enabled.
No VPS deployment or live router changes in this implementation slice.

## 2026-10-02 first-project onboarding reference located

User requested reuse of the first project's gateway workflow. Existing local
reference sources confirm it was more than connection-record CRUD:

- `ui-reference/app.routers.tsx` save success receives hub.routerScript and
  rollbackScript, opens a page-level paste window even when site filtering would
  hide the card, and reports missing script as not connected rather than success.
- `lib/cloud-vps.server.ts` buildCloudRouterScript generates the WinBox New
  Terminal setup; buildCloudRollbackScript supplies removal. Script performs WAN
  preflight, configures WireGuard/management access and prints handshake guidance.
- UI guidance explicitly tells owners Add router -> paste -> Check now -> Test.

Use that interaction as the onboarding reference for the Go/static console.
Do NOT transplant the old script verbatim: it embeds a server-issued router
private key, removes existing magic-cloud resources, changes the full user group
policy, enables/configures www-ssl and adds broad tunnel input rules. New workflow
must preserve existing keys/settings on retries, scope management access, clearly
confirm changes and offer rollback only for resources owned by this enrollment.
Current new enrollment uses a router-generated public key; a safe initial
router-side key/bootstrap step is still needed for copy/paste onboarding.

No reference script was executed, router altered or onboarding deployed during
this inspection. Keep successful tunnel handshake and authenticated management
verification separate from record creation and Hub allocation.

## 2026-10-02 enrollment UI and LAN controls — LOCAL ONLY

MAIN_GATEWAY cards now contain an admin-only inline Connect Magic Hub form with
public-key/port guidance and explicit consent. The separate frontend module
guards duplicate submissions, escapes responses and distinguishes peer enrollment
from router setup and LAN verification. No private key is requested. UI fixture
tests passed for role visibility, consent, detached pages and unavailable rollout.

New ephemeral LAN Sessions bind exact tenant/gateway/operator/address/port and
expiry. Revocation, shutdown and expiry close streams; pending dials are cancelled
and late results denied. Restart invalidates session IDs. Revocation remains
possible after inventory/reachability changes, but only for the authenticated
managing owner of that session. Ten sessions per operator, eight streams per
session and 1000 total are enforced locally.

GatewayLANTransport uses pinned SSH direct-tcpip through the server-resolved
gateway, never a direct app-host connection to the LAN destination. It uses a
bounded handshake and independent socket expiry. A localhost-only SSH fixture
verified the exact forwarding destination and successful streaming/cleanup.
RouterOS local SSH forwarding must already be explicitly enabled by the operator;
this transport does not change SSH, firewall or public management configuration.

LANAccessHandler create/revoke controls enforce authenticated admin-level roles,
strict bounded JSON, explicit consent, 1–60 minute durations and a mandatory
server-owned scope resolver. Session creation reports connectivity_verified=false.
Routes are deliberately NOT mounted: verified fresh LAN discovery, authoritative
scope resolver, isolated device-access origin/frontend, active authorization-change
containment and runtime lifecycle wiring remain to implement. No live LAN access,
Hub enrollment flag, router setting or production frontend changed in this turn.

Targeted race suites passed for routeros, lanaccess, hub, handler, server and
config; handler/lanaccess/hub/server/config passed again after control-handler
and disconnected-inventory revocation edits. Both Hub UI fixture scripts and
git diff --check passed.
No physical MikroTik verification is claimed. Continue integration locally.

## 2026-10-02 create-only Hub enrollment — LOCAL ONLY

Go EnrollPeer uses POST by-router, never legacy PUT. Older Hub 404/405 and
conflict/unavailable responses fail without fallback. Canonical nonzero public
key validation and response contract version/key/active/private IPv4 /32 checks.
New scripts/hub-enrollment.mjs is an injected synchronous create-only operation:
same-key/port active retries are read-only; different key/port, disabled records,
duplicate tenant/router peers and globally reused WireGuard keys are rejected.
Atomic durable enrolling intent precedes WireGuard sync, read-back verifies exact
public key/address, then active state is persisted. Interrupted setup retries use
the original allocation. Persistence failures do not become an empty peer store.

scripts/hub-create-only-enrollment.patch adds a separate signed POST route with
strict storage reads, atomic fsynced replacement and WireGuard dump read-back.
It intentionally leaves legacy PUT unchanged; do not use that route from the app.
Single managed process/writer assumption must be checked before rollout.
scripts/test-hub-enrollment-route.mjs consumed current Hub source over read-only
SSH and tested the patch in an isolated VM with mocked filesystem/WireGuard and
forbidden listeners/network. Passed signed creation, idempotent retry, key-change
rejection, foreign tenant global key reuse rejection and incorrect scope denial.
Pure operation tests also cover crash retry, corrupt storage and failed read-back.

Local app POST /api/v1/vps/peers/:deviceId/enroll requires OWNER/ADMIN-level role,
server-derived tenant/user/gateway, MAIN_GATEWAY, live plan, bounded request and
explicit confirm_enrollment. New vps.hub_enrollment_enabled defaults false and
requires configured lookup; no production flags changed. Response explicitly
reports router_configuration_required=true and lan_reachability_verified=false.
App tests reject viewers, foreign gateways, downstream devices and absent consent.
Gateway enrollment UI/configuration guidance is implemented locally above;
patch rollout/backup and LAN integration remain pending. No customer peer or
live WireGuard config mutated.

## 2026-10-02 recovery restart containment — LOCAL ONLY

Added RecoveryReconciler and tenant-scoped FenceInterrupted repository. Voucher-
first locks atomically fence stale transferring/frozen rows before returning any
router authority; active completed transfers and other tenants stay untouched.
Blocked rows remain eligible for repeated containment after an offline router
returns, including after expiry. No usage reset, credential issuance, activation,
or extension of the original deadline occurs. Batch capped at 25; failed/uncertain
DB commits return no claims. Network I/O is outside database transactions.

Server.Start now starts a cancellable immediate/30-second worker only under the
existing VoucherAccessEnabled opt-in and live-applier requirement. Tenant IDs are
enumerated in paginated batches; every grant operation uses tenant RLS. Shutdown
cancels and joins before closing dependencies. Restore has an overall one-minute
deadline, additionally bounded by caller/purchase expiry, ahead of the two-minute
stale fence. Blocked purchases still require an audited owner repair workflow;
the worker contains uncertain access, it does not restore it automatically.

Disposable local PostgreSQL TestDB_GuestRecoveryRestartFence passed: persisted
12MB retained, fixed expiry retained, late Complete denied, repeated blocked
containment claims available, recent/foreign/completed states unchanged. Targeted
race suites and full go test ./... passed with MM_TEST_DB_URL set. New code not
deployed, migrations not applied, Recoverer remains nil in server routes.
Hub enrollment still unsafe to expose: live PUT by-router overwrites existing
public keys; needs a create-only/CAS contract. Existing relay targets only the
gateway REST endpoint, not arbitrary LAN services. LAN transport/analytics rollout
and frontend cookie recovery integration remain unfinished; do not call delivered.

## 2026-10-02 transport timeout release — DEPLOYED

Selective release: only internal/provisioner/routeros/transport.go and its test
were uploaded to /home/ubuntu/integration-checks/transport-release.p5aZcn on
mikromagic-app. Staging used the VPS's current source baseline, not bulk local
source. Routeros/provisioner/handler/server race tests passed there. Production
diff was only the 11-line deadline/cancellation addition; secret-bearing error
redaction was already present on the VPS. Guarded original source hash before
copying the two tested files, then used canonical ./deploy.sh api successfully.
No migration, config edit, frontend publish or unfinished-feature activation.

Rollback backup: /home/ubuntu/deploy-backups/transport-20261002.sqaZt4
contains old transport source/test and old server binary. Source SHA256 now
9eb0490e44b70fa6b3fca94b5f5123eb798d8faf833ced1dd9ab8ab00a4bc5f8.
Running /proc/MainPID/exe and deployed server SHA256 both match
c5acdda0b7a50d58ee78a7e7c190f04534e78d0998f28bebbd82b2730bcf7f5a.
Service active; landing/app/health 200; verified HTTPS health status healthy.

## 2026-10-02 ambiguous paid-recovery completion — LOCAL ONLY

Added explicit migration 20261002200000_guest_recovery_completion.sql with
completed_transfer_id receipt; only a frozen-to-active completion may change it.
NULL clearing and arbitrary replacement are refused by the guard. Complete
persists receipt before clearing transfer_id; Block can now fence the exact
active completion after a successful COMMIT whose response was lost, retaining
usage/deadline. Receipt/current MAC/usage/scope matching prevents stale cleanup
from blocking a later completed transfer. Voucher lock order matches Reserve.
No router I/O under DB lock. Startup preflight requires the new column.

Actual RecoveryService with real PostgreSQL repository and fake router simulates
successful Complete followed by a lost reply: returns needs-repair, containment
runs, ledger becomes blocked, cookie lookup/transfer cannot resume, repeated
cleanup is harmless, and 12MB consumption/receipt remain. Existing multi-transfer
DB acceptance now proves old cleanup cannot fence a newer completion. Full local
Go suite with real disposable DB and database/guestaccess/handler/server race
suites pass. DB runner now requires this additional acceptance test. Disposable
container removed; existing services untouched. Migration/source NOT deployed.
This fixes a commit-error fence, not restart reconciliation or counter persistence.
Those, live-session accounting, actual hotspot login/UI and Hub/LAN completion
remain outstanding. Physical MikroTik testing explicitly deferred by user.

## 2026-10-02 paid-cookie resume HTTP and startup checks — LOCAL ONLY

User requested continuing remaining software work while deferring MikroTik tests.
Added GuestAccessResumer and POST guest-resume, using the shared server-anchored
paid-cookie/consent boundary and explicit HTTPS same-origin + CSRF intent guard.
Body-supplied tenant, voucher and secret cannot override authority. Duplicate or
missing proof denies; errors are generic/no-store; no new cookie is minted.
Success returns binding_verified and verified remaining_bytes, explicitly not
an active internet session. No login, MAC transfer, grace or quota reset occurs.

Opted-in voucher constructor now wires credential issuance and read-only resume
with GuestRecoveryRepository and DeviceApplier. Mutating Recoverer remains nil
because uncertain completion/restart reconciliation is unfinished. Default
VoucherAccessEnabled remains false; no production config change or deployment.

Startup preflight now requires both guest tables, forced RLS, required columns,
enabled before-update row guard triggers on the expected public functions, one
named authenticated tenant policy with read/write expressions, and each SELECT,
INSERT and UPDATE privilege independently. This is structural compatibility,
not a cryptographic verification of SQL function/policy contents. No auto-migrate.
Postgres skill informed isolation checks. Real disposable-DB negative tests cover
absent schema, missing forced/enabled RLS, disabled/dropped triggers, absent/extra
policy, dropped column and revoked UPDATE privilege. The latter caught a combined
privilege-list check (any privilege rather than all), corrected before acceptance.

13 resume HTTP cases plus both endpoint origin-guard matrices pass. Full Go suite
passed with local listeners permitted and the disposable local PostgreSQL URL.
Database/guestaccess/routeros/handler/server race suites passed with real DB tests.
Credential-redacting DB runner now also requires schema-readiness acceptance.
Disposable database/container cleanup only; existing local services untouched.
Next: live-session/restart accounting, uncertain-completion reconciliation,
lost-cookie owner repair, actual hotspot-login/front-end integration, and other
unfinished Hub/LAN features. Physical router testing remains explicitly deferred.

## 2026-10-02 Mac-local PostgreSQL acceptance — TEST ONLY

User requested local Mac testing instead of the blocked expanded VPS upload.
Docker Desktop's desktop-linux socket was unavailable; the explicit default
context served the local engine. No Docker context/settings changes were made.
Used the existing official postgres:16 image in a separately named, labelled
container with tmpfs database storage and a random loopback-only published port.
No production credentials/configuration or existing containers were used.

All six targeted real-DB checks passed with no skips: durable voucher claim,
guest recovery (including current-binding lookup and original-code retry fences),
provision orchestration, transition trials, business renewal and authenticated
subscription HTTP. Full database/guestaccess/routeros/handler/server race suites
also passed against that disposable database. The broader run exposed a stale
money-path isolation test expecting administrative active-to-used mutation. Updated
it to assert that redemption is denied with unchanged status/first-seen, while
own-business revocation succeeds and cross-business revocation remains denied.
Production policy was not relaxed. git diff --check passed.

Verified no harness databases remained before container cleanup. Earlier notes
about pending expanded real-DB acceptance are superseded by these local results;
the expanded source bundle was not uploaded to the VPS. Feature wiring, schema
preflight, live-session/restart accounting, uncertain-completion reconciliation
and physical MikroTik acceptance remain unfinished. No deployment this turn.

## 2026-10-02 original-code retry fence — LOCAL ONLY

GuestProvisionRepository.Reserve now rejects every active grant before returning
it to router provisioning, including before any recovery credential row exists.
An original voucher code + MAC is first-use authority only, not returning-guest
ownership. This closes stale original-allowance provisioning during/after transfers
without relying on recovery schema visibility. Returning guests must use the paid
cookie path; lost initial response/cookie requires audited owner repair. Feature
remains disabled, so this policy change is not exposed in production yet.

Expanded real-DB provisioning acceptance to require denied active-code retry with
no router call and unchanged active grant/expiry. Recovery acceptance now exercises
code retries before a transfer, while a transfer is claimed, and after completion
using both original/new MACs; verifies neither provisioning nor containment runs
and transferred current-binding authority remains unchanged. Postgres skill used
for tenant transaction/lock boundaries. New DB assertions remain unverified until
the updated disposable Postgres harness runs; local database tests skip that suite.
Full local Go suite and guestaccess/routeros/handler/server race checks pass.

Prepared /private/tmp/mm-recovery-resume-20261002.tar.gz containing explicit source/
test delta for resume lookup/service, HTTP recovery, read-only RouterOS verification
and retry fence. Attempted upload to mikromagic-app:/home/ubuntu/integration-checks/
mm-recovery-resume-20261002.tar.gz was REJECTED by auto-review: user approval covered
the earlier credential bundle, not this expanded payload. No upload/workaround or
production mutation. Ask approval for this specific new test bundle, then extract
subscription baseline + approved credential delta + this delta into a NEW isolated
directory and run credential-redacting cmd/guest-voucher-db-test with production
config read only for credentials. No production deployment/schema/config change.

Next work: updated Postgres acceptance, schema preflight and safe cookie/resume
wiring, live-session accounting, ambiguous completion/restart reconciliation,
owner lost-cookie repair and physical MikroTik acceptance.

## 2026-10-02 RouterOS current-binding verifier — LOCAL ONLY

DeviceApplier now implements BindingVerifier with read-only commands. Validates
tenant/device/voucher/current MAC/quota/revision, synchronized clock (5s), host and
owned explicitly enabled user, allocated cap = total minus prior transferred
usage, and exactly one current-MAC expiry scheduler. Guard must have an owned
original-voucher or recovery UUID name, exact generated event at original epoch,
10s interval, explicitly enabled state and startup schedule. Original and recovery
event builders extracted without changing generated scripts; existing tests pass.

Only DISCONNECTED bindings can currently be measured. Router-side active-session
checks bracket persistent counter reads and a following read rejects a login race.
Invalid/negative/exhausted counters, changed user, skew or uncertain guard deny
success. No user create/enable/disable, session/cookie removal, reset-counters or
IP bypass operation occurs, even on failure. Active-session accounting is NOT
implemented and is not inferred by adding possibly overlapping counters. This is
a verified eligibility snapshot, not proof the guest's internet session is active.

Stateful fake-router tests cover original/transferred allocations, read-only
commands, foreign user/host/guard, wrong cap, disabled state, clock skew, duplicate/
modified/non-startup guard, active session/login race and invalid/exhausted counters.
Full local Go suite and routeros/guestaccess/handler/server race tests pass;
targeted routeros tests also pass after strict enabled-state checks. Diff check clean.
Official current MikroTik HotSpot manual consulted for active/user interfaces:
https://manual.mikrotik.com/docs/authentication-authorization-accounting/hotspot-captive-portal/
Physical counter persistence after reboot remains UNVERIFIED; this does not complete
restart-safe accounting or automatic reconnect. Newly added DB resume assertions
still need an isolated Postgres run. No VPS upload/deployment or schema change.

Resume HTTP/frontend and recovery schema preflight/credential wiring remain off.
Original-code retry isolation and ambiguous completion/restart reconciliation must
be addressed before enabling; do not bulk deploy the dirty tree.

## 2026-10-02 current-binding resume authority — LOCAL ONLY

Added BindingSnapshot and ResumeService, distinct from MAC transfer/provisioning.
The service validates private recovery proof and same current MAC, bounds work by
the original expiry, asks a read-only BindingVerifier for current-binding usage,
then re-reads ledger authority. Changed tenant/gateway/voucher/MAC/revision/quota/
usage/deadline, cancellation, exhausted data or failed verification deny success.
Remaining bytes subtract both prior transferred usage and verified current-binding
usage. There is no create/enable/reset/contain operation in the verifier seam;
status failure does not mutate a guest's current connection or transfer window.

GuestRecoveryRepository.Lookup implements the tenant-scoped committed read from
active recovery + active grant + used voucher. Constant-time secret hash comparison
and current MAC are mandatory. Original first-use MAC is not connection authority.
Postgres best-practices skill applied: short transactions, no router I/O under DB
locks, tenant isolation and a second lookup after remote I/O.

Pure orchestration tests cover repeated resume without replenishment, post-readback
authority changes, bad proof, foreign initial claim, missing revision, lookup and
router errors, negative/exhausted counters, cancellation and exact expiry. Extended
real-DB recovery test with repeated read-only lookups, wrong credential/tenant/
gateway/MAC denial, old-MAC denial after transfer, cumulative usage and revision
preservation, and blocked-transfer denial. NEW DB assertions have not yet run
against Postgres; local DB tests skip without the harness URL. No SQL schema change.
Full local Go suite and guestaccess/handler/server race checks pass; diff check clean.

Still no production wiring or deployment. BindingVerifier has NO RouterOS
implementation yet: must verify owned current user/host, fixed expiry guard and
allocated quota, and obtain accurate current-binding consumption without counter
reset or stale active-session accounting. Do not expose an endpoint based on ledger
numbers alone. Recovery schema preflight, original-code retry isolation, same-MAC
HTTP/frontend integration, reconciliation and physical acceptance remain open.

## 2026-10-01 cookie-authenticated recovery HTTP boundary — LOCAL ONLY

Added GuestVoucherHandler.Recover and optional Recoverer seam. The request accepts
only a new MAC and hotspot consent; business/gateway resolve server-side and voucher
ownership comes only from __Secure-mm_guest_recovery. Missing, malformed or duplicate
cookies fail closed; JSON secret/voucher/business fields cannot replace the proof.
Portal enabled/active-tenant callback and consent checks precede recovery. Responses
are no-store, hide internal errors/credentials and verify returned tenant/gateway/
voucher/MAC/transfer/deadline/quota before confirming access. allocated_bytes is the
allocation at transfer completion, not a live usage counter; cookie is not renewed.
No grace grant or business entitlement mutation occurs here.

registerGuestVoucherEndpoints conditionally registers guest-recovery only when an
explicit Recoverer exists. Additional same-origin recovery middleware requires an
Origin matching the request host and X-MM-CSRF:1 even without business cookies;
Authorization cannot bypass it. Existing body/rate limits remain on the route.
Production constructor still supplies no Recoverer, so the endpoint remains absent.

Handler tests cover 21 valid/invalid proof, consent, disabled portal, foreign device/
claim, wrong gateway/voucher/MAC, expired/exhausted/negative allowance and uncertain
operation cases. Route tests verify missing/cross-origin/HTTP/invalid-origin/header
requests fail and unwired recovery remains absent. Targeted handler/server tests
pass. Full `go test ./...` and guestaccess/handler/server race checks pass locally.
No database/schema change,
VPS upload or production deployment in this step.

Still required before wiring/enabling: recovery schema/RLS startup preflight, a
current-MAC status/resume path that verifies persistent router counters and never
reapplies original quota, isolation of original-code retries from transferred
bindings, ambiguous-completion reconciliation/owner repair, frontend automatic
retry only for MAC changes, and physical MikroTik acceptance. Do not bulk deploy.

## 2026-10-01 approved recovery credential VPS acceptance — TEST ONLY

User explicitly approved the previously blocked test-only source transfer. Rebuilt
the delta and uploaded to /home/ubuntu/integration-checks/
mm-recovery-credential-20261001.tar.gz, not the production checkout. Extracted the
previous subscription baseline plus this delta into a new isolated directory:
/home/ubuntu/integration-checks/recovery-credential.CaJrfg.

Credential-redacting disposable DB runner passed all six required checks with no
skips: GuestProvisionOrchestration, GuestAccessRecovery (including eight concurrent
issuers, exactly one credential, retry suppression and unverified grant denial),
GuestVoucherDurableClaim, SubscriptionTransitionTrials, SubscriptionBusinessRenewal
and SubscriptionAuthenticatedHTTP. Guestaccess/handler/server race tests also pass
in the isolated VPS directory. Read-only database-name inspection confirmed no
mega_magic_test_* or mega_magic_subhttp_* databases remain. No credentials printed.
Postgres skill applied for isolation and short transactions.

No production source/config/schema/account mutation or deployment. This supersedes
the prior blocked DB validation note only; it does NOT enable automatic recovery.
Issuer remains unwired pending recovery schema preflight, authenticated cookie
recovery/status routes and current-MAC resume with remaining-quota authority.
Physical MikroTik acceptance, reconciliation and the other open features remain.

## 2026-10-01 recovery credential issuance/cookie boundary — LOCAL ONLY

Added optional ProvisionService.Credentials seam, called only after verified
router readback and successful grant finalization. Issuer failure, malformed
credential, cancellation or expiry does not return a secret or confirmed access;
existing containment/failure fencing applies. Stored grant plaintext is cleared.
GuestRecoveryRepository.Issue uses a short tenant-scoped transaction, voucher-first
lock order, authoritative active grant/MAC/quota/expiry checks and one committed
credential. Existing rows yield no plaintext and are never rotated by code/MAC
retry. Any transaction/commit error suppresses the transient secret. Postgres
best-practices skill informed tenant isolation and transaction boundaries.

GuestVoucherGrant.RecoverySecret is excluded from JSON. HTTP redemption checks the
verified MAC and accepts transient credentials only through a Secure, HttpOnly,
host-only __Secure-mm_guest_recovery cookie, scoped to the gateway portal API path
and original paid expiry. Empty retry credentials leave existing cookies untouched;
no credential goes into JSON, browser localStorage or the grace cookie. HTTPS is
required for browser delivery. Lost initial delivery is deliberately not repaired
by reminting from a code/MAC; an audited owner recovery path is still required.

Unit tests cover post-finalization order, failures, retries, cookie security/path/
fixed expiry and JSON exclusion. Full Go suite and guestaccess/handler/server race
checks pass. Added real-DB regression for eight concurrent issuers (exactly one
secret), retries and wrong tenant/gateway/MAC/quota/unfinalized grants; NOT run yet:
local Postgres absent and Docker daemon unavailable. Auto-review rejected copying
the source/test delta to the VPS test area as unapproved sensitive-source egress.
Do not work around that restriction; obtain approval for the specific transfer
before running the isolated VPS DB harness. Local source archive exists at
/private/tmp/mm-recovery-credential-20261001.tar.gz (may need rebuilding after edits).

No production deployment, migration or config change. Optional issuer is NOT wired
in guest_voucher_routes.go yet: recovery schema preflight, current-MAC resume and
remaining-quota authority must be integrated first. Cookie alone does not implement
automatic reconnect. Anonymous recovery/status routes, lost-cookie owner repair,
restart reconciliation, physical MikroTik acceptance, selective subscriptions,
Hub enrollment and LAN services remain open.

## 2026-10-01 guest recovery scope/deadline hardening — LOCAL ONLY

Recovery now rejects inconsistent tenant/gateway/voucher/new-MAC authority and
invalid old bindings before any router or cleanup writes. Missing scope or an
already canceled caller cannot reserve a transfer. Every stop/read, usage freeze,
activation and completion step uses the earlier of the caller deadline and the
original purchase expiry. Cancellation/expiry after a successful storage or router
call is not reported as confirmed access; uncertain work gets separate bounded
router containment and durable failure-fence budgets. Frozen claim corruption
cannot replace the original scoped claim used for containment.

Printed-voucher provisioning now also checks cancellation/expiry after finalization,
including active retries, and cannot start from an already canceled caller. Active
retries ignore stale provisioning leases; unfinished workers remain lease-bound.

Regression coverage includes foreign claims, malformed/multicast/zero old MACs,
same-MAC transfer rejection, missing scope, work deadlines, cancellation/expiry at
stop/freeze/complete, corrupted frozen claims, and voucher finalization races.
Full `go test ./...` and race tests for guestaccess/routeros/handler/server pass.
Database integration suites were not rerun against Postgres this turn; no SQL or
schema changed. No deployment or production mutation.

Still open: recovery credential issuance/delivery via a secure device-scoped cookie,
anonymous recovery/status routes and current-MAC resume without original-quota
reset; restart-safe reconciliation and ambiguous completion repair; physical
MikroTik MAC-rotation/quota/expiry acceptance; selective subscription rollout;
Hub enrollment and LAN services. Do not enable or deploy unfinished guest access.

## 2026-10-01 authenticated subscription acceptance — LOCAL ONLY

Configuration now rejects indefinite grandfather access when subscription-only
mode is enabled; require_payment plus one-time transition trials is the confirmed
policy. Added validation coverage for missing/unknown/grandfather vs require_payment.

Fixed new-registration trial insertion to use WithTenant after system-level signup
writes and before commit. Forced trial RLS no longer depends on the bootstrap
connection bypassing RLS. Any trial insertion error rolls back the signup. Postgres
best-practices skill used for this boundary.

Added TestDB_SubscriptionAuthenticatedHTTP in server package. It creates/drops
its own cryptographically named disposable DB, applies init + maintained auth/
Store/trial migrations, uses actual setupRoutes declarations and real database
sessions/cookies, and tests: registration exact 336h trial; login does not extend
trial; trial allows routers; expired trial returns 402 for routers while account/
checkout/history stay accessible; disabled coin purchase rejected; monthly bank
purchase -> approval -> paid state -> routers restored; duplicate approval 409;
missing CSRF 403; VIEWER approval 403; foreign-tenant approval 404; suspension
revokes the session and returns 401. No live router or external payment call.

Credential-redacting DB runner now requires six tests across database/server,
fails on skips, and all six pass in VPS isolated test directories; disposable DBs
were dropped. Local Go/race and navigation/Store/static UI tests pass. Sidebar and
guide hide coins/unlock keys in subscription-only mode; old ledgers are preserved.
Direct key endpoint is read-only; no key purchase route exists. Coin Store purchase
is disabled by policy; ADMIN-only manual ledger adjustments are bookkeeping, not
an additional subscription checkout, and remain unchanged.

No deployment, production account change or rollout migration. Still need a
selective subscription release plan with backups/config/migration/live verification;
do not bulk deploy the dirty checkout or enable unfinished guest-voucher/recovery.
Guest recovery cookie/automatic current-MAC resume, reconciliation, Hub enrollment
and LAN services remain open. Tests here do not prove physical MikroTik acceptance.

## 2026-10-01 subscription startup/routes/Store — LOCAL ONLY, NOT DEPLOYED

Startup wires registration/router/site/store/topology/VPS/AI SubscriptionOnly
switches together, configures require_payment policy, checks trial + purchase
schema and RLS, and skips the legacy Sovereign quota-expiry sweep in this mode.
Protected tenant routes, separate media owner routes and guest-voucher issuance
owner routes have the shared subscription middleware. Connector/anonymous guest
routes remain separate, and guest expiry/quotas are not disabled by this policy.
Account security/status/checkout/history and operator approval paths remain usable
after expiry with existing auth/suspension/role gates still in force. Missing DB
or status query failure returns generic 503; expired feature access 402; no-store.

New middleware and coordinated-switch tests cover paid/trial/expiry, exact route
allowlist, media/issuance denial, disabled legacy mode, missing DB, query errors
and viewer rejection on approvals. Full local Go and handler/server/subscription
race tests pass. All five disposable VPS DB tests pass including startup preflight;
the harness dropped its DB, no production rows were touched. Postgres skill used
for schema/RLS preflight. Frontend Store status notices, preserved history, hidden
legacy capacity offers, fail-closed checkout, duplicate submit guard and renewal
link added. Store VM behavior test, static UI audit and layout checks pass.

Still need actual authenticated HTTP+DB registration/expiry/renewal acceptance,
review direct coin/key mutation routes and navigation, and safe selective release.
No migration/config toggle/production deployment this turn. The broader guest
reconnect/expiry reconciliation, Hub enrollment and LAN work remain incomplete.

## 2026-10-01 business-wide subscription renewal — LOCAL ONLY

Added `StorePurchaseRepository.DecideSubscriptionTXN`, selected by StoreHandler's
existing SubscriptionOnly switch for both HTTP and Telegram approvals. Tenant
transaction advisory lock is acquired before purchase/account locks, so different
purchasers cannot overwrite the shared renewal deadline. Renewal uses database
time and the greatest of remaining one-time trial and approved monthly/annual
purchase expiry, adding exactly 30 or 365 x 24 hours. Legacy account pass expiry
is not authority. Pending purchase decision, entitlement mirror and audit remain
one transaction; duplicate approvals cannot extend time; foreign purchaser or
entitlement tenant mismatch fails with rollback. No external I/O under the lock.
Used Postgres best-practices skill for short transactions and tenant boundaries.

Added disposable DB regression for concurrent monthly + annual approvals by two
users, preserving the 14-day trial, duplicate/foreign purchase denial, exact expiry,
rejected payment, renewal after expiry ignoring a stale 1000-day legacy pass, and
foreign purchaser rollback. Extended credential-redacting DB runner to require
this fifth test rather than permit skipped database coverage. Full local Go suite
and subscription/handler/guestaccess race tests passed. After the final tenant
guard, database/handler/server/subscription package tests and handler/subscription
race tests passed again. All five disposable VPS DB tests passed, including the
final foreign purchaser rollback case; the harness dropped its throwaway database.

No deployed app, production row, migration or feature toggle changed. Subscription
startup/middleware/UI wiring remains incomplete. Recovery credential delivery and
current-MAC resume are still the next guest integration step, not implemented this
turn. Hub enrollment, live LAN services and physical router acceptance remain open.

## 2026-10-01 explicit guest voucher API/UI — LOCAL INTEGRATION, NOT DEPLOYED

Added GuestVoucherHandler and gated startup routes. `POST /api/v1/guest-vouchers`
is admin/owner/developer-only, authenticated and active-tenant guarded. It requires
an owned MAIN_GATEWAY, label, bounded count/time/data/price, then atomically inserts
codes plus immutable guest allowance snapshots. Codes start unused with no expiry;
elapsed validity starts at first reservation, not printing. Uses the Postgres skill's
short tenant-scoped transaction guidance. No business entitlement mutation.

`POST /api/v1/portal/:deviceId/guest-voucher` resolves tenant from gateway, checks
active business, portal voucher setting/consent and MAC input, then invokes the
provision service. It only reports success for an active matching unexpired verified
grant. Error details are hidden; responses no-store; origin/body/rate guards apply.
Startup requires explicit `guest_wifi.voucher_access_enabled` (default false),
live applier, forced-RLS grant schema, encrypted credentials and allowed target
CIDRs. Existing legacy voucher endpoints are not silently repurposed.

Owner Vouchers UI shows a responsive guest issuance form only when the backend
capability is enabled and role allows editing. Gateway-only options, quota inputs,
first-use explanation and duplicate-submit guard added. Guest portal chooses the
dedicated guest endpoint only from server capability; no failed-request fallback.
It no longer claims verified connectivity for legacy business-pass redemption.
Price column uses price_mmk instead of missing amount. Existing print selection
pagination unchanged. No gradients or media artwork changes.

Full local Go suite and targeted guestaccess/handler/server/RouterOS race tests
pass. Handler tests cover device-anchored tenant, consent, invalid MAC, disabled
voucher login, busy/error/reconciliation responses, batch bounds. New UI behavior
tests, voucher print pagination, layout and static UI audit pass. Database tests in
the ordinary suite skip without the harness env; previous disposable provisioning
test evidence remains valid, but actual HTTP+DB+router end-to-end is not proved.

Still incomplete: durable recovery credential delivery and automatic/current-MAC
resume, session/status integration, revoke/expiry/renewal reconciliation and audited
repair, subscription enforcement/renewal/UI, Hub enrollment and LAN services.
No migration/config toggle, router write or deployment made in this turn. Physical
MikroTik traffic/reboot/private-MAC acceptance is still required before enabling.

## 2026-10-01 provisioning orchestration and transition trials — LOCAL ONLY

User requested all five unfinished feature areas be completed. User subsequently
confirmed existing unsubscribed accounts get a one-time 14-day transition trial;
new registrations still get one 14-day trial. Updated subscription migration to
insert missing trials for existing businesses without current monthly/annual paid
access, preserving existing trials on rerun. Migration/enforcement NOT deployed.

Added guestaccess ProvisionService, tenant-scoped GuestProvisionRepository and
RouterOS GuestProvisionAdapter. Reserve commits before router I/O; router work is
bounded by lease/deadline; finalize rechecks revocation, expiry and worker fencing.
Active retry preserves allowance/expiry and revalidates database authority after
router verification. Uncertain writes contain owned access and durably revoke the
grant rather than release it for a new allowance. Containment verifies disabled
user plus zero active sessions/cookies. Revoked uncertain grants require explicit
audited repair; no automatic retry/renewal is implemented. No arbitrary commands
or new public routes exposed. Existing legacy PublicRedeem is unchanged.

Full Go suite and guestaccess/RouterOS race tests passed. New containment tests
pass. Disposable PostgreSQL tests on VPS passed all four: durable voucher claims,
recovery concurrency, provisioning orchestration and subscription transition trial
duration/rerun/tenant isolation/exact expiry. Initial trial test lacked the Store
schema prerequisite; fixed by applying the maintained Store migration first.
Tests staged in `/home/ubuntu/integration-checks/provision-trial.CCWr6R`, not the
production checkout; harness drops its disposable database. No production account,
router or subscription changes. Postgres best-practices guidance informed short
transactions and tenant-scoped operations.

Next: wire explicit guest issuance/redemption and paid recovery-secret delivery;
integrate current recovered binding into ordinary redemption; finish expiry/renewal
reconciliation and audited repair; wire subscription enforcement/renewal/UI with
business-level purchase serialization; implement Hub enrollment and LAN services.
Physical traffic/restart/private-MAC tests and clock synchronization remain needed.
Do not deploy these partially integrated files or the complete dirty worktree.

## 2026-10-01 interim cookie fence — DEPLOYED AND VERIFIED

Deployed the five-file cookie guard release using the canonical VPS `deploy.sh all`.
All five deployed source SHA256 hashes match the local release. API service is
active; public HTTPS health reports healthy. Deployment checks returned HTTP 200
for landing, app and health. No database migration was required.

Disposable database-backed handler verification passed: initial grant, secure
cookie, same-MAC resume preserving request/expiry and one router grant, changed-MAC
and forged-cookie denial, six-hour cooldown, eligible retry and approved-request
denial. The test used a fake router and dropped its disposable database; it did
not change production guest rows or real router access. Fixed first-response
expiry precision to use the committed database timestamp. Full local Go suite,
targeted race checks and deployed targeted handler tests passed.

Owner-facing explanation is under Captive portal configuration, below Grace
time/data settings. Recognition is an additional fence only: cookie deletion or
another browser can evade it. Changed-MAC requests are blocked, not automatically
reconnected. Real iPhone/captive-browser persistence and gateway accounting still
need physical-network acceptance. Banking-only access remains disabled.

Rollback backup: `/home/ubuntu/deploy-backups/guest-cookie-20261001.cTsaBM` contains
previous source, server binary and installed web assets. The two new files
`guest_grace_cookie.go` and `guest_grace_cookie_test.go` were absent before this
release and must also be moved aside if rolling back. Unfinished MAC recovery,
router adapter changes and unrelated local work were NOT deployed.

## 2026-10-01 interim cookie fence — LOCAL IMPLEMENTATION, NOT DEPLOYED

Added `internal/handler/guest_grace_cookie.go` and tests; server initialization
derives a dedicated-purpose HMAC key from the configured JWT secret (cookies are
not JWTs). Successful guest purchase/resume writes a Secure, HttpOnly, SameSite=Lax,
host-only `__Secure-mm_guest_grace` cookie scoped to that gateway's portal API
path, with 30-day recognition lifetime. This lifetime does NOT extend grace or
change the existing six-hour cooldown. Requires HTTPS and the same-origin portal;
plain HTTP/captive browser cookie loss are known limitations.

Cookie payload binds business/device/request and timestamps; signature, scope,
expiry and maximum lifetime checked. Invalid/stale-to-missing-record cookies deny
fresh grace, not fall back silently. Existing original request is loaded under
tenant scope and row lock, after the current MAC advisory lock, before current
MAC-based grant logic. An active cookie-recognized request with changed MAC is
denied with an owner-reconnect message; expired requests keep the six-hour rule.
An approved original request cannot obtain fresh free grace from this path.
After six hours a failed request may retry under the normal current-MAC policy.
Cookie issued only after successful transaction commit. No schema changes.

Automatic MAC rebinding is intentionally NOT wired: it must stop old access and
checkpoint remaining usage before enabling another MAC. This fence blocks new
grace for a recognized changed MAC; it does not itself restore connectivity.
Clearing cookies, another browser/captive cookie store, another gateway or
network-blocked cookie storage remains outside this guard's proof of identity.

Signing/tampering, tenant/device isolation, expiry, secure headers, missing vs
forged cookies, live/expired/six-hour MAC-change policy and unchanged allowance
unit tests pass. Full suite and targeted race regression checks run separately;
physical Safari/captive-browser acceptance and full DB-backed handler/gateway
flow remain unverified. Live VPS/portal unchanged.

## 2026-10-01 interim cookie fence — USER ACCEPTED LIMITATION

User accepts cookie-based recognition as a practical additional abuse fence, not
100% protection or verified identity. Interim direction: retain current grace
time/data limits and six-hour failed-payment cooldown; use a secure server-issued
cookie to identify the existing server-side request across MAC changes while the
cookie is present. Never replenish quota or restart expiry on reconnect. A MAC
change must not simply re-run ConfigureGuestGrace with a new full allowance:
safe stop/accounting/rebinding is required, or deny the transfer without granting
fresh grace. Cookie deletion/alternate browser remains a documented limitation.
Banking-only replacement remains deferred to physical-network acceptance.
This entry records accepted policy only; no cookie issuance/validation or live
router behavior was changed in this turn.

## 2026-10-01 banking-only access — DEFERRED PHYSICAL NETWORK TEST

User explicitly deferred Myanmar banking endpoint discovery and banking-only
walled-garden acceptance until a physical MikroTik Wi-Fi network is available.
iPhone Mirroring on ordinary internet cannot verify gateway enforcement. Later:
obtain official endpoint information or consented destination/DNS observation;
test real iPhone/Android payment flow with mobile data off; verify unrelated
traffic, VPN, alternate DNS, IPv6 and MAC-rotation/reconnect remain blocked;
verify pending/approved/expired payment transitions and unchanged purchased quota.
Never capture banking passwords/OTPs or decrypt banking TLS. Do not deploy broad
bank/cloud/CDN allowlists based on guessed endpoints. This is deferred work,
not an enabled production feature or scheduled automation.

User is considering current time/data grace plus authentication cookies as an
interim approach; no implementation change authorized by that question alone.
Cookies can resume an existing server-side allowance while present, but are NOT
verified identity and cannot reliably prevent fresh grace after cookie deletion,
a separate browser/captive-portal cookie store, and a changed MAC. MikroTik MAC
cookies are MAC-bound. Keep the six-hour rule unless general-internet grace is
actually replaced; do not claim the cookie approach is rotation-proof.

## 2026-10-01 CHR read-only compatibility checks — NOT DEPLOYED

Added `cmd/guest-router-check` (explicit device UUID required; refuses device names
not prefixed `CHR lab router`). Built isolated Linux diagnostic and ran from
`/home/ubuntu/integration-checks/router-readonly.ettHuF/guest-router-check` using
existing encrypted credentials/pinned host key. No application deployment,
database mutation, router configuration or guest access mutation performed.

Real CHR rejected `print stats value-list`. Replaced adapter statistics retrieval
with explicit owned-user `get bytes-in`/`get bytes-out` and verified counter-get
syntax on an existing lab user. Active/cookie count commands verified. These
checks prove syntax, NOT final accounting correctness or recovery acceptance.
Lab epoch clock is consistently approximately 11 seconds behind the app server:
app server `NTPSynchronized=yes`; CHR NTP `enabled=false synchronized=false`.
Recovery clock check correctly fails closed. NTP configuration remains unchanged;
synchronization must be addressed before traffic/expiry acceptance tests.

SSHTransport now bounds established connection I/O (handshake and command) with
the earliest context/config timeout and closes on cancellation. Loopback stalled-
handshake timeout/cancellation regression tests added. Recovery database Block
gets a fresh budget after containment, not a possibly exhausted cleanup context.
Full Go suite passed after transport/counter changes; targeted race tests passed.
Verified identity provider/cooldown, public integration, restart accounting and
repair/audit workflow are still required before deployment.

## 2026-10-01 private-MAC recovery — LOCAL ROUTER ADAPTER, NOT LIVE

Added `internal/provisioner/routeros/guest_recovery.go` implementing the recovery
router interface on DeviceApplier. It checks owned user, gateway/server and quota,
disables the original binding, removes its active session and cookies, verifies
both gone, then reads final counters. Replacement creation is disabled-first,
uses only ledger-remaining bytes and the unchanged epoch deadline, and verifies
a per-transfer expiry guard and quota before enabling. Original sessions/cookies
are checked again before and after activation; clock skew/rollback is checked
again at activation boundaries. Foreign users are never overwritten or contained.
Service-level failure handling contains owned bindings and blocks reconciliation.

Stateful fake-router tests pass for stop-before-measure ordering, unchanged
quota/deadline, stale session/cookie rejection, clock rollback, bad guard/quota/
activation readback and containment. These are NOT real RouterOS acceptance tests.
Remaining gates: verify CLI/counter behavior on CHR (including restart continuity),
profile hook hardening, durable usage reconciliation and audited repair, purchase
issuance/recovery credential delivery, public session routes, current-binding
ordinary redemption, and verified-identity OTP/cooldown. No migration, public
endpoint or adapter deployment has occurred. Production six-hour bank cooldown
remains MAC-based and can still be evaded by changing MAC.

## 2026-10-01 private-MAC recovery — FIRST LOCAL IMPLEMENTATION, NOT LIVE

User raised iOS rotation locking out paid guests and bypassing MAC-only grace
cooldowns, then requested a first implementation. Added `internal/guestaccess`
recovery orchestration and `GuestRecoveryRepository`, plus migration
`20261001210000_guest_access_recovery.sql` (requires the guest grant migration).
Secure 256-bit recovery keys are hashed at rest and issued only against a live
active/used guest voucher. Current connection is separate from immutable first
MAC. Recovery serializes one transfer, preserves the original deadline, stops
and verifies old access before checkpointing consumed data and enabling the new
binding with ONLY remaining bytes. Failed/uncertain transfers are contained and
blocked pending reconciliation. At this checkpoint router operations were only
an interface contract; the newer local adapter is described above. No public recovery route or token
issuance is wired into the still-legacy PublicRedeem endpoint.

Recovery rate limit: three transfers per six-hour window per purchase, tested;
this is separate from the failed-bank-transfer six-hour grace restriction.
Database guards prohibit lowering consumption, replacing ownership/credential,
or switching current MAC without a frozen transfer. First-use MAC is untouched.
Wrong token/tenant/gateway denied; eight concurrent recovery requests give one
winner; stale/double checkpoint or finalization denied. Multiple rotations carry
50 MB cumulative consumption on a 75 MB purchase; resetting the recovery rate
window does not reset usage or expiry. Tests passed in the disposable VPS harness
at the existing isolated integration-checks path, with no live migration/data
or application source changes. Full Go suite, recovery race tests passed.

`IdentityProof.EligibleForGrace` tests enforce server-verified subject/tenant and
existing retry deadline independent of MAC. This is policy ONLY: no guest SMS
provider, OTP challenge flow, identity sessions or durable identity cooldown
ledger is configured. Existing router-administration TOTP is NOT guest phone
verification. Production grace remains MAC-based and is NOT rotation-proof.

Clearly labeled interactive DESIGN PREVIEW (simulation, no network mutation):
`/private/tmp/mm-guest-recovery-preview-20261001/index.html`, localhost port 8769,
Python preview server session 56278; in-app browser tab 23 kept as deliverable.
UI click verified the simulated unchanged 35 MB / 10:30 AM result. Screenshots
in that directory. The preview is deliberately outside deployable web assets.

Next gates: implement verified RouterOS stop/read/cookie revocation and remaining-
quota adapter, audit trail/repair workflow, purchase issuance/HTTPS recovery
credential delivery and public session-token routes with bounded attempts, fix
ordinary guest redemption to use current binding after recovery, persistent
verified-identity cooldown + actual OTP provider, CHR traffic/rotation/expiry
acceptance and only then controlled migration/deploy. No commit or deployment.

## 2026-10-01 printed guest voucher foundation — LOCAL ONLY

Continuing after the commerce deployment, added a separate guest-internet grant
model/repository and `20261001200000_guest_voucher_grants.sql`. No legacy voucher
backfill: a business/WebFig pass must not silently become guest WiFi. Allowance
snapshots are immutable, references tenant-scoped, first claim bound to gateway
and MAC, expiry fixed at first reservation. Short transactions reserve a fenced
worker lease before router I/O; stale workers cannot finalize or release a new
lease. A confirmed failed worker can release only its own lease so a one-minute
voucher need not wait for the two-minute crash-recovery lease. It never unclaims
the voucher, changes MAC or restarts validity.

Real PostgreSQL concurrency/tenant tests passed in a disposable database via
`cmd/guest-voucher-db-test`, staged at
`/home/ubuntu/integration-checks/guest-voucher.g8SYMw`. Eight concurrent claims:
one winner, seven busy; foreign tenant/gateway/MAC denied, retry preserves expiry,
stale lease release/finalization denied, snapshot mutation blocked. Harness drops
its temporary database. Production schema/data/source were NOT changed.

New RouterOS `guest_voucher.go` helper installs and reads back a scoped recurring
epoch-deadline guard before creating a disabled MAC/server-bound quota user,
verifies time again before enabling, and contains uncertain activation with an
owned-user-only disable/disconnect attempt. Active retries recheck expiry and do
not reset counters. Existing disabled users are deliberately not re-enabled;
failed creation/readback may require owner repair. Unit and race tests cover
ordering, fixed deadline, idempotence, clock skew, missing MAC login, foreign or
disabled users, bad guard/quota/activation readback and expiry during setup.

NOT wired into voucher issuance or PublicRedeem yet. No new deployment or commit.
PublicRedeem STILL invokes the legacy business-pass redemption core. Remaining
gates: explicit guest issuance + allowance UI, reserve/commit/router/readback/
finalize/session orchestration with revoke-race containment, production migration
preflight, actual CHR timestamp/scheduler/reboot/traffic acceptance, paid renewal
and reconciliation. Scheduler granularity is ten seconds; router clock skew is
currently allowed up to thirty seconds, so exact wall-clock expiry is NOT proved.
Do not claim printed vouchers grant guest internet in the deployed application.

## 2026-10-01 scoped commerce deployment — user explicitly authorized

After the incomplete-status checkpoint, the user said "You can deploy now".
Published the tested subset through VPS `./deploy.sh all`: stronger mutation
roles, six-hour failed-grace cooldown, input/consent/upload guards, voucher
status guards, RouterOS error redaction, full-ledger revenue and print selection
pagination. Did NOT publish `guest_expiry.go` or the paid-expiry changes in
`guest_grace.go`; restart-safe elapsed validity remains unfinished. No unrelated
subscription/LAN/traffic-flow work, database migration or router configuration
was included. No new local commit was made because the local dirty tree contains
interleaved earlier work outside this release.

Backup: `/home/ubuntu/deploy-backups/commerce-hardening-20261001.Hve2yk/`
contains existing target source (`source.tar`), old binary (`server`), installed
frontend (`installed-web.tar`), and the older cookie test. Canonical `server.prev`
also retains the previous binary. Release targets: 22 source/test/script files,
all SHA-256 verified equal local/VPS. Initial VPS test run stopped on an older
cookie fixture that did not actually set a cookie; synchronized the locally
passing test with a real cookie plus a separate cookie-free API-client test.
All five focused VPS package suites then passed. Config and SSH/capacity/store/
auth schema checks passed. Service active; landing/app/portal/health HTTP 200;
anonymous revenue HTTP 401; oversized public portal request HTTP 413.

Still NOT production-certified: printed voucher internet provisioning, paid
renewal/elapsed restart handling, receipt bearer-token anchoring, router/DB
failure reconciliation, branded upload/walled-garden/bypass protections and the
live one-minute grace test. Approved clients fail closed on new automatic grace
requests; renewal currently requires the owner. Database integration suites
remain skipped without an isolated harness DSN. Do not describe this release as
completion of the full security workflow.

## 2026-10-01 guest-commerce hardening — LOCAL, NOT RELEASED

User authorized fixes for printed vouchers AND mobile banking, a six-hour failed
payment retry cooldown, complete revenue, print selections of 1–100 with pages,
security hardening, then commit/deploy only after successful acceptance.

Local implementation now includes explicit OPERATOR guards for voucher create/
status and guest decisions, ADMIN for portal edits/voucher deletion, irreversible
voucher status guards, six-hour cooldown after the failed grace deadline,
consent checks, bounded public rate/body inputs, stricter QR URLs, phone/last-six
validation, honest receipt-save errors and atomic pending/deadline submission,
CSV formula escaping, redacted RouterOS transport errors, paginated print
selection, and a separate full-ledger guest revenue API. Receipt photos use
unique private paths so a racing upload cannot overwrite an accepted receipt.
Failed database submissions can leave unreferenced private photos; retention
cleanup remains to be designed. Receipt request IDs still lack scoped bearer
tokens, so this is NOT a complete anonymous-payment security fix.

New paid expiry scheduler is only a local first iteration. Never renews a retry
deadline and uses calendar arithmetic rather than string date comparisons.
It still needs persisted plan/deadline snapshots, restart/offline reconciliation
and real RouterOS readback/traffic acceptance. Approved clients currently fail
closed on re-purchase to protect paid users from failed-grace cleanup; proper
paid renewal must be implemented before release.

Verification: full `go test ./...` passes with localhost test access enabled;
database integration tests still skip without their DSN. Node voucher print
pagination and network workspace regressions pass; inline app/portal scripts
parse; `git diff --check` passes. VPS PostgreSQL read-only revenue query succeeds.
`scripts/test-guest-revenue-sql.mjs` verifies synthetic 155 requests, including
125 approved sales = 12,500, with pending/rejected excluded. No rows inserted,
no migrations applied, no router writes, no commit or deployment this phase.

RELEASE BLOCKERS: printed PublicRedeem still calls the old business WebFig-pass
redeemer instead of provisioning guest internet; initial guest elapsed validity
and quota/device snapshot semantics incomplete; branded portal upload/walled
garden/IPv6/FastTrack/bypass hardening not accepted; authenticated request-token
anchoring and router/DB failure reconciliation incomplete; live one-minute bank
transfer grace + six-hour retry acceptance not run; paid restart expiry not
proved. Do not deploy the whole dirty tree or claim all progress completed.

## 2026-10-01 gateway / portal / guest-commerce security audit

Read [WORKFLOW-SECURITY-AUDIT-20261001.md](WORKFLOW-SECURITY-AUDIT-20261001.md)
before treating earlier workflow-completion notes or historical voucher scorecards
as production acceptance. This was a read-only audit, not a repair/deployment.
Key findings: viewer-accessible guest approval/portal edits/voucher mutations;
printed voucher redemption grants a business WebFig entitlement, not RouterOS
guest internet; maintained branded portal upload/walled-garden deployment is
missing; revenue silently uses only 50 guest requests and mixes legacy business
entitlements with hotspot sales. Paid MMQR time is connected uptime, not elapsed
wall-clock validity. Router quota helpers have passing unit tests, but actual
guest traffic enforcement, bypass testing and PostgreSQL concurrency/RLS checks
were not certified by this audit. Several key deployed source hashes match;
the installed app HTML differs overall, although its revenue code was checked.
Focused Go tests, RouterOS/voucher/Hub race checks and Hub/network UI regressions
passed. No production or router changes were made. Preserve separate
implemented → tested → migrated → deployed → verified evidence per workflow.

## 2026-10-01 confirmed account deletion

Owner explicitly confirmed permanent deletion of eight **unsubscribed** accounts
after the expiry audit found no recorded expired subscriptions: Leopard, magic,
mgkyaw, Min, Phyo, PyaeSone, Thet Aung and venusvenus. These are not an inferred
expiry cohort. The fixed-ID cleanup was applied to production in one guarded
transaction and independently re-audited: all eight users are absent; kyaw,
both Nish accounts, Developer, Itsmenish and Zay remain (six user records).
Four auth sessions and eight zero-balance wallets cascaded; historical records
were retained with nullable actor references cleared. Businesses and all
business/device configurations were deliberately preserved, not deleted.
There is no application-level undo; no recovery from database backups was tested.
Do not automatically purge orphan business records or expand the ID allowlist.

The user additionally authorized deploying completed fixes. Compared live/local
hashes of the app, shared layout, media appearance/dashboard/overview, site map,
Magic Dude prompt/Gemini client, media handler and Hub client: all matched.
These completed source slices are already published; no bulk dirty-tree release
was made. Subscription-only and LAN/Traffic Flow scaffolding remain incomplete.

**Additional completed auth fixes deployed:** the live diff lacked the local
positive JWT lifetime validation, account-state lock/check before session
insertion, refresh-token identity validation, and cookie-client refresh/state
handling. Published only `internal/auth/service.go`, `sessions.go`, and
`web/assets/js/api.js` plus their tests via canonical API/web deployment.
Local and VPS auth race checks and four browser-client tests pass; production
schema/config preflight and landing/app/health return success. Disposable
PostgreSQL session lifecycle tests remain skipped, not verified. No migration
was applied and no subscription switches were enabled. Rollback:
`/home/ubuntu/deploy-backups/auth-fixes-20261001.i3usbREd/`.

## 2026-10-01 Magic Dude behavior and scrolling

Implemented → tested → migrated (not needed) → deployed → live checked.
The assistant prompt defaults to natural Burmese, 1–3 short sentences, a playful
but respectful tone, and role-filtered directions using actual sidebar/tab names.
Read-only, secret-handling and explicit-change-approval rules remain intact.
Live testing exposed English-only and mixed-script provider output. A server-side
language safeguard substitutes a Burmese retry notice for those outputs; it does
not certify fluency or correctness. Common library/gateway/site/Telegram location
questions use vetted, role-aware Burmese directions. General tone and brevity
remain model instructions rather than guaranteed linguistic quality.
The popup now localizes welcome/status/errors and follows new messages after
layout, content resize and reopening. It prevents duplicate in-flight submissions,
does not send on IME composition Enter, and avoids page-scroll/focus stealing.

Go handler/provider tests, focused Magic Dude race checks, the new browser-client
regression script and static UI/layout checks pass. No new migration is needed.
Only the prompt helper, prompt call and chat UI were published; production retains
its existing access-control code, not the unfinished local subscription switch.
API/web deployment preflight and health checks pass. Rollback:
`/home/ubuntu/deploy-backups/magic-dude-20261001.xJHdB6w5/`.
Live English library-location question returned the short vetted Burmese answer.
The overflowing conversation ended within 1px of the bottom after a new reply
and after closing/reopening. Proof: `/private/tmp/magic-dude-burmese-20261001.jpg`.

Next unfinished work remains subscription-only rollout (existing unpaid-account
policy unanswered), Magic Media integration acceptance and safe Hub enrollment,
LAN-access/Traffic Flow integrations. Their existing handoff entries remain the
source of phase-specific evidence; do not infer completion from code scaffolding.

## 2026-10-01 shared frame alignment

Implemented → tested → deployed → live checked (no database migration needed).
`web/assets/css/layout.css` defines shared section gaps, panel insets, wrapping
action rows, aligned checkbox labels, responsive record forms, and bounded
table/dialog frames. Connection creation now belongs to its panel header;
gateway/device actions have their own spaced row. Media artwork is unchanged;
no new gradients were introduced.

Frontend only published with `deploy.sh web`. Rollback copy:
`/home/ubuntu/deploy-backups/layout-20261001.W2dnwP/`.
Verified loaded operational/business/media/admin screens at desktop width;
gateway form at 1440px uses two columns. Eight representative complex screens
at 390px (gateway, downstream, Sites, media, Store, portal, topology, profile)
showed no document-level horizontal overflow; mobile record form is one column.
Static UI audit and layout/network/Hub/media/map regression scripts passed.
This is frame containment verification, not exhaustive acceptance of every
hidden dialog or feature function. No records, payments or routers were changed.

Subscription-only policy remains unfinished and NOT deployed/migrated/enabled.
See `docs/SUBSCRIPTION-ONLY-ROLLOUT.md`. The user confirmed monthly/annual payment
after a new business's 14-day trial. Existing unpaid business policy is still
unanswered. Local backend foundation tests pass; PostgreSQL integration and
production rollout remain unverified. Do not deploy the entire dirty worktree.

**Updated:** 2026-10-01 (Asia/Yangon).

**Project:** `/Users/aces/Magic Development/mega-magic-project`.

**Maintained code:** `cmd/`, `internal/`, `pkg/`, `web/`, current migrations,
validation scripts and `deploy.sh`; see the [README](../README.md).

This is a dated delivery record. Check the current tree and deployed revision
before assuming it still describes live state. These documentation edits do not
constitute a new production rollout.

## Intent and current phase

### Owner-requested Gateway features (2026-10-01)

**Hub inspection correction (read-only, 2026-10-01):** `mikromagic-hub` resolves
to host `magical-hub`. The live service `mikromagic-hub-peer` runs
`/opt/mikromagic-hub/service.mjs` on loopback `127.0.0.1:8787`. WireGuard `wg0`
uses `10.77.0.1/24`; one peer had a handshake about 52 seconds old during this
inspection (others were old or never handshaken). IPv4 forwarding is enabled.
The Node service already implements signed peer provisioning/inspection,
disable/remove operations, downstream REST relays with certificate fingerprint
checks, and WebFig launch/session authorization. HTTPS at
`https://hub.mikromagic.com/internal/provisioner/v1/peers/test-peer` validated
without TLS bypass and rejected the unsigned request with 401.

Do not infer that Hub infrastructure is absent from the Go app's helper-only
VPS package. No equivalent HMAC Hub client was found in maintained `internal/`
or `cmd/`; integrate the existing service rather than creating a second control
plane. Its HMAC contract signs method/path/timestamp/requestId/scope/tenantId/
routerId/requestedBy/raw body. Inspect replay rejection before adding access
grants: the reviewed authorization function checks signature and time skew but
does not itself deduplicate request IDs. No general LAN-session or Traffic Flow
collector route was observed. A fresh peer is not proof of a particular tenant's
LAN reachability or completed app onboarding.

No Hub keys/environment files were read, no peers/routes/firewall/services were
changed. Do not run `/opt/mikromagic-hub/hmac-selftest.mjs` as a read-only check:
its deployment note explicitly says it provisions peers in the live store.

**Go adapter update (local only):** `internal/hub` now implements the existing
signed `peers:inspect` GET and `peers:create` PUT contracts. Requests use verified
HTTPS, bounded timeout, unique cryptographic request IDs, exact canonical HMAC
fields and a 32-byte hex signing key. Redirects are refused; responses are size
bounded and tenant/router/peer identity checked. Enrollment rejects invalid and
all-zero WireGuard public keys. Errors do not echo upstream body/signatures or
key material. A fixed Node-generated signature vector, mock contract/security
tests, race checks and Go vet pass. Initial testing used no real credential;
see the deployment/credential update below. No enrollment has been made.

**Read-only lookup integration:** Config now supports an explicit,
default-off `vps.hub_lookup_enabled` flag and protected environment credentials.
Signing keys are excluded from JSON. `/api/v1/vps/peers/:deviceId/live` checks
admin-level role, active pass and tenant-owned MAIN_GATEWAY before using signed
router lookup; it reports actual allocation and handshake freshness, not LAN
readiness. Config, client, handler and server tests pass. **Deployment update:** the Hub GET extension passed nine
isolated contract cases and five signed live checks and is now deployed; peer
store/WireGuard allowed-IP hashes are unchanged. The app API is deployed after
production config/schema checks and server-side tests; health passed. No migration
required. **Credential update:** after the initial approval-review block, the
owner explicitly authorized storing the Hub signer on the app. It was transferred
directly over encrypted SSH pipes to root:root 0600
`/etc/mega-magic-hub-lookup.env`; systemd loads it and lookup is enabled. The Go
client on the app passed signed HTTPS inspection of an existing peer, exact
identity matching and cross-tenant denial. Public health 200; anonymous endpoint
401. No customer peers/router settings changed. Existing signer permits mutating
peer scopes too; approval does not authorize invoking those operations.
[Rollout status and backups](HUB-LOOKUP-ROLLOUT.md).
**Gateway UI update:** read-only Check Magic Hub button is implemented, UI-tested
and deployed for gateway management roles, not viewers/downstream devices. The
signed-in Nish/DEVELOPER browser request hit the existing active-Sovereign-pass
gate and showed its explanatory message. This denied case is verified; actual
lookup success through the app-user UI is not. No plan/permissions/router changes.
Frontend backup `/home/ubuntu/deploy-backups/hub-ui-20261001.D4gfJ5lx/`.
Next: owner direction on whether read-only Hub status remains paid-pass gated,
then successful authenticated UI lookup. Never silently bypass billing gates.
Operator CLI transport verification is not browser acceptance success.
The complete LAN features are still not delivered.
Enrollment still needs preview/approval/audit and replay/idempotency protection.
Do not expose the signing key, caller-selected scope or raw Hub proxy in the UI.
Enrollment changes live peers; creating this client does not authorize invoking
it against arbitrary customer routers. Enrollment remains unwired/unverified.

The owner supplied LAN traffic/apps and temporary LAN access references and
requested equivalent functionality in Mega Magic. Backend groundwork is local;
the complete features are not implemented or deployed. Place them inside the
selected Gateway workspace; do not add unrelated global sidebar tabs.

**LAN traffic & apps:** 24-hour and 7-day views; upload/download volume and share
by client; IP/MAC/hostname where observed; estimated services/destinations and
top services per client; client filters linked to topology. Include Unknown,
classification confidence, last collection time and collection coverage. No
HTTPS payload capture or decryption. Shared CDN addresses do not prove which app
was used. Missing data is not zero usage. Owner/admin explicitly enables or
disables collection; seven-day retention and deletion must be enforced server-side.
Traffic Flow/IPFIX collector, tenant-bound exporter identity, aggregation and
classification are not present in the maintained code. Flow totals require
direction/deduplication tests and explicit treatment of sampling, NAT and IP reuse.
RouterOS CPU-visible Traffic Flow excludes hardware-offloaded traffic; do not
disable hardware offload or FastTrack automatically to improve statistics.
Source: https://help.mikrotik.com/docs/spaces/ROS/pages/21102653/Traffic+Flow .

**Temporary LAN access:** select a detected LAN/VLAN on the chosen gateway, one
target IP, one port and a bounded duration; display active sessions, countdown,
close/revoke and an audit history. Web services can have an authenticated Open
device action; arbitrary TCP tools need a separate authenticated client path,
not an iframe claiming to support them. Keep upstream device authentication.
Default deny: owner/admin creation only, viewer cannot open access; delegated
technician access requires an explicit owner grant. Authorize business, gateway,
LAN and target at creation and on every connection. No public unauthenticated
URLs, unrestricted destinations, cross-tenant routing, platform management or
metadata endpoints. Revalidate redirects/connection destinations and isolate
untrusted device web content from the main app origin. Expiry/revocation must
close existing connections and survive worker restart, not just hide a browser
button. Overlapping LAN ranges across businesses must remain isolated.
The current VPS helpers shape peer config and health responses; they do not
establish a tunnel or implement this access-session lifecycle.

**Local implementation update:** `internal/lanaccess` has a pure authorization
policy for one private IPv4 target/port, maximum one-hour grants, verified gateway
scope, mandatory protected-network inventory, owner/admin/developer roles and
per-connection revalidation. It denies other tenants/gateways/users, changed
roles, revoked/expired grants, target/port changes and platform management targets.
It is NOT an access-session repository, proxy, stream revocation worker or API.
Callers must resolve inventory and protected networks server-side, enforce
authenticated scope and reload authoritative grant state; never accept Scope or
Grant from request JSON as authority.

`internal/trafficflow` decodes bounded NetFlow v5 IPv4 metadata, validates lengths,
record counts/timestamps/sampling and handles uptime rollover. Sampling is kept
explicit; no automatic extrapolation or invented app classification. This is NOT
a collector, exporter-authentication mechanism, aggregation service or IPFIX/v9
decoder. Raw UDP identity is not tenant authentication. Both packages passed
focused race tests; decoder fuzzing passed 434,768 executions in three seconds.
The complete feature's migration, deployment and live verification remain pending.

**Delivery:** partial backend helpers implemented/tested locally only. No router,
firewall, Hub or production changes made for this request. Next prerequisite
remains verified Magic Hub
enrollment and tenant/device/site bindings. Then build collection/access services
and their Gateway UI, test isolation/expiry/revocation and real telemetry on an
explicitly authorized lab gateway before production rollout. Enabling export,
changing network paths or granting live LAN access needs target-specific owner
authorization; these reference images do not authorize such changes.

### Network workspace audit fixes (2026-10-01)

- **Implemented:** Sites create/edit uses the existing main map with a draggable
  draft pin, cancel/clear controls and no second map. Duplicate site/device
  tables and duplicate Add router control removed. Device actions follow role
  and deployment layer; downstream add/edit controls no longer stop on a missing
  button. New device categories match their deployment layer. Old connection
  metadata is not presented as working Magic Hub enrollment/authentication.
  Online connection badges require a timestamp within five minutes. Gateway
  preview responses are invalidated by intervening input changes; hidden controls
  stay hidden despite button display styles.
- **Tested:** map and network workspace regression scripts, navigation role tests,
  static UI audit and diff check pass. Local mock browser checked single-map
  pinning/cancel, downstream add/edit, gateway dialog and initially hidden Apply;
  no browser console errors observed. No live router writes were used as tests.
- **Migrated:** not applicable; frontend-only changes.
- **Deployed:** frontend-only canonical `./deploy.sh web` completed. Public map
  JS version 3 and redesign CSS `a68e2d6d` byte-match source; served app contains
  the new map import and preview invalidation. Landing/app/health return 200 and
  API MainPID is unchanged. Existing media dashboard version 6 is preserved.
  Rollback: `/home/ubuntu/deploy-backups/network-ui-20261001.jEkZRv/`.
- **Verified:** local mock UI, regressions and public release assets/health.
  Real authenticated production workflows and Magic Hub enrollment are not
  verified by these checks.

**Next network work:** implement secure Magic Hub onboarding and device/site/parent
gateway/connector bindings through the backend, with tenant isolation, protected
credentials, strict host-key verification and end-to-end connection tests. A saved
record, transport dropdown or cloud badge does not prove router authentication.
Full downstream configuration is not implemented by this frontend patch. Do not
alter live router/firewall/Hub networking without target-specific authorization.

### Guest tile editor and design presets (2026-10-01)

**Production rollout update:** the editable tile/template follow-up and the
gradient-free revision are now deployed via frontend-only `./deploy.sh web`.
This supersedes the local/pending deployment notes below. Appearance JS/CSS 5
and dashboard JS 6 were byte-compared against their fresh public CDN URLs;
app/landing/health return 200 and www-data can read the assets. The API MainPID
was unchanged: no API restart, schema migration or Jellyfin settings/library
changes. Production authenticated editor acceptance is not yet verified.
Protected rollback snapshot:
`/home/ubuntu/deploy-backups/media-design-20261001.YJtjrM/`.
The initial attempt safely rolled back because `curl | rg -q` exited early;
the corrected check downloads the full HTML before inspecting it.
Browser drafts still do not publish a playable guest page or modify Jellyfin's
native web client. Real catalog/artwork, publishing and paid playback remain
unfinished integration steps.

**Design direction, revised by owner:** no gradients, AI-looking decorative
artwork, neon treatment, or forced matching to Mega Magic's green console theme.
Use solid surfaces, professional typography/layout and owner-supplied artwork.
Guest templates now are **Cinema, Editorial and Screening room**; the Forest/
Neon presets below describe the superseded initial implementation. Uploaded
pictures have no colour filters or overlays, with captions outside the image.
Legacy palette drafts map to neutral palettes without dropping uploaded pictures.
Current deployed asset versions:
appearance JS/CSS 5, dashboard JS 6. All media tests, static frontend audit and
diff check pass. Local browser verified preset selection, migration of an old
draft, retained uploaded image, no gradient in preview markup and no console errors.

- **Implemented:** Magic Media → Guest design has Cinema, Editorial and
  Screening room presets; editable category titles/captions/pictures,
  optional background picture and editable featured preview card titles/pictures.
  Presets preserve uploaded artwork and wording. Cinema shelves uses wide tiles
  and landscape artwork inspired by the user's Jellyfin reference.
- **Tested:** appearance, dashboard and overview Node checks pass. Local isolated
  browser checks verified preset selection, edited title, actual PNG upload
  re-encoded to JPEG, save/reload persistence, picture removal and no console
  errors. No real Jellyfin metadata or customer data was changed.
- **Migrated:** not applicable (browser drafts only). **Deployed:** frontend-only.
  Cache versions: appearance JS/CSS 5, dashboard JS 6.
- **Verified:** local browser workflow and live public assets/health; authenticated
  production editor acceptance remains pending.
  Browser drafts are business/account scoped, not server-published themes or
  changes to Jellyfin's native web client. Real catalog/artwork, guest playback,
  entitlement enforcement and publishing remain separate unfinished work.
- Picture input accepts JPG/PNG/WebP up to 5 MB, decodes/resizes/re-encodes locally,
  rejects SVG/remote URLs and bounds stored data URLs. Sandbox preview allows
  only data images, no scripts or external requests. Storage failures are shown.
  Before rollout run the three media tests and diff check, back up the exact
  frontend files, preserve readable public asset permissions and verify fresh
  versioned URLs rather than assuming a local implementation is live.

Build browser tools that let each customer operate their own MikroTik network
and hotspot business. Informational panels do not count as configuration tools.
Discover RouterOS capabilities rather than using model allow-lists; gateway
configuration must work independently of onboard wireless radios.

Current phase: **functional workflow completion and production acceptance**.
Continue the remaining gateway/AP and business workflows, then verify the complete
customer and guest paths. No blanket claim of “fully developed” is supported.

## Map integration requirement (user reminder)

For the OpenFreeMap + MapLibre global map and 2D/3D views, preserve
visible, clickable attribution. Do not hide or disable MapLibre's attribution
control. Retain © OpenMapTiles and OpenStreetMap data credits; also include an
OpenFreeMap credit (optional for the provider, preferred for this project).
Verify credits in both views and on mobile without overlays obscuring them.
Printed maps and map footage in videos need applicable attribution as well.
Provider guidance: https://openfreemap.org/#attribution .
User-supplied source repository: https://github.com/hyperknot/openfreemap .
Frontend deployed on 2026-09-30 using `./deploy.sh web` on `mikromagic-app`.

- **Implemented:** interactive global map on Sites, selectable coordinate markers,
  management-path status colours, fit-locations control, 2D/3D toggle, and building
  extrusions at zoom 15+. MapLibre 6.11.2 is vendored with its license. An empty
  site list displays Myanmar. Missing coordinates never become 0,0 pins.
- **Tested:** `node scripts/test-site-map.mjs`, frontend audit, and diff check pass.
  Local browser acceptance used mock tenant data with real OpenFreeMap tiles:
  Yangon framing, zoom, 3D toggle, visible mobile attribution, no browser errors.
- **Migrated:** not applicable; no database or backend changes for this release.
- **Deployed:** map module, vendor assets, CSS and app markup published. Frontend
  backup: `/home/ubuntu/deploy-backups/sites-map-20260930-rollout/`.
- **Verified:** public map module and worker return 200 with JavaScript MIME;
  app, landing and API health return 200. Live authenticated customer account
  acceptance remains to be checked; local mock acceptance is not production proof.

3D means mapped building extrusions, not satellite imagery or photorealistic
terrain. Coverage varies. Keep attribution visible; the public provider has no
SLA. Update the map module's import version when changing it because assets use
immutable caching. Future agents must retain tenant-scoped site APIs and must
not send customer names/router metadata to the tile provider.

### Sites location picker follow-up (2026-09-30)

- **Implemented/tested/deployed:** Add/Edit Site now offers click-to-place and
  draggable pins that fill latitude/longitude, Pin map centre and Clear pin.
  Coordinate fields remain an optional manual fallback. No location is silently
  saved just because the map opens. New sites default to Asia/Yangon timezone.
- Default Sites viewport is Myanmar (95.9 longitude, 21.9 latitude, zoom 4.8);
  a selected saved site opens its own location. Show all locations fits saved pins.
- Device allowances removed only from Sites, including its unnecessary API read;
  the existing router allowance UI and server enforcement are retained.
- Unit tests cover click/drag coordinate synchronization, clear/centre buttons,
  Myanmar defaults and disposal. Frontend audit and diff check pass. Local mock
  browser verified map click fills coordinates, centre/clear, visible credits and
  absence of the allowance panel, with no console errors.
- **Migration:** none. **Deployment verification:** app/landing/health 200;
  release uses map module version 2 to avoid stale immutable cache. Production
  authenticated site creation/edit acceptance remains unverified; no live customer
  site was created for testing. Backup:
  /home/ubuntu/deploy-backups/sites-picker-20260930-rollout/source-before.tar.gz .

## Stage register

### Magic Media outbound pairing/library pilot (2026-10-01)

- **Implemented locally:** hashed single-use 10-minute tenant pairing; 24-hour
  connector token; revocation/replacement; forced-RLS connection/job migration;
  bounded, at-most-once command queue; active actor role/suspension checks;
  outbound HTTPS runner with hidden local credential prompts; dashboard Connection,
  Libraries and Guest design tabs. Fixed library/status/add/scan operations only.
  Local writes need `-allow-library-changes` plus web confirmation. No deletes,
  remote shell, port exposure, saved credentials or background installation.
- **Tested:** focused Go/race tests and mock connector round-trip pass; handler
  role/context and credential checks pass; entire project compiles and focused vet
  passes. Frontend tests/audit/diff check pass. Owner-role mock browser verifies
  connection status, library listing, add confirmation/result, guest design and
  no console errors. No live Jellyfin mutation or authenticated pairing performed.
- **Database tested (2026-10-01):** all three PostgreSQL integration tests passed
  five consecutive runs with race detection on a disposable PostgreSQL 16 Docker
  container on the VPS. Covered single-use pairing, concurrent at-most-once claim,
  tenant read/write denial, completion replay, revocation, pairing/job/token expiry,
  business suspension, actor deactivation/role/tenant changes and connector
  replacement. The real DB run exposed a SQL NULL result scanning bug, now fixed.
  Tests use minimal business/user fixtures and migrate isolated schemas; this is
  not a full production-schema migration rehearsal or live Jellyfin acceptance.
  `MM_MEDIA_TEST_DSN` still requires literal loopback and `mm_media_test` only.
  Test container/database are disposable; production DB and services untouched.
- **Migration rehearsal:** passed against a schema-only production copy in an
  isolated PostgreSQL 16 container; no customer rows copied.
- **Migrated/deployed (2026-10-01, explicitly approved):** migration
  `20261001120000_magic_media_connector.sql` applied to production `mega_magic`.
  Both tables have enabled/forced RLS and authenticated runtime grants. Scoped
  source deployed using `./deploy.sh all`; API restarted and frontend published.
  Feature enabled through systemd `98-magic-media.conf` with
  `MEDIA_CONNECTOR_ENABLED=true`; source default remains off. Protected rollback
  backups: `/home/ubuntu/deploy-backups/media-connector-20261001-rollout/`
  (`database-before.dump`, `source-before.tgz`, `web-before.tgz`).
- **Production verified (partial):** landing/app/health 200; dashboard module 200
  JavaScript; owner endpoint without login 401; connector without token 401;
  browser-Origin connector request 403. Existing signed-in Nish/DEVELOPER browser
  loads Connection (correctly Not connected), Libraries controls and Guest design
  preview at `https://mikromagic.com/app.html#media`. No pairing code created,
  credential collected, Jellyfin write or payment made for verification.
  Do not confuse this with real connected-library acceptance or a ready installer.
  VPS media/handler/config/server race suites, whole-project compile and focused
  vet passed before rollout. Disposable schema-rehearsal container removed.
- **Remaining:** real read-only owner pairing, signed
  Windows installer/service, secure graphical local credential entry/OS storage,
  job retention/distributed rate limits, real catalog/artwork/guest player and
  separate paid media entitlement enforcement. See MAGIC-MEDIA-INTEGRATION.md.

### Media overview and guest movie theme follow-up (2026-10-01)

- **Media workspace redesign deployed (2026-10-01):** removed duplicate title
  panel; compact tabs, connection status card/three-step key guide, separate
  collection and add-library panes, scoped responsive styling. Mock browser
  library load and frontend tests/UI audit passed with no console errors.
  Frontend-only publish, no API restart or Jellyfin mutation. Initial publish
  rolled back after nginx denied a privately extracted asset; explicit 644
  permissions and fresh versions fixed it. Verified public JS `?v=4` and CSS
  `?v=3` exactly match release source and health is 200. Rollback backups:
  `/home/ubuntu/deploy-backups/media-ui-20261001-rollout/`.
  Owner screenshots now show Connected after the authentication repair; this
  confirms UI connection status, not yet live library-write or dashboard metric
  acceptance. Guest palettes and paid-access behavior were not changed.

- **Local compatibility repair:** owner's first pairing attempt reached Jellyfin
  but failed before redeeming the cloud code. Public local server probe confirms
  Jellyfin 12.1.0. Adapter used legacy `X-Emby-Token`, disabled by default in v12;
  switched to encoded `Authorization: MediaBrowser Token="…"`, with regression
  tests, and rebuilt `build/magic-media-connector-mac`. Owner must retry with the
  existing key entered locally. No key collection, reset, legacy-auth enablement
  or automatic pairing performed. This is a local connector change; the running
  cloud API needs no restart for this repair.

- **Implemented locally:** fixed read-only `dashboard` jobs; typed Jellyfin server,
  library counts, active sessions and now-playing summaries; native whole-computer
  CPU/RAM and local drive measurements. Overview is manually refreshed and dated,
  not a continuously live chart. Missing readings remain Unavailable, not zero.
  API keys, raw API responses, private IPs and unrelated paths are excluded.
- **Guest design:** existing Forest/Midnight/Cinema plum themes, accent colour and
  branding remain; Comfortable/Compact movie-poster layouts added. This is still
  an account/business-scoped browser draft and preview, not a published guest
  catalog. Preserve this appearance contract when wiring real Jellyfin artwork,
  movie lists and guest playback; media purchases remain separate from internet.
- **Tested:** media race suites, full-project compile, focused vet, frontend unit
  checks and UI audit passed. Four PostgreSQL tests passed three consecutive race
  runs on disposable PostgreSQL 16, including dashboard completion/isolation.
  Mock browser verified Overview cards (including zero versus unavailable).
  Guest preview verified Compact poster layout and Cinema plum selections with
  no browser console errors; existing theme normalization/rendering tests pass.
  Windows amd64 and macOS arm64 connector builds pass; neither is a signed installer.
- **Migrated/deployed (2026-10-01):** after the user's continuation approval, a
  protected backup and isolated production-schema-only rehearsal passed. Applied
  `20261001140000_magic_media_dashboard.sql`, deployed scoped API/web through
  `./deploy.sh all`. Backups:
  `/home/ubuntu/deploy-backups/media-dashboard-20261001-rollout/`. The opted-in API
  checks the dashboard operation constraint before startup; existing forced RLS
  remains intact. Disposable rehearsal database removed after successful tests.
- **Production verified (partial):** landing/app/health 200; unauthenticated
  owner/connector routes 401; browser-Origin connector request 403; versioned
  dashboard and appearance CDN modules exactly match release source. No signed-in
  browser session was available for this rollout, and real paired-Jellyfin readings
  remain unverified. No live library changes, guest purchases or credential entry.
- **Next:** updated Mac build `build/magic-media-connector-mac` is prepared, not
  launched. Perform real read-only pairing acceptance with credentials entered
  locally by the owner. Do not collect keys in chat.
  Guest catalog/artwork, published themes, player and paid entitlement enforcement
  are separate unfinished work, not features validated by this overview slice.

### Magic Media local library adapter details (2026-10-01)

- **Implemented:** `internal/media/jellyfin` loopback-only client: public status,
  authenticated library list/add and scan request. Header-only credentials,
  redirect/proxy refusal, bounded responses, sanitized errors, no write retries.
  `cmd/media-check` is a read-only local diagnostic, not an installer/connector.
- **Tested:** focused adapter tests with race detection pass, including empty
  libraries, Windows/POSIX paths, comma-containing folders, validation, cancellation,
  redirects and credential redaction. API requests exercised against mock servers.
- **Verified locally:** the owner's Jellyfin responds as version 12.1.0 and setup
  complete. The new checker successfully probes it without credentials. No account,
  key, library or router changes. Authenticated live operations remain unverified.
- **Migrated:** not applicable. **Deployed:** not deployed; no production changes.
  Existing guest appearance preview remains unchanged. No functional cloud pairing,
  tenant media API, installer, paid access or guest playback is supplied yet.
- See [integration notes](MAGIC-MEDIA-INTEGRATION.md) for contracts, local checks,
  next steps and authorization boundaries. Next is outbound tenant-bound pairing
  and secure local credential entry, then real dashboard library operations.

### Magic Media guest appearance prototype (2026-10-01)

- User-approved direction: owner-provided Windows PC hosts Jellyfin; Mega Magic
  supplies simplified management and separate paid media access (not bundled
  with internet). Windows first. One Magic Media sidebar section should ultimately
  contain setup and dashboard, with an owner-editable guest web experience.
- **Implemented/tested/deployed:** `#media` appearance editor and sandboxed guest
  iframe preview: brand text, three themes, accent colour, category visibility,
  featured sample artwork, sample pass price/duration, desktop/mobile sizing.
  Browser-only drafts are scoped by business ID and account email. No server-side
  persistence or guest publishing. OWNER/ADMIN/DEVELOPER visibility and route gate.
  Product guide now filters entries using the same presentation policy.
- Preview artwork and titles are explicitly demos; purchase button is disabled.
  Text is escaped, theme/colour are validated, iframe prohibits script/network
  access. No Jellyfin/API keys, router writes, purchase creation or OS installation.
- **Tests:** media normalization/escaping/sample-only checks, role checks, frontend
  audit and diff check pass. Owner-role mock browser verified live branding,
  browser draft save, mobile preview, no horizontal page overflow or console errors.
- **Migrated:** not applicable. **Deployed:** frontend-only `deploy.sh web`.
  App, landing and health return 200. Live authenticated customer acceptance is
  still unverified. Backup: /home/ubuntu/deploy-backups/media-preview-20261001-rollout/ .
- **Next:** secure Windows installer/connector pairing, tenant-bound Jellyfin
  integration, real library data and authenticated playback, media plans/payment
  approval + expiry enforcement, server-persisted design with publish/revert.
  Build and verify those workflows before replacing any prototype labels.
  PC provisioning, network changes, production migrations and paid infrastructure
  require the applicable owner authorization; do not infer permission to alter
  customer PCs or gateways from permission to publish this design preview.

### Dedicated gateway workspace and sidebar (2026-09-30)

- **Implemented/tested/deployed:** new `#gateways` page filters devices by
  MAIN_GATEWAY role (not model name), keeps connection records and existing
  configuration actions there, and excludes capacity/request panels. New gateway
  records use tunnel transport and only MAIN_GATEWAY as the layer.
- `#routers` now presents downstream switches/APs/controllers; gateway cards and
  connection forms no longer mix with that device inventory.
- Sidebar: Platform and shared Translations are DEVELOPER-only; Templates,
  Terminal log, AI usage and Team are OWNER/ADMIN/DEVELOPER tools. Customers retain
  their tenant monitoring, security, backups, sites and business tools.
  Empty sidebar headings hide. Hidden controls have explicit CSS enforcement,
  and restricted hash routes are blocked before their renderer runs.
- This is UI presentation policy, not a substitute for API authorization.
  Existing server permissions/RLS were not changed. No database migration.
- Tests: navigation policy checks, frontend audit, diff check, local mock browser
  gateway-only form/filter, owner hidden tabs and rejected Translations hash pass.
  Live app/landing/API health checks return 200 after `deploy.sh web`.
  Live authenticated tenant acceptance remains unverified.
- Backup: /home/ubuntu/deploy-backups/gateway-page-20260930-rollout/ .
- **Not completed:** Magic Hub enrollment/key exchange/peer installation and
  authenticated connection verification. A gateway record is not a connection.
  Next priority is tenant-bound Hub enrollment, with isolated address allocation,
  narrowly scoped router bootstrap, server peer reconciliation, verified
  tunnel + RouterOS authentication and revocation. Do not claim this UI release
  supplies that backend or automatically enroll existing customer hardware.

### Telegram readability and guest approver identity

- **Implemented:** friendly Store business names and formatted amounts; short
  display-only order references for Store/guest notifications and decision acks.
  Full IDs remain internal to callback decisions. Profile includes a numeric
  Telegram approver user ID, separate from the destination chat ID.
- **Security:** guest bridge checks sender, chat, live binding and purchase
  business; disabled/replaced bindings fail closed; old loops are canceled.
  Binding writes now use the tenant transaction rather than the root DB handle.
- **Tested:** targeted Go packages and authorization/message unit tests;
  PostgreSQL integration tests require a disposable DB and remain unverified.
- **Migrated:** user-authorized additive migration applied on production;
  `approver_user_id` is bigint, NOT NULL, default 0, with RLS still enabled.
- **Deployed:** API + web published using `deploy.sh all`; service active,
  landing/app/health HTTP 200. Targeted Go packages pass on the VPS.
  Backup: `/home/ubuntu/deploy-backups/telegram-approver-20260930-rollout`
  (database dump, application and served-frontend snapshots).
- **Live verified (partial):** schema and rollout checks passed. Authenticated
  Profile save/read-back and real guest approval remain unverified: the available
  browser has no signed-in session. One existing enabled binding has no approver
  ID, so its guest Telegram approvals pause until its owner fills in that field.
- **Next:** owner saves numeric user ID in Profile → Telegram action approvals;
  verify read-back, then an authorized real guest approval and wrong-user denial.
  No real payment was submitted or approved just for these tests.

Track **implemented → tested → migrated → deployed → verified** independently.
`Partial` limits the claim to the stated checks. `N/A` means no change is needed.
No evidence or a skipped test means `unverified`.

### Magic Dude Gemini provider

- **Implemented:** native Gemini chat client, independent `ai.magic_dude_provider`
  selector, protected environment key loading, safe rate-limit errors and token
  accounting. Fleet scanning keeps its separate provider configuration.
- **Tested:** provider header/role mapping, redacted error handling, answer/thought
  separation, usage accounting and configuration loading/missing-key refusal;
  affected AI/config/handler/server suites passed.
- **Migrated:** N/A.
- **Deployed:** Gemini provider support and interactive setup script installed on
  `mikromagic-app`, 2026-09-30. Affected tests and schema preflights passed on
  VPS; canonical API deployment completed, service active, landing/app/health
  returned `200`. Gemini activation awaits the replacement key; the existing
  provider configuration was retained.
- **Verified:** mock provider only. Google credentials and real chat remain
  unverified until the owner enters a replacement key on the VPS. The key shown
  in the screenshot is exposed; do not reuse it.
- **Entry point:** [provider setup guide](MAGIC-DUDE-PROVIDER.md),
  `bash scripts/configure-magic-dude-gemini.sh` on `mikromagic-app`. Hidden prompt;
  root-owned `/etc/mega-magic/magic-dude.env`; default `gemini-3.8-flash`.
- **Backup:** `/home/ubuntu/deploy-backups/gemini-provider-20260930-215227`.

### Customer-network gateway workflow

- **Implemented:** `internal/provisioner/routeros/gateway_setup.go`,
  `internal/handler/gateway_handler.go`, server routes and `web/app.html` form.
  Preview, backup, apply, readback and partial-failure cleanup are present.
- **Tested:** unit validation, preview binding, duplicate/overlap refusal,
  cleanup and foreign-tenant refusal; mock browser preview/apply. Passed.
- **Migrated:** N/A for this workflow; existing device/credential schema required.
- **Deployed:** API/web published on `mikromagic-app`, 2026-09-30.
- **Verified — partial:** published form and protected endpoint checked. New
  network mutation and real guest connectivity were not exercised on the live
  CHR during rollout.
- **Scope:** existing uplink selection; separate LAN/optional VLAN, DHCP/client
  DNS, scoped NAT, optional Hotspot and connection to existing RADIUS. Uplink/
  default-route provisioning and installation of a RADIUS server are not included.
  Existing firewall policy requires separate connectivity acceptance.

### Secure WebFig setup

- **Implemented:** `internal/provisioner/routeros/webfig.go`, admin route
  `POST /api/v1/routers/:id/webfig/secure`, router panel action. Creates/reuses a
  certificate, restricts HTTPS to configured trusted networks, disables HTTP,
  saves backups, reads back services and restores on failure.
- **Tested:** unit success, failure restoration and tenant-target checks; mock
  browser action/result. Full Go suite, UI audit and browser acceptance passed.
- **Migrated:** N/A.
- **Deployed:** API/web rollout completed on 2026-09-30.
- **Verified — partial:** anonymous request `401`, published UI, service active,
  health `200`, post-restart logs checked. No live CHR WebFig change was executed.
- **Remaining:** disposable-CHR certificate/service acceptance, rollback readback,
  private-network browser access and authenticated remote WebFig session/proxy.
  A private URL is not proof of cloud browser reachability.
- **Backup:** `/home/ubuntu/deploy-backups/webfig-20260930-211039`.

### Authentication hardening

- **Implemented:** session revocation on password/role/account/business changes
  and logout; refresh rotation/reuse detection; strict JWT validation; HttpOnly
  cookies; cross-origin write protection; authentication rate limiting.
- **Tested:** Go tests, focused race checks and four browser-client tests reported
  passing; latest full Go suite passed locally and on the VPS. New PostgreSQL
  session integration tests remain **unverified** without a disposable-DB run.
- **Migrated:** `20260930220000_auth_sessions.sql` applied on VPS; authentication
  schema preflight passed during deployment.
- **Deployed:** production API/web updated on 2026-09-30; users must sign in again.
- **Verified — partial:** schema readiness, protected routes, cross-origin write
  rejection and health checked. Full session lifecycle/concurrent refresh/reuse
  acceptance is not established by those checks.

### Frontend behavior repairs

- **Implemented:** customer-oriented wording, navigation-race repair, action/
  status distinctions and mobile behavior repairs in the current static frontend.
- **Tested:** static audit and mock browser acceptance covering Fleet, Topology,
  Revenue, portal, Magic Dude, gateway/WebFig and narrow layouts passed.
- **Migrated:** N/A.
- **Deployed:** pending frontend batch published with the 2026-09-30 rollout.
- **Verified — partial:** page/assets published; complete authenticated production
  browser regression and cross-browser runs remain outstanding.

### Store and bank-transfer grace prerequisites

- **Implemented:** Store transaction/activation core and tenant permission repair;
  customizable time/data grace fields and RouterOS grace control. Default data
  allowance is 50 MB; access ends at the first exhausted time or data limit.
- **Tested:** automated coverage exists. Inspect execution environment and skips
  before asserting complete money-path acceptance.
- **Migrated:** grace-data and Store runtime permission migrations applied in
  earlier 2026-09-30 work. Store preflight passed again during latest deployment.
- **Deployed:** earlier 2026-09-30 batch; latest schema checks remained green.
- **Verified — partial:** current end-to-end paid activation, duplicate decisions,
  bot approvals and real grace exhaustion require explicit acceptance records.

No migration is known to be awaiting application for the latest gateway/WebFig
and auth batch. This does not establish that every historical migration ran.
New schema changes require their own apply/check records. Older “production
unchanged” and “auth migration pending” messages were superseded by this rollout.

## Next tasks and required checks

1. Certify new gateway/WebFig operations on disposable CHR: apply, fresh readback,
   failure/cleanup, foreign-tenant refusal and management-path continuity. Exercise
   actual guest DHCP, internet and captive login. Recheck certificate collisions,
   TLS settings, restored service state and HTTPS port handling.
2. Finish WAN/LAN, bridge/VLAN and existing DHCP/NAT/Hotspot/RADIUS editing beyond
   creating a new customer network. Each write needs validation, preview, backup,
   scoped apply/readback and recovery evidence.
3. Implement local Wi-Fi and controller/external AP configuration from discovered
   capabilities. SSID controls belong to the actual AP/radio. CHR's lack of a radio
   must not restrict another customer's gateway features.
4. Complete secure cloud WebFig access with tenant/device authorization, session
   expiry, reachable management transport and audit checks.
5. Run auth-session PostgreSQL integration tests on a disposable database,
   including refresh reuse, revocation triggers and suspension/reactivation.
6. Record complete Store/guest purchase acceptance: submission, correct bot/web
   approval, exactly-once entitlement/ledger effects, rejection, reload and tenant
   refusal. Prove grace time/data exhaustion on RouterOS and reconcile Revenue
   with approved hotspot sales.
7. Complete operational topology/guest tools and Myanmar/Chinese localization
   checks. Use historical recovery requirements as context, then inspect current
   code before assuming an old gap is still open.

Run relevant Go/client/browser checks and `git diff --check` for changes. Use
verbose disposable-database/lab test output to distinguish executed tests from
skips. Follow [hardware certification](HARDWARE-CERTIFICATION.md) and
[migration procedure](DATABASE-MIGRATIONS.md). After releasing, record revision,
environment, backup, schema results, deploy result and exact feature smoke checks.
Schema readiness is not proof of database transaction behavior.

## Authorization and tenant boundaries

The owner previously authorized the VPS deployment connection to finish and
publish pending application work. Routine backed-up API/web deployments and
necessary additive migrations for that work have authorization; do not ask again
solely because an already-authorized deployment is the next step. The current
README/handoff request calls for local documentation edits.

Additional owner authorization is required for these actions **unless the exact
target and purpose already have explicit authorization**:

- Disruptive customer-router configuration, firewall/management route changes,
  reboot or reset. App deployment authorization does not cover arbitrary devices.
- Destructive database operations, restoring/overwriting live data, tenant
  deletion, privilege expansion or disabling authentication/RLS protections.
- Live payment decisions, coin credits, entitlement grants or customer payment
  account changes outside an authorized test or operating decision.
- Hub networking, domain/DNS routing, credentials or unrelated service changes
  beyond the application deployment scope.

Disposable-lab tests have prior authorization within documented guardrails.
Read-only diagnosis and routine local implementation can proceed. Check existing
target-specific permission before requesting it again.

The verified DB login is `mega_admin` (superuser, `BYPASSRLS`), distinct from app
roles. App access comes from `users.user_role` and `platform_admins`; the owner
identified `nish` as the owner admin. Tenant DB operations must establish the
restricted `authenticated` role and business context. Inspect the actual
transaction/connection path: the DB owner login alone provides no tenant isolation.
Previously leaked secrets were revoked/rotated. Do not ask for or reuse transcript
passwords/tokens; use configured protected credentials and redact secret output.

## Historical records

### 2026-10-03 Magic Media publishing follow-up

Server-saved guest designs and planned pass settings, public links, revision
conflict checks and unpublishing implemented. This does not enable paid playback;
the combined purchase/approval/identity/timed-access/expiry runtime remains open.
Go race/static/browser checks passed, as did three isolated PostgreSQL publication
tests. Production backup and schema-only rehearsal completed with owner approval.
Migration `20261003120000_magic_media_publications.sql` was applied, forced RLS,
tenant policy, grants and non-bypass runtime role checked. API/web deployment
completed at `d51e212`; landing/app/health returned 200. Signed-in Nish live
Guest design shows **Publish guest page** enabled and **No guest page has been
published**, proving the new owner read path loads. No business page was published
automatically, and no live Jellyfin/paid playback acceptance was performed.
See [media publishing](MEDIA-PUBLISHING.md) for contracts, verification and backup.

`specs/PROGRESS.md`, `specs/ROADMAP.md` and first-project recovery audits are dated
history/design context; some claims conflict with later work. Preserve their owner
requirements and decisions, but use this handoff and current environment evidence
for delivery status. The acceptance register's proof requirements still apply;
its old coverage/gap lists need reconciliation with current code.
