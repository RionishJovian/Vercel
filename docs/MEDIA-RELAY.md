# Owner-PC catalog and playback relay

## Superseded architecture — do not deploy for video

The owner confirmed on 2026-10-04 that playback is only for guests on the
business's RouterBOARD network. Movie bytes must remain on that local network.
The historical implementation and tests below describe the rejected cloud
video relay, not the target architecture or current source behavior. Source
now rejects cloud stream requests and video uploads; only catalog metadata can
use this transport. Local playback and its network/paid-access acceptance are
still required before offering paid purchases. Hostinger migration is recorded
in HANDOVER; the unavailable Z.com hosting notes below are historical.

## Source implementation, 2026-10-04 — not deployed

The owner selects up to sixteen playable Jellyfin libraries in the appearance
editor and publishes that selection. No library is shared automatically. The
existing local adapter verifies item ancestry before opening a stream. Its
Jellyfin API key stays on the owner PC, not in guest URLs or cloud tasks.

Four independent connector workers poll for guest requests without blocking
owner inventory jobs. Catalog results and media bytes travel over authenticated
outbound WebSockets using `github.com/coder/websocket` v1.8.15. This is not an
arbitrary URL proxy. A request is leased once to its business and connector
credential. The process-local broker caps requests at four per business and
sixty-four globally; it fails closed on restart. Catalogs contain at most one
hundred path-free typed items per page.

Checkout now checks that a selected library is available before soliciting a
bank transfer. Receipt upload still cannot grant access: the business's guarded
Telegram approve/reject decision settles the pass. The approved guest exchanges
their bearer proof for a separate, publication/pass-path-scoped HttpOnly,
SameSite=Strict playback cookie. Staff cookies are ignored; neither the guest
proof nor Jellyfin key appears in the video URL. Production must use HTTPS.
The cookie signature is process-local and invalidated by an API restart; the
guest can load the library again to obtain a new session without repurchasing.

Browsing does not start paid time. Activation happens only after the PC has
authorized the file and supplied valid stream/range metadata. An offline PC or
rejected local file does not activate the pass. The ledger starts the absolute
clock once; subsequent seeks cannot renew it. The cloud and PC recheck access
within five seconds, with three-second authorization limits. Expiry closes an
already-open stream. Refund, tenant suspension, unpublish/library removal and
connector revocation fail closed. Cloud socket frames are bounded to 64 KiB;
media response writes have bounded deadlines to interrupt a stalled guest.

Direct static streaming is implemented, including single byte-range seeks. This
does not certify every browser/codec or provide transcoding/DRM. Previously
downloaded bytes cannot be recalled. Traffic traverses the app relay, so account
for PC uplink, app bandwidth and hosting policy; do not silently route movies
through a web-only/free CDN plan. Cloudflare's current Tunnel/CDN guidance
requires appropriate services for video delivery:
https://developers.cloudflare.com/fundamentals/reference/policies-compliances/delivering-videos-with-cloudflare/

## Verification in this continuation

Actual localhost HTTP/WebSocket tests joined the typed Jellyfin adapter, paired
PC runner, cloud relay and guest player endpoints. They verified catalog
browsing without activation, complete byte transfer, seek offsets, one-time
activation, cross-library and cross-publication denial, staff-proof denial,
refund denial and deadline interruption during an open stream. Three race runs
passed before the WebSocket conversion; the WebSocket acceptance and affected
media/handler/server/auth/configuration/Telegram race suites passed afterward.
DOM regressions verified explicit owner-library selection/publish/reload,
safe library/title rendering, receipt review status, session/catalog/player
integration, secret-free media URLs and UI stopping on refund. These used
synthetic records, a mock local Jellyfin API and no real payment/bot messages.

Native browser decoding against the owner's authenticated Jellyfin installation
is still an acceptance boundary, not a claimed result. This continuation did
not read a real Jellyfin credential, alter a library or re-run PostgreSQL tests
against a production copy; the preceding source review covers ledger integration.

## Hosting and rollout prerequisites

The user reported both VPS instances suspended due to billing. Two read-only
SSH attempts to the app timed out. No production deployment, database restore,
DNS change, tunnel installation, router change or VPS stash operation occurred.
Screenshot claims that backups are complete are not restore verification.

Before enabling paid sales on a chosen host:

1. Verify the selected backup by restoring into an isolated database; preserve
   the original and verify the application schema/data boundary.
2. Apply the additive paid-pass migration explicitly after the backup/rollback
   check. Keep the feature flag off until acceptance succeeds.
3. Forward WebSocket Upgrade/Connection headers on
   `/api/v1/media-connector/relay/:task/socket`; preserve Authorization, reject
   browser Origin, and allow the native stream response without proxy buffering.
   Send a CSP permitting same-origin media. Keep the existing edge hardening.
4. Pair an owner-local PC and select a disposable, permitted test library.
   Verify native decoding, seeking, receipt approval/rejection, expiry and PC/API
   disconnect/re-pair behavior with synthetic receipts, not customer payments.
5. Enable paid access only on the accepted deployment and verify public checkout
   and that business's real Telegram binding without charging a customer.

Code rollback disables paid access and restores the previous binary/frontend.
Do not restore an old database over issued passes. A laptop-hosting migration
or public DNS change needs separate user authorization. A web tunnel does not
replace the existing MikroTik UDP/WireGuard hub.
