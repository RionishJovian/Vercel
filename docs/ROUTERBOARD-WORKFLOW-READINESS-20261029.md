# RouterBOARD workflow readiness — 29 October 2026

The app-side flow is site and gateway selection, consented one-paste generation,
local paste on RouterOS 7, Finish connection, encrypted tenant-bound credential
storage, then read-only inventory and intentionally reviewed configuration.
This release prepares that flow for owner-operated hardware testing. It does
not claim a working physical tunnel, guest internet or radio acceptance.

## Corrections

- Inventory and health distinguish missing management setup from transport
  failures, with a direct connection action and read-only retry.
- Expired original installation keys remain inaccessible. Normal setup still
  returns a recovery hold without replacing the Hub peer or router key. An
  explicit Repair installed connection checkbox permits a separate repair
  script for a tunnel already installed on that RouterBOARD. Its interface,
  ownership, router public key, endpoint, address and routes must match; it
  stops before management changes if those checks fail. Repair cannot create a
  new Hub enrollment. The response marks `one_paste: false` and
  `existing_tunnel_required: true`, and the frontend requires repair consent.
- Repeat-paste validation compares RouterOS array output with semicolons;
  creation still uses comma-separated allowed addresses. This matches the
  documented `:tostr` array format rather than rejecting a valid multi-route
  peer on its second paste.
- Successful Finish connection refreshes the saved management address and
  capability buttons without resetting an open settings editor. A failed
  refresh retains the authenticated result and asks for a page refresh.

## Evidence

The full Go suite ran against a newly initialized disposable PostgreSQL 17
cluster: 1,477 test/subtest passes, zero failures. Eighteen tests skipped:
eight physical RouterOS/CHR checks, two unrelated opt-in live pricing checks,
and eight separate media-bridge database checks. Credential round trips, wrong
cipher rejection, tenant isolation, encrypted pending enrollment, coordinator
restart, expired-key hold and explicit recovery ran against real test tables.
The cluster was shut down after the suite.

All 110 critical UI tests passed, including enrollment consent, duplicate
submission prevention, safe script rendering, repair-mode consent and verified
connection refresh. The eight frontend proxy tests, two deployment transforms,
six-surface web audit, device/topology, guest-device and Hub reservation tests
also passed.

A read-only snapshot of the currently deployed Hub program was tested in an
isolated JavaScript VM with mocked storage/WireGuard. Signed enrollment,
lookup, retry, tenant/scope denial, reservations and management migration
passed. Its enrollment/migration modules match the repository files exactly.
No real peer store or kernel peer was changed by those tests.

The VPS signed lookup works. UDP 51820 is listening and allowed through the
firewall. The host route uses management source `10.77.0.1`; the API container
routes through its bridge with Docker masquerading, matching the script's
private `/32` restriction and the configured management allowlist. Actual
packet delivery/authentication from the selected RouterBOARD remains unverified.

## Owner's test flow

1. Open Magic Hub and confirm the intended site and RouterBOARD record.
2. Open Connect Magic Hub, authorize setup and generate a script immediately
   before using it; management setup expires after 30 minutes.
3. If the app reports that the original installation script expired, choose
   Repair installed connection only when this RouterBOARD already contains its
   original Magic Hub tunnel, then generate its repair script. No replacement
   key is silently installed. A missing or mismatched interface needs a separate
   deliberate recovery decision.
4. Paste the entire script in WinBox New Terminal. It must end with the setup
   completion message. A STOP/error is a failed setup, not success.
5. Select Finish connection. It requires a recent handshake, SSH key login,
   matching enrollment proof and router key, RouterOS identity, and successful
   encrypted credential storage. A handshake alone is insufficient.
6. Read Router overview and the relevant settings. Review and confirm each
   intended change; verify the reported readback and backup. Guest internet,
   voucher first-use, captive portal and throughput need real client checks.

Wi-Fi editing retains its separate physical-radio acceptance hold. Automatic
RouterOS/firmware updates are not implemented. No changes to these availability
limits, account passes, current router settings, stored customer data or Hub
peer identities are made by this release.

References: [RouterOS array formatting](https://help.mikrotik.com/docs/spaces/ROS/pages/283574370/Scripting+Tips+and+Tricks),
[WireGuard peer fields](https://help.mikrotik.com/docs/spaces/ROS/pages/69664792/WireGuard).

## Activated release

API and web images: `mega-magic-api:router-workflow-ready-20261029` and
`mega-magic-web:router-workflow-ready-20261029`. API executable SHA-256:
`17822885752a1239cda09398e002898ae69d85866552a929da9abaa3c2082e96`.
All 55 frontend files match local source, container files and served HTTP bytes;
HTML and changed scripts also match through public HTTPS. Dependency readiness
is healthy. Data container identities and runtime/security configuration are
preserved. Protected rollback images and manifests remain on the VPS.
