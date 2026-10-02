# Read-only Magic Hub lookup rollout

## Current delivery state (2026-10-01)

Implemented and tested locally. No database migration required. The read-only
Hub GET extension and app API are now deployed. Hub signed live checks passed
(existing peer, tenant denial, router mismatch, scope denial, unsigned denial).
Peer-store and WireGuard allowed-IP hashes were unchanged after deployment.
App config/schema checks passed; API, landing and app health checks returned 200.

**Enabled and transport-verified:** After the initial approval-review block,
the owner explicitly authorized storing the existing Hub signer on
`mikromagic-app`. Transfer completed directly between encrypted SSH pipes,
without displaying the key. `/etc/mega-magic-hub-lookup.env` is root:root 0600;
systemd drop-in `97-magic-hub-lookup.conf` loads it. Lookup is enabled and the
API was restarted successfully. The actual Go client on the app host passed
verified HTTPS lookup of an existing peer, exact identity checks and cross-tenant
denial. Public health returned 200; anonymous live-lookup requests returned 401.

The existing signer can authorize mutating peer operations too: do not expose
it in UI/logs or interpret this approval as authorization to enroll/remove
customer peers. The Gateway UI now includes a read-only **Check Magic Hub**
button for OWNER/ADMIN/DEVELOPER gateways only (not downstream devices/viewers).
It renders actual allocation/handshake data, safely escapes response text,
prevents duplicate clicks and ignores detached-page results. Friendly errors
distinguish required pass, disabled lookup and lookup failure. It does not claim
LAN readiness or enable enrollment.

UI regression tests and the production-source frontend audit passed. Frontend
is deployed (redesign CSS hash `da74f601`). Browser acceptance used Nish's
existing DEVELOPER session and the registered CHR lab gateway. The button showed
`An active Sovereign pass is required to check Magic Hub.` This verifies the
authenticated denied path only: the existing subscription gate prevents this
account from reaching the Hub lookup. No pass was purchased/granted and no gate
was bypassed. Successful authenticated UI lookup remains unverified. Deciding
whether read-only status should be exempt from the paid pass requires owner
direction, not an agent silently changing billing policy.

Frontend backup: `/home/ubuntu/deploy-backups/hub-ui-20261001.D4gfJ5lx/` holds
prior source files and published web tree. Local screenshot evidence:
`/private/tmp/magic-hub-gateway-check-20261001.jpg`.
LAN traffic collection and temporary LAN sessions remain unfinished. The CLI
diagnostic tests operator transport directly, not browser acceptance success.

Backups: Hub `/opt/mikromagic-hub/backups/lookup-20261001-005442/service.mjs`;
app `/home/ubuntu/deploy-backups/hub-lookup-20261001.23o6EOT2/` includes the prior
binary and the three changed existing source files. Both services are active.

The app exposes authenticated `GET /api/v1/vps/peers/:deviceId/live` (check the
server's API prefix when deploying). It requires an owner/admin-level role, an
active Sovereign pass, and a tenant-owned `MAIN_GATEWAY` device. It does not
enroll peers or change router settings. Handshake freshness is not LAN readiness.

For future installations, keep `VPS_HUB_LOOKUP_ENABLED=false` until explicitly
approved app credentials are installed and app-to-Hub verification succeeds.
The existing PUT route must **never** be
used as a lookup fallback. The required read-only Hub extension must:

- Require `peers:inspect`, with the URL router ID matching the signed router ID.
- Find an existing non-removed peer belonging to the signed tenant and router.
- Return `publicPeer(peer)` or 404; never allocate an address or call
  `wgSyncPeer`, `savePeers`, or any relay/firewall mutation.
- Reject cross-tenant requests and wrong scopes. Test with an isolated peer
store and mocked WireGuard reads; do not run the production HMAC self-test.

The staged `scripts/hub-read-only-lookup.patch` targets the inspected Node
service. Verify its context against the current source before applying it;
it is not a replacement for isolated contract tests or a production backup.
`test-hub-lookup.mjs` tested the inspected service in an isolated VM with fixture
credentials/peer data, forbidden writes/network and mocked WireGuard reads.
`verify-hub-lookup.mjs` performed read-only live checks on the Hub; credentials
stayed inside that process and were not displayed.

Configure the app through protected service environment settings:
`VPS_HUB_URL`, `VPS_HUB_KEY_ID`, `VPS_HUB_SIGNING_KEY` (canonical 32-byte hex).
Do not paste the key into chat, source control, shell command arguments or UI.
Then opt in with `VPS_HUB_LOOKUP_ENABLED=true`. HTTPS verification stays enabled.

Deployment checks: back up exact Hub source and app release; deploy and test
the read-only Hub route before enabling the app; confirm tenant-denied cases,
missing peers, stale handshakes and upstream failure responses; confirm peer
store/WireGuard configuration are unchanged. Roll back the app flag first if
inspection fails. Peer enrollment, general LAN access and Traffic Flow collection
remain separate work and are not enabled by this rollout.
