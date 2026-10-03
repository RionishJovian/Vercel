# Confirmed audit bug fixes — 2026-10-03

Scope: the defects from LIVE-APP-AUDIT-20261003.md, not enabling unaccepted product workflows.

- Payment and voucher status labels use their own enumeration rather than connectivity badges.
- Fleet/Topology describe management reachability, mapped relationships and per-device transport; neither invents a verified WireGuard handshake from an online management signal.
- Security uses published stable/long-term/testing fixes, installed release branch before selected update channel, and beta/rc ordering. Capture, live Security and Magic Dude use the same resolution. Unknown channels recommend stable. No device upgrade or mitigation is performed.
- The guide includes WAN, PPPoE, Wi-Fi and existing networks and accurately scopes templates and vouchers.
- Voucher UI distinguishes unavailable issuance and failed list reads from a genuinely empty list. This does not enable issuance or certify guest access.

Sources for security bounds: https://mikrotik.com/supportsec/september-2026-vulnerability/ and https://mikrotik.com/supportsec/cve-2026-52346/.

Acceptance: dedicated release-track/prerelease tests and UI regression checks; broader test/deployment status recorded below when verified. No database migration is required. Wi-Fi remains held, and paid media/connector integration remains separate unfinished work.

Validation: full `go test ./...` passed with local mock-server networking allowed;
focused security/handler/monitoring race tests and both audit/network workspace
JavaScript regression scripts passed. The restricted first full-suite attempt
could not bind mock loopback servers; it was rerun, not counted as a product bug.
No production database integration test or customer-router write was performed.

## Production rollout

User approved push, backup and live rollout. Code commit `91f6ae3` was pushed to
GitHub main and fast-forwarded on the clean VPS checkout. Canonical
`./deploy.sh all` completed its production config/schema checks, rebuilt and
restarted the API, published versioned frontend assets and reloaded nginx.
Landing/app/health returned 200; API/nginx are active. Shared app.js source and
installed asset SHA-256 matched. Backup: `/home/ubuntu/mm-audit-fixes.PUV3xT`
(previous API binary, compressed web tree and previous source commit).

Signed-in verification: Revenue displayed two Approved and four Rejected rows;
Security recommended 7.24.2 for the stable 7.15.3 CHR; Topology displayed
Management reachable and Mapped relationships instead of an invented active
tunnel; Vouchers explained the unavailable deployment; Product guide included
the four network workspaces. The unavailable voucher route remains held.

No database migration or router/payment write was performed. Production flags
remain network controls true and Wi-Fi controls false. Uncommitted paid-media
foundations were excluded. This verifies these defects, not whole-app readiness.
