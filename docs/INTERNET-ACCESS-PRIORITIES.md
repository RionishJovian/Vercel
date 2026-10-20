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
