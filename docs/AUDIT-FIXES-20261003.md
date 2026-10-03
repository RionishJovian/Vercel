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
