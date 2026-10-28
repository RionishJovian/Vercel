# Router inventory read correction — 29 October 2026

The selected `Neighborhood RouterBOARD — Test` failed both Router overview and
network inventory because its own saved management target was absent. A
read-only production query confirmed no management IP, SSH username, encrypted
credential or pinned host key. Fresh Hub lookup and kernel WireGuard checks
reported no handshake and zero received/transmitted bytes for its allocated
peer. The owner reports the setup script was run. Pending setup records expire
on October 24/25; they do not establish successful router authentication.

The API now returns HTTP 409 with `router_management_required` for missing,
incomplete or unpinned saved targets after the existing tenant/device check.
Configured transport failures still return a sanitized 502; outbound policy,
host-key checks and network write gates remain enforced. Raw connection errors
are never returned. No telemetry ingestion behavior changes in this release.

Network settings and Router overview show the management prerequisite, offer a
read-only retry, and open the selected router's existing Connect Magic Hub
section. Opening that section does not generate a script or apply router
settings. Both changed scripts have new cache versions. Inventory remains
unavailable until the tunnel and saved management connection are verified.

Verification: affected Go packages and full Go suite passed; all 110 critical UI
tests passed, including new missing-connection, exact-device setup navigation,
read retry, no-write and escaped-error checks. The device/topology checks,
production frontend build and six-surface web audit passed. A private production
preflight uses the real runtime role and database in read-only mode to exercise
both 409 responses and a foreign-tenant 404, with zero transport attempts. The
existing runtime privilege/schema preflight also passed.

Deployment is additive to the previous VPS API source and uses the complete
matching local production frontend. It preserves runtime configuration,
security headers and all data containers. Rollback images and release evidence
are retained privately. Packaging verification initially rejected a candidate
binary permission mismatch and restored the prior release; the corrected image
is verified as readable/executable by the application user before activation.
This correction does not certify physical router connectivity.

The successful release uses `mega-magic-api:inventory-read-fix-20261029` and
`mega-magic-web:inventory-read-fix-20261029`. API executable SHA-256:
`1b8a7e5014e732872aa2d40ac28e7aebb014a0fb2081531b9abf46c7bf7cff24`.
All 55 frontend files matched the running container and HTTP responses; changed
assets and HTML also matched through public HTTPS. DB/Redis/NATS container IDs
and configuration hashes were unchanged. The VPS WireGuard listener is on
UDP 51820 with an allowing firewall rule; no successful handshake is recorded
for the selected peer. Further router-side diagnosis needs its current online
state and the result of running the setup script; do not assume the tunnel is
working solely because a script was pasted.
