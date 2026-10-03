# Live app re-audit — 2026-10-03

## Verdict

**Reachable and improved, but NOT fully functional across the requested business workflows.**

Fresh read-only checks began at 14:52 UTC / 21:22 Asia/Yangon. Production checkout
was clean at `91f6ae3`; API and nginx were active. This report supersedes the
earlier audit's current-status claims, not its historical evidence. Local dirty
paid-media code is NOT part of this production version.

No deployment, database migration, publication update, payment, approval,
account change, router apply, reboot or restore occurred. Device selections
only fetched router inventory. The browser was signed in as Developer; other
roles and cross-tenant negative cases were not tested.

## Earlier defects rechecked — fixed in the live presentation

- **Revenue statuses:** six existing requests display Approved/Rejected, not
  Unknown. Revenue explicitly labels owner approval rather than bank settlement.
- **Topology/fleet:** direct management is labeled Management reachable, not
  Tunnel active. Topology explicitly disclaims verified WireGuard/customer traffic.
- **Security recommendation:** the stable 7.15.3 CHR now shows upgrade to >=7.24.2,
  rather than the earlier universal 7.23.4 threshold. This checks the live
  recommendation only, not every advisory/version combination or exploitation.
- **Guide:** WAN/failover, PPPoE, Wi-Fi and existing-network workspaces are covered.
- **Voucher capability:** the UI now explicitly says guest voucher creation is
  not enabled; sample layouts do not grant internet access.
- **Templates:** 51 templates load and the page explicitly describes a reference
  library, not an execution console. This fixes the presentation, not template execution.

## Confirmed unavailable functions and readiness gaps

### P1 — Paid Magic Media is still not a live service

The published guest page has a disabled Media access not available yet button.
The public API returns revision 1, `purchases_enabled:false` and
`playback_enabled:false`. The deployed source lacks
`internal/handler/media_purchase_handler.go` and registers the public design GET,
not the new receipt checkout/status/upload routes. The live editor has planned
pricing, not bank/MMQR checkout controls. No functioning guest catalog/player or
timed paid stream was demonstrated. Local receipt/approval tests are not deployment.

Selected payment workflow is **owner bank/MMQR receipt upload and owner approval
through that business's own Telegram bot**, not an automated payment gateway.
Finish and verify the owner-PC relay, guest player, receipt decisions and expiry
before accepting real payment or enabling sales.

### P1 — Guest voucher issuance remains disabled

Anonymous `/api/v1/guest-vouchers/config` returns 404 and the signed-in Vouchers
page reports the disabled capability. There are no existing vouchers in this
workspace; the print preview consists of clearly labeled SAMPLE tiles. No new
guest access code can be issued through this live workflow. Redemption and quota
enforcement were not exercised.

### P2 — Wi-Fi configuration remains held

Router inventory loads, but the selected device page says Wi-Fi changes await
physical-radio acceptance. The deployed non-secret flag is
`MM_NETWORK_WIFI_CONTROLS_ENABLED=false`. The CHR is not a physical Wi-Fi device.
This is an intentional safety hold, not a newly reproduced HTTP failure.

### P2 — Media computer is disconnected and the connector is a terminal pilot

Connection shows Not connected for the saved Mac identity. The instructions
require a terminal running the connector and advertise a 24-hour session. Cloud
media functionality cannot reach the owner-PC library while disconnected.
Disconnect alone is not proof of a server bug. Unattended reconnect, renewal and
restart recovery are not certified.

### P2 — Portal configuration needs an owner review

The saved plan row is labeled 1GB but contains **1 minute, 1 MMK and 1 MB**. This
is confirmed saved form data, not an assertion that calculation code is broken.
The MMQR image URL is empty; bank details are present. Voucher and mobile-payment
methods are both selected. The loaded guest preview defaults to voucher login,
despite disabled new issuance and no existing codes in this workspace. Guests
can select the mobile-payment alternative; the voucher default is not evidence
that every guest is completely blocked. Review plan terms and method availability
before exposing this configuration to customers. Nothing was saved during audit.

## Positive checks, with explicit limits

- `/health`, `/app.html`, `/media.html`, portal HTML and the checked app/network/
  media JavaScript assets return 200. API/nginx active. An aggregate journal scan
  over the preceding 15 minutes found zero lines matching panic/fatal/runtime
  error/connection-refused/deadline-exceeded. This is not exhaustive monitoring.
- Overview/Fleet/Topology show one reachable managed CHR. Incidents has no open
  incidents for this workspace. Platform separately shows three open incidents
  in other businesses; this is not inconsistent with the workspace view.
- WAN inventory completes and exposes primary/backup or equal-cost sharing,
  actual interfaces and ISP gateways. Only ether1 is discovered as an Ethernet
  port, so this target cannot establish real dual-ISP acceptance.
- PPPoE inventory completes and exposes service, account and subscriber-plan
  tasks with suggested addresses, DNS and speed. Existing-network inventory
  completes with DHCP/pool/DNS/Hotspot/reservation/bandwidth choices; there are
  no existing DHCP service rows. Empty inventory is not automatically an error.
- Guided network setup loads interfaces and suggests `192.168.50.1/24`. Uplink
  and customer port choices remain explicit; the management WireGuard interface
  is not offered as a customer-facing LAN. No LAN was created.
- Captive Portal has gateway/profile/review deployment controls. Its iframe now
  renders an actual guest voucher/payment preview rather than the earlier DNS
  error screenshot. No deployment or guest internet access was tested.
- Backups lists an existing recovery point. Its history includes an older WebFig
  certificate-signing failure; that is historical, not a newly reproduced failure.
- Store/history, Revenue, Team, Events, AI usage, Translations, Sites, guide,
  Templates, scans, terminal log and Platform render. No changes were submitted.
- Live app and public media browser error/warning samples were empty.

## What this audit cannot certify

Healthy pages do not prove every button or business operation works. Not tested:
new-user onboarding; non-Developer permissions and tenant isolation; router
enrollment; real captive guest login; bank receipt approval and actual internet
grant; voucher issuance/redemption/expiry/data exhaustion; physical radio/SSID
changes; real ISP cable-cut failover and throughput; restore/reboot; paid-media
catalog/playback/seek/expiry/revocation; connector restart and renewal. No whole-
app security clearance or production-complete claim is justified.

## Audit-tool check (not a live defect)

`node scripts/test-audit-fixes.mjs` passes locally. The static
`scripts/audit-web-ui.mjs` flags two new local media buttons because it only
searches inline script references and does not follow the external guest module.
Those buttons have handlers in local `media-guest.js`; the diagnostic is a tool
coverage gap, not evidence of a broken deployed button. Those new buttons are
not present in production. This audit did not change application source to silence it.

## Acceptance order

1. Verify the saved portal plans/access methods with the owner.
2. Complete paid-media relay/player and local connector lifecycle, then test the
   selected manual bank/MMQR approval workflow end-to-end with disposable fixtures.
3. Complete voucher and physical Wi-Fi acceptance before enabling those features.
4. Run scoped, authorized write-path acceptance on an appropriate disposable
   router and guest network; preserve backup, readback and rollback evidence.

Audit only: no fixes or live mutations were made during this re-audit.
