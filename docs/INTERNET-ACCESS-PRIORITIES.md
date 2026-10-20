# Owner's internet-access priorities — 2026-10-21

Work on the existing Go/web app in this order before unrelated media, cosmetics,
or housekeeping work:

1. Add a router and complete its Magic Hub management connection.
2. Quick configuration and basic network configuration.
3. Issue and print batches of guest internet vouchers.
4. Voucher sales/revenue reporting.
5. Internet Hotspot captive-portal login and guest internet access.

Treat this as one customer journey: site → router connection → network setup →
voucher batch → guest captive-portal login → verified access → accurate reporting.
Preserve the current stack, records, tenant isolation, Hub integration, and the
separation between Internet Hotspot and Jellyfin/media workflows.

## Acceptance checks

- Router: registration/site assignment alone is not Connected. Require a fresh
  Hub handshake and authenticated, identity-bound management read. Reuse saved
  management credentials across settings pages. Retrying setup must not rotate
  an existing reserved tunnel key or allocate a second connection.
- Quick/basic configuration: read actual router settings, keep WAN/LAN roles
  explicit, retain review/apply/readback/rollback, and preserve management access.
- Voucher batches: issue the guest-internet product for the correct business
  and gateway, retain time/data limits, unique codes, printing, and first-use
  validity. Business subscription passes are a separate product.
- Revenue: distinguish issued face value, voucher usage, confirmed voucher sales
  and approved portal payments. Do not label printing or redemption as cash
  received without a corresponding sales record. Current full-ledger portal
  payment totals are not a complete voucher-sales dashboard.
- Captive portal: verify the installed profile, correct gateway/client binding,
  valid-code login, actual internet traffic, time/data enforcement, reconnect,
  expiry/revocation, and denied invalid/foreign codes on a real guest device.

## Current evidence and boundaries

Commit `0747795` repairs a reproduced one-paste retry failure: a transient Hub
response or management-preparation failure no longer loses the private half of
the already reserved public key during the 30-minute process-memory setup window.
The key cache is tenant/device-bound, synchronized and bounded to 256 attempts.
Expiry/restart retains the existing explicit recovery hold; it does not silently
rotate an established peer. Nothing is persisted to the database or sent to Hub
except the public key.

Clean-source verification: 576 top-level internal Go tests passed, zero failed;
focused router/Hub race tests passed; critical UI 10/10, audit and both builds
passed. Separate disposable database tests passed. Eight router lab/live/hardware
tests remain skipped after separately exercising the other database fixtures.

The owner identified the previously tested account's registered `hAP ax2 lab`
router and confirmed that the hardware is not currently available. A current-run
read-only live app check returned tunnel address `10.77.0.8/32`, no recent
handshake, and “Last handshake: Never observed.” This describes the current Hub
observation, not proof of the device's lifetime connection history or its cause.
No reset, new peer, site update, router configuration or production deployment
was performed. RouterOS version and real guest internet acceptance are pending.

Next: follow this priority order for remaining local app work, preserving the
mixed uncommitted revamp. Resume physical connection and end-to-end acceptance
when the router is available; this document does not authorize production or
router mutations.

## Quick Config recovery — 2026-10-21

Fixed two reproduced recovery failures in the identity/DNS applier. Recovery no
longer inherits cancellation or an expired deadline from the original request;
it retains context values with its own 30-second limit. The app reports settings
as restored only after reading back and matching the previous identity/DNS
values. Failed recovery preserves the original error and reports the rollback
failure rather than claiming restoration. This restores targeted settings, not
the complete router configuration or every firewall rule.

New tests reproduced failures before the fix, then passed. An isolated export
of committed source plus the two changed Go files passed the entire RouterOS
package with race detection: 119 top-level passes, zero failures and eight
explicit lab/live/hardware skips. The actual Go server build passed. Evidence:
`/Users/aces/Documents/Codex/2026-10-21/asd/work/quick-config-recovery/`.

Next local check: saved-router reuse and the basic LAN/DHCP/Hotspot setup flow,
then voucher batch issuance/printing. This recovery fix does not establish that
all basic configuration or guest internet functions are complete. The physical
router remains unavailable; no deployment or router write was performed.

## Basic setup context and apply controls — 2026-10-21

Initial LAN/DHCP/Hotspot setup now restores only an authorized saved gateway or
automatically reads the single registered gateway. A manual selection is shared
with the other network pages; unavailable browser storage does not block reads.
This initial setup no longer depends on the unfinished navigation module.

Applying locks the gateway selector and form fields, consumes the reviewed
request once, prevents duplicate preview/apply requests and requires literal
`verified: true` before reporting success. Changing inputs or selections
invalidates review; stale inventory cannot replace the current gateway form.
Failure unlocks the controls and requires a new review before retrying. Physical
WAN/LAN roles remain explicit, and opening the page performs reads only.

Nine new DOM regressions passed. Isolated source with fresh npm dependencies
passed all 19 critical UI test entries; the mixed working tree passed 22,
including its existing uncommitted workflow tests. Real Chromium acceptance
passed navigation, suggestions, explicit ports, preview invalidation, synthetic
verified apply and 390px layout. These use synthetic APIs, not a physical router.
Evidence: `/Users/aces/Documents/Codex/2026-10-21/asd/work/gateway-setup/`.

Continue with guest internet voucher batch issuance and printing. Actual guest
DHCP, DNS, captive login and internet access still require the unavailable router.

## Guest voucher batch printing — 2026-10-21

Issuance now keeps the exact returned batch selected for printing instead of
discarding it and selecting unrelated inventory. The operator can explicitly
switch to loaded inventory. Selections remain bounded to 100 codes, with Next
for larger batches; preview samples never enter print documents. A successful
issuance remains printable when inventory refresh fails. A view-rendering error
after issuance reports that codes were created and blocks a duplicate submit.

Printed prices now use the API's `price_mmk`, including zero, with the legacy
`amount` fallback retained. Tiles include the plan and known guest validity/data
allowances. New-batch data allowance comes from the issuance response; existing
inventory without that field does not invent one. Refreshed usage/expiry status
is applied to retained batch IDs without admitting unrelated codes.

Preview and print share border-box sizing and adaptive type. A4 geometry reserves
padding and cut gaps; overly dense tiles omit complete detail lines with a
preview notice, preserving the code. Continuous thermal strips can grow to keep
codes readable. The latest-batch selection is local to this page, retained when
saving its print layout; persistent batch history is not implemented here.

Six new DOM tests passed. Isolated source with fresh npm dependencies passed all
25 critical UI entries; the mixed tree passed 28. Six-surface web audit passed.
Chromium accepted one synthetic 105-code issuance, disjoint 100 + 5 printed
selections, correct price/time/data, no samples/older codes in printed documents,
fitting A4/thermal tiles and a 390px viewport. No physical printer, router or
production issuance was used. Evidence is under
`/Users/aces/Documents/Codex/2026-10-21/asd/work/voucher-batch/`.

Next priority is voucher revenue: distinguish issued face value and usage from
confirmed sales and approved portal payments. Real captive login, internet
traffic, quotas and reconnection remain pending hardware acceptance.

## Owner-confirmed activation-based voucher reporting — 2026-10-21

The owner clarified that printing leaves a voucher unused. The guest buys the
printed code and enters it on the Internet Hotspot portal. Successful activation
should update the dashboard automatically; there is no additional staff sale
confirmation step. Label this **activated voucher value**, separately from
approved portal payments or bank-confirmed cash settlement.

The existing first-use reservation fixes elapsed validity and the client binding;
printing does not start it, and retries/reconnects do not replenish validity or
data. The new nullable `guest_voucher_grants.activated_at` is written once by
verified finalization, not by issuance, printing or reservation. A database
trigger protects that timestamp. Failed/stale finalization creates no activation
value. Existing active records are not assigned invented historical timestamps.

The new authenticated, tenant-scoped `GET /api/v1/voucher-report` summarizes the
complete recorded guest-grant inventory and activation value by Yangon day,
7/30-day windows and gateway. Business passes, foreign tenants and vouchers
without guest grants are excluded. Unused stock, activation in progress, active
grants, expiry/revocation and first-use records without a verified timestamp are
shown separately. Recorded activation value remains historical after revocation;
it is not a refund-adjusted cash ledger or a current online-guest count.

Revenue now keeps voucher activations and approved portal payments in separate
sections. An unavailable source does not hide the other or become zero. The
payment inbox and CSV identify portal versus business-pass sources and clearly
describe their limited inbox scope.

Clean-source verification with fresh npm dependencies passed 31 UI entries,
all six web-audit surfaces, the actual Go server build and Chromium revenue
acceptance including payment approval/readback and 390px layout. The internal
Go suite on disposable PostgreSQL passed 582 top-level tests with zero failures;
20 separately gated fixture/hardware tests skipped. The mixed tree passed 34 UI
entries. Database coverage includes activation exactly once, immutable time,
no legacy backfill, tenant/RLS isolation, a 139-record report and Yangon windows.

**Rollout dependency:** apply
`migrations/20261021090000_guest_voucher_activation.sql` before deploying this
backend with guest voucher access enabled. Startup preflight rejects a missing
column/guard; the app does not auto-migrate. The migration was exercised only in
disposable local databases. No production migration, deployment, payment,
voucher issuance or router write occurred. If reverting the application binary,
retain the additive column to preserve activation history.

Evidence: `/Users/aces/Documents/Codex/2026-10-21/asd/work/voucher-report/`.
Next: Internet Hotspot portal flow checks locally, then physical DHCP/DNS,
valid/invalid-code login, internet traffic, quota/expiry and reconnection when
the owner's lab router is available.
