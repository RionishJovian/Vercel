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

## Verified release and physical installation

Live API: `mega-magic-api:portal-verified-20261029`.
Live frontend: `mega-magic-web:portal-utf8-20261029`.
All 67 frontend files matched local source, container and HTTP; changed assets
also matched HTTPS. The API binary hash is
`b01934522dda743fad8dc169a78c05f8e49a5a927b4a57ae09468e16dbc41e95`.
Readiness passed. Data containers, deployment configuration, encrypted router
credentials, WireGuard allocations and 19 existing voucher identity/allowance
snapshots were preserved. Rollback copies remain on the VPS.

The owner explicitly authorized the portal Apply step for this installation.
The saved design was installed on hAP ax2 / `mm-lab-hsprof` at 16:20 Yangon time
on 29 October. The deployment verified all 18,553 UTF-8 HTML bytes, the scoped
TCP 443 rule and the active profile directory. The app recorded the operation.
The active file is `mm-portal-e4db84599f1f0563/login.html`; its SHA-256 is
`b279d2f89345e096fa45f46de73ea22922f0bd162ccd18eff8617e696ca79d55`.
The verified original-page backup is
`mm-portal-e4db84599f1f0563/login-backup-e4db84599f1f0563.html`.
The original `mm-local-lab-20261022` directory was retained.

Live testing exposed three provider-format issues that the earlier fixtures did
not model: RouterOS array concatenation repeated the field label; raw multiline
SSH strings failed to parse; and unescaped UTF-8 input silently lost glyphs.
Scalar conversion and ASCII commands containing explicit UTF-8 byte escapes now
preserve the exact page and script bytes. Parser errors are detected without
returning credential-bearing provider output to API callers.

The IP allow-list has no `dynamic` field. The old predicate hid existing rules
and made its cleanup check incorrectly report success. Five identical entries
from failed attempts were checked by exact IDs, comment, host, port, protocol,
server, action and enabled state, then removed; zero remaining was verified and
audited. The corrected apply installed one verified owned entry. Previous failure
audit records were retained. The original portal profile stayed active during the
failed attempts. Other allow-list entries were not changed. See the official
[IP allow-list reference](https://manual.mikrotik.com/docs/cli-reference/ip/hotspot/walled-garden/ip/)
and [HotSpot documentation](https://manual.mikrotik.com/docs/authentication-authorization-accounting/hotspot-captive-portal/).

Checks passed: full Go suite, 887 checks against disposable PostgreSQL with the
race detector (11 environment-gated skips), 127 UI checks, the production frontend
build and device/site continuity checks. All eight read-only observation sources
were available on the physical hAP ax2. The diagram was visually checked using an
actual router snapshot. A requested viewport override did not take effect, so
visual verification used the actual browser width rather than claiming a 390px
phone test. The deployed guest design was visually checked in an inert local
preview backed by the verified saved design. Browser access to the live app was
blocked by the default network path; HTTPS checks succeeded over the working
interface and on the VPS.

A guest login has not been consumed or certified. The last readiness read showed
the `Magic Cafe` guest radio inactive and zero Mega Magic voucher users installed.
Portal installation does not configure that radio or install a voucher batch.
Those remain separate operations with their existing review and apply gates.

The changed portal/map backend files were reconciled against exact deployed
preimages. Pre-existing route/field ordering and platform business-listing DTO
differences were retained outside this task. This record does not claim all
backend source files or local/production database contents are identical.
