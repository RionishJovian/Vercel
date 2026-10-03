# Magic Media paid access — owner-PC architecture

## 2026-10-04 review evidence

Reviewed the source changes against the production handover. Corrected the
checkout idempotency boundary so an old page's checkout key cannot return or
rebind an order after unpublishing and republishing. Three fresh PostgreSQL 16
race runs passed all bridge integration tests using only synthetic fixtures on
127.0.0.1:55436. Scoped media/handler/server/configuration/Telegram race tests and
vet passed. A new guest receipt DOM regression passed receiving terms, omitted
staff cookies, private upload, delivery retry, approval refresh and refund.
This review did not rerun real Chromium or signed-in local Jellyfin acceptance.
No production table, checkout/player rollout, real payment or bot delivery was
performed. The guest catalog/playback relay still needs implementation.

Each business owner runs Jellyfin on their own PC. The outbound connector pairs
that PC to their business account. Neither the cloud API nor a guest chooses a
central movie server. Jellyfin administrator credentials stay on the owner's PC.

## Payment workflow clarified by the owner

There is no external automated payment gateway. Use the existing portal-style
bank/MMQR workflow: the business publishes its own QR image or bank name,
account number and account-holder name; the guest transfers money and uploads
a successful receipt; that business's own Telegram bot delivers the receipt
with Approve/Reject controls. Only its authorized owner decision grants the
paid pass. Uploading an image alone never grants access. This clarification
supersedes the earlier automated-provider assumption.

The source ledger now supports immutable receipt submission and serialized
owner decisions. Review IDs are UUIDs that fit Telegram callback data limits;
private receipt paths and hashes are not returned to guests. Receipt arrival
must meet the upload deadline, but owner review may happen later. Concurrent
approvals are idempotent, conflicting decisions fail, and the same image cannot
approve two orders in one business. This hash check is not bank verification
and cannot detect altered images; the owner must inspect the actual receipt.
Approval leaves the playback clock unstarted until first authorized activation.
Refund/rejection cannot be reversed by a stale approval tap.

The owner-bot supervisor shares one guarded update stream for Wi-Fi and media
decisions, not the platform billing bot. New `SendReceipt` transport attaches
the image as a private Telegram document with decision buttons; it does not
create a public receipt URL. Transport tests used a mock Telegram server, not
real customer messages. Guest checkout/status/upload HTTP routes and the bank/MMQR
editor now call these boundaries in source; no production table or sales feature
was enabled.
Three isolated PostgreSQL race runs passed manual approval, duplicate evidence,
late upload, tenant isolation, rejection/refund and clock-start checks. The
temporary test container was removed; only synthetic records were used.

## Current source work (2026-10-03, not a deployed checkout/player)

The new `magic_media_passes` additive migration and bridge methods implement:

- Server-side snapshots of the published MMK price and duration. Commerce accepts
  one hour through thirty days; an unsupported appearance duration is rejected,
  never silently truncated. A pass does not mutate when the owner edits pricing.
- Guest-bound hashed credentials and a random idempotency key. Checkout retries
  return the same order and deadlines, not additional access.
- A provider-internal settlement boundary: correct amount/currency and a unique
  provider reference are required. It must only be called after provider-specific
  signature, merchant-account and actual-settlement verification. A browser
  success URL, receipt upload or guest-supplied JSON is not payment proof.
- Serialized settlement and activation. Payment does not start the clock;
  first authorized playback starts it exactly once. Subsequent activation does
  not extend expiry. Refund/rejection and suspension deny access.
- Database authorization checks the absolute expiry on every request; no expiry
  worker or browser countdown is trusted. Late payments need reconciliation or
  refund rather than automatic access after an abandoned checkout deadline.
- Forced tenant RLS, indexed tenant lookups and no external calls within database
  transactions. A provider reference cannot settle two different passes.

`internal/media/access.Copy` is the owner-PC streaming enforcement primitive.
It authorizes before opening a local source, bounds authorization checks to
three seconds, rechecks at most every five seconds, and closes an open source at
expiry, revocation or cloud-authorization failure. The source must honor context
cancellation and unblock reads on Close (HTTP response bodies do). The absolute
deadline is never refreshed by a successful check. Already downloaded/buffered
content cannot be recalled; this is access control, not DRM.

Checkout/status/receipt HTTP routes now use the ledger. Playback enforcement is
not currently invoked by the deployed connector or public guest page. This is
not evidence that catalog transport or playback integration is finished. The
new migration has NOT been applied to production. No real payment was accepted,
existing Jellyfin user altered, existing library changed or sales flag enabled.

The connector now has an internal `Runner.StreamMedia` boundary joining the
typed Jellyfin stream to timed-access enforcement. It requires a paired runner,
an authorization check and an absolute ledger deadline; safe range/type metadata
is supplied to the relay before any bytes are copied. Denial never opens a local
source, relay failure closes it, and expiry interrupts a blocked read. Connector
tests cover these behaviors without changing libraries. The cloud outbound relay
and public player do not call it yet; this is local integration, not deployed
guest playback. Focused access/connector/Jellyfin race tests and vet passed.

The local Jellyfin adapter now has bounded, path-free catalog pages and typed
direct streams. Fixed item/library IDs are validated; stream access checks real
ancestor membership before opening the item. Credentials remain in headers on
the owner PC, never guest URLs; redirects and arbitrary paths are refused.
Single byte ranges and upstream partial-response metadata are checked, including
matching the returned offsets to the requested seek and clipping at EOF. Streams
require an absolute deadline and have a bounded response-header wait, rather
than the metadata client's fifteen-second total timeout. Direct streaming does
not certify every browser/container/codec or transcoding combination.

## Verification

Full Go tests, focused race tests and vet passed. The new PostgreSQL acceptance
test ran three consecutive times with race detection on a loopback-only,
disposable PostgreSQL 16 container using synthetic accounts and payment events.
It verified concurrent settlement/activation, immutable commercial terms,
reference replay, refund, expiry, suspension and tenant RLS. Streaming tests
verified denial before source open, ordinary transfer, deadline interruption of
a blocked read, and revocation/cloud-outage interruption. The database tests do
not certify a real payment-provider webhook or Jellyfin stream.
Local adapter race tests verified library scoping, bounded catalog output without
source paths, byte-range transfer, rejected cross-library requests, malformed
ranges and credential-bearing redirects. These used a synthetic HTTP server.

The approved Mac test installation was started and its public endpoint verified:
Jellyfin 12.1.0, `http://127.0.0.1:8096`, initial setup complete. The owner signed
in locally. The initially empty server now has one clearly labelled
`Magic Media — disposable acceptance` Home Videos library, containing only a
generated twelve-second H.264/AAC clip under
`/private/tmp/mm-media-fixture.8mjZpk`. No existing credential was read or reset.
The Jellyfin web client reported a decoded 320x180 frame, readyState 4 and an
advancing start position; an "Unable to play media" notification also appeared.
On the fresh signed-in retry, the web client again reported "Unable to play
media" with readyState 0 and no decoded frame. Sustained native playback and
Magic Media's timed playback are NOT certified by these observations.
The local server's own OpenAPI contract was read for the new
Items/Ancestors and static-stream endpoints; no admin key was copied to chat.

## Required continuation

The owner clarified bank/MMQR receipt review through each business's own bot.
No automated provider name, merchant account or gateway API key is required.
Existing provider-settlement primitives are not the selected customer workflow.
Reuse the portal's tenant-owned bot binding and private receipt storage. Wire
Owner bank/QR settings, guest-bound checkout/status/upload and receipt delivery
are now implemented, including explicit retry/status if Telegram is unavailable.
Receiving details are snapshotted with price/duration; editing the published bank
account does not change an existing purchase. Guest requests omit staff cookies.
Receipt submission alone cannot approve or activate a pass. Synthetic browser
checks covered publish/reload/conflict, checkout/upload, failed Telegram delivery,
approval status/reload, cookie isolation and unpublishing. Three isolated database
race runs also passed the new bank snapshot and guest-bound lookup assertions.
The private temporary PostgreSQL container was removed after the tests.
Do not accept
real payments before a working owner-PC playback path is verified. Payment and
playback acceptance uses disposable fixtures and synthetic receipts; do not
charge a real customer as a test.

Next wire catalog and outbound
owner-PC playback transport to these boundaries. Verify failed/duplicate/late
payments and refunds end-to-end, real media seeking and expiry while streaming,
connector disconnect/restart and tenant isolation. Only then request the scoped
production backup/migration/deployment and verify the real guest workflow.
