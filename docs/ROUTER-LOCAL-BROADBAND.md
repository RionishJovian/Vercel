# Router-local broadband accounts

Implementation date: 2026-11-04. This document describes local source changes,
not a production deployment or physical RouterBOARD acceptance result.

## Owner workflow

Customer internet (`/v2/#/network/pppoe`) now includes paid broadband accounts.
Use a PPP plan configured in the existing workspace, choose a username/password,
enter the period in days and payment received, then confirm the displayed router.
The backend uses the saved Magic Hub management connection to install the account.
No customer browser receives router credentials or issues RouterOS commands.

Existing PPP-plan labels may contain spaces, punctuation and Unicode within the
existing 64-character database limit. Labels are preserved exactly and passed as
quoted RouterOS values. Empty names, control characters and leading/trailing
whitespace are rejected; subscriber username rules are unchanged.

- New accounts start their purchased period at the first successful connection.
- One day means 24 hours; a purchase accepts 1–366 days.
- Renewing an unused account adds prepaid days without starting its clock.
- Early renewal extends the existing expiry. Late renewal starts the added period
  at router application, while retaining the original first-use timestamp.
- Renewal preserves suspension. Resume is a separate intentional action and does
  not extend expiry. Expiry disables the secret and disconnects its PPPoE session;
  the account is retained.
- An uncertain request remains pending. Use **Retry saved operation** rather than
  entering a replacement payment. The saved identity prevents repeated extensions.
- If a success reply is lost, repeating the same installation or renewal with unchanged details on
  the current screen reuses its unconfirmed identity. Once success is acknowledged,
  a deliberate next renewal receives a new identity. This temporary retry state is
  held only in memory; server-saved pending operations remain recoverable after
  reopening the screen. Subscriber passwords are not placed in browser storage.
  The confirmation identifies a retry of an earlier unconfirmed change. Only that
  exact operation's acknowledgement or definite rejection clears its retry identity;
  other actions do not clear unresolved payments. Suspend/resume retain their
  existing request behavior and cannot replay an obsolete cached action.
- **Refresh from router** only reconciles evidence. It never records a sale.

The Revenue page has a separate PPPoE recorded-payment section. It counts applied
create/renew operations once, using the original request date for “today” in
Asia/Yangon. Pending operations are shown separately. These amounts are entered by
the owner; they are not bank settlement or inferred session/usage revenue. Existing
Hotspot activation and payment summaries retain their own definitions.

## Persistence and isolation

`db/migrations/20261104000000_broadband.sql` adds `broadband_accounts`,
`broadband_operations` and `broadband_session_usage`. Each row has business and
router ownership, composite foreign keys, and forced tenant RLS. Operation order
and payment/request identity are immutable; applied and rejected states are
terminal. One pending operation is allowed per account.

Pending operation material, including a new account's password, is encrypted using
the existing credential key and included in the existing credential-rotation
workflow. HTTP responses exclude ciphertext and request hashes. Passwords are
excluded from the request digest; a retry compares the decrypted saved password.
The encrypted provisioning material remains saved for operation recovery.

Account locking occurs before creating the operation snapshot. SQL transactions
are committed before contacting a router. A marker on the router identifies the
last operation; reconciliation accepts only known operations for that account and
does not let delayed reads replace newer operations or their traffic samples.
Only a proven pre-install create rejection retires its app record and releases
the username; an ambiguous transport failure never does so.

## Router installation and outage behavior

The adapter reuses the existing SSH transport, host verification and device write
serialization. It does not enable RADIUS, change Hotspot configuration or replace
unmanaged subscribers. It clones the chosen PPP profile into an owned per-account
profile, retaining addressing/rate/IPv6 settings. Source profiles with custom
connection hooks are refused rather than overwritten. Generic editors cannot
modify the owned accounts/profiles or assign those profiles to unrelated accounts.

The secret comment stores operation identity, prepaid period, suspension and
explicit expiry. The profile comment stores immutable first-use evidence. Keeping
the anchor separate lets a login and renewal overlap without overwriting each
other's operation records. The first-use hook verifies the actual username.

An owned local scheduler checks accounts every 15 seconds. It disables/disconnects
expired, suspended, malformed or clock-unverified accounts. Once a temporary
startup clock pause clears, valid accounts can be enabled again; enabling is
conditional on the exact state that was evaluated, so an older check cannot undo
a newer suspension. Use the app's Suspend action for managed accounts: direct
manual changes to `disabled` are not an independent suspension policy.

Time enforcement requires RouterOS timestamp support and a synchronized NTP client
at initial provisioning and after a reboot/new script execution context. The app
also compares router and server clocks before provisioning/readback. A volatile
watermark detects backwards movement without repeatedly writing router flash.
After initialization, loss of the app or NTP reachability does not itself stop
local authentication or expiry checks. A clock rollback fails closed; correcting
such a rollback may require restoring trusted time and restarting the execution
context. The application does not silently replace existing NTP configuration.

Prerequisites are an existing functioning PPPoE service/address plan, saved Hub
management access, required script permissions and trusted router time. This batch
automates subscriber installation; it does not claim to configure an arbitrary
unprepared router's WAN, PPPoE server or NTP configuration from factory defaults.
Those remain existing network setup/onboarding responsibilities.

The background worker reads each business/router independently of Hotspot and app
subscription expiry. Missing or altered owned router records retain previous app
evidence and leave uncertain changes pending. Retrying an operation also completes
and verifies any remaining disable/disconnect side effects before acknowledging it.

## Accounting limits

Session traffic is **observed traffic**, not a complete offline accounting journal.
Samples are kept as per-session maxima, not added repeatedly. Counter regressions
or reused IDs are marked as uncertain. Sessions that start and finish entirely
between successful polls cannot be reconstructed by this implementation. A router
reboot can erase unsampled counters. No data-volume billing or promise of exact
outage usage is based on these samples. Paid validity and payment deduplication do
not depend on traffic samples.

Hotspot recovery also now expires a first-observed voucher in the same catch-up
transaction if its original first-use evidence already places it past expiry.
It retains the original anchor rather than granting a fresh validity period.

## Verification and release

Verified locally with a disposable PostgreSQL 17 instance: full internal Go tests
with the race detector, tenant isolation/operation recovery/session regressions,
credential rotation, and the migration runner including the real baseline and
migrations. Frontend tests, lint and production build, existing critical UI tests,
deployment configuration and Vercel proxy regressions were also checked. Known
existing warnings: NetworkEditor hook dependency and the large map bundle.
Browser review uses synthetic API fixtures; it is not live router evidence.

Router command fixtures check commands and recovery behavior; they do not execute
RouterOS. A controlled router trial should cover simultaneous Hotspot/PPPoE,
first-use/expiry while the app is unreachable, renewal/retry, reboot/NTP recovery,
suspension and generic-editor protection on the actual installed RouterOS version.
Expiry is a periodic check, not a hard real-time deadline; its nominal interval is
15 seconds, plus execution delay under load. Subscriber capacity is not benchmarked.

For a separately authorized release: preserve a database backup and the current
release, apply the additive migration using `deploy/server/mm-migrate up`, run the
existing runtime grants step, then release the API and `/v2/` build together. Keep
the existing credential key and explicit `MM_NETWORK_CONTROLS_ENABLED` gate. The
saved management applier must be enabled for router operations/background reads.
Missing schema leaves this new section unavailable without altering existing
product data. Do not undo this feature by deleting live accounts, tables or router
records: rolling back the UI/API leaves installed router-local enforcement active.
An older backend cannot rotate these new encrypted records, so retain the current
credential key until a broadband-aware version handles rotation again.

Official integration references: [PPP AAA](https://manual.mikrotik.com/docs/authentication-authorization-accounting/ppp-aaa/),
[scripting](https://manual.mikrotik.com/docs/developer-guides/scripting/),
[scheduler](https://manual.mikrotik.com/docs/system-information-and-utilities/scheduler/).
Documentation capabilities must be checked against the installed router version.
