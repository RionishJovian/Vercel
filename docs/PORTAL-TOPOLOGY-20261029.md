# Router portal and observed topology — 29 October 2026

The production app retains the existing Go API and Jade Glass frontend.

Portal deployment discovers only profiles attached to exactly one enabled
HotSpot server. Fixed getter output replaces RouterOS's presentation-oriented
print output. A single gateway and eligible profile are selected automatically;
review and explicit apply remain required. Review checks the saved design,
CHAP helper, storage, original login page and backup/staging plan. It does not
change the router. Successful file readback is distinct from a guest login test.

The site map uses the persisted device site assignment and pinned, tenant-scoped
read-only router observations. Router own interfaces and multicast addresses are
excluded. MAC identity merges DHCP, ARP, neighbour, bridge forwarding, HotSpot and
Wi-Fi observations. Station-mode Wi-Fi peers are uplinks, not guest clients.

A solid connection records the router's reported Wi-Fi association, neighbour or
bridge forwarding port. An unseen downstream switch may share a forwarding port;
the map does not invent physical hops. DHCP and ARP alone use dashed unknown
attachments and do not establish online status. Unread menus are reported,
not converted into an empty successful inventory. Refresh replaces the snapshot;
late responses cannot overwrite a changed site/router context. Reads never enable
configuration writes. Friendly categories remain saved metadata.

The hAP ax2 photo comes from MikroTik's official product page. Generic device
symbols are Phosphor library icons, not photographs of unidentified hardware.
Asset provenance and the Phosphor MIT licence ship with the local image bundle.
The Ruijie topology guide informed device-image nodes, port labels and the
clickable inspector: https://jacs.ruijienetworks.com/wiki2/article/72 .

Verification and live release evidence are recorded separately; this document
alone does not certify current router connectivity or installed portal state.

## Verified release

Live immutable images: `mega-magic-api:portal-topology-20261029` and
`mega-magic-web:portal-topology-20261029`. All 67 frontend files matched the
local release, container and served HTTP bytes; changed frontend assets also
matched public HTTPS. Readiness passed. Existing data containers, configuration,
router credentials, WireGuard peer allocations and 19 issued voucher identity /
allowance snapshots were preserved. Rollback copies remain on the VPS.

Local checks: full Go suite, 882 passing checks against a disposable PostgreSQL
database with the race detector (11 environment-gated skips), 126 UI checks,
production frontend build and the existing device/site continuity checks.
The new diagram and inspector were visually checked in a local browser fixture.
The viewport override did not take effect; visual verification used the actual
browser width, rather than claiming a 390px device test.

The read-only production probe confirmed the gateway's saved site is returned,
but its pinned management connection timed out. The VPS's allocated hAP ax2
peer had no handshake for roughly 3.5 hours and its SSH port was unreachable.
Live profile discovery, portal review and actual guest acceptance remain
unverified until that router reconnects. No portal was installed by this task.
The browser's default network route also closed live app connections; HTTPS
release verification succeeded over the working interface and on the VPS.

The release updated the portal/map backend files against their exact deployed
preimages. Pre-existing server route/field ordering and platform business-listing
DTO differences were retained outside this task. This is not a claim that all
backend source files or local/production database contents are identical.
