# Live management-panel deployment — 2026-10-24

> **Note (2026-11-01):** the old console this document describes (`web/app.html`, the vanilla `web/v2` and their scripts) was removed. The console is now the React app in `app/`, served at `/v2/`. This is a dated record; file names and UI steps below may no longer exist.

Release `88d0dfc` (including `d456a21`) deployed and enabled on srv2032937 at approximately 12:00 UTC. Authorized by the owner's “deploy and enable live” instruction.

## Provenance

- Independent Linux AMD64 VPS rebuild exactly matched the local reviewed build: `cfec1a54aab423ed395fdb51bac9ea46903a6700c01356f83b60b983f0e751d5`.
- API image `mega-magic-api:88d0dfc`: `sha256:df1b62ff14360e23a32e9ed5ffbee8460fab87f6172aee0221f4a9b5516891a3`.
- Web image `mega-magic-web:88d0dfc`: `sha256:7a82f06f38392b2ddc948b71d71e97654b8c1cc8c69e7cfd0fcd5b9f1cf9987c`.
- Mechanically staged Hub service SHA256: `76926def47eede930f0f9adee13f46cdee29fb4368e210548ad78f40617f1d77`.
- Hub migration module SHA256: `97346f0d3cddd4df51c872987454721e21264af653559a73a723f6911669f34b`.

## Activation and validation

Applied additive migration `20261024070000_management_address_migration.sql`. Enabled `MM_MANAGEMENT_MIGRATION_ENABLED=true` in API Compose environment and `MM_HUB_MANAGEMENT_MIGRATION_ENABLED=true` in the existing private Hub environment. Existing general network-control flag stayed enabled. The separate Wi-Fi write flag remained unchanged/restricted.

The staged signed-route integration and Hub state-machine tests passed on the VPS without live router writes. Hub signed readonly `inspect` returned HTTP 200, phase `none`. API readiness reported database, Redis and NATS OK; web container became healthy; Hub systemd unit remained active. No fatal/panic/error lines were found in the post-rollout API log check.

All five public frontend files matched source byte-for-byte: app.html, layout.css, network-control.js, hub-management.js and management-migration.js. Unauthenticated migration-status and network-overview requests returned HTTP 401. The existing signed-in platform account showed all ten task categories and “Saved migration status loaded” with the preparation button. No preview/apply/recover was submitted.

Migration journal count remained zero. Peer-store bytes and WireGuard allowed-IP routes matched pre-deployment snapshots. Individual-pass rollout timestamp, seven-day grace records, enforcement flag and pass count matched their pre-deployment snapshot exactly. No credentials, account roles, payments or router configuration were changed.

## Backup and rollback

Validated private backup: `/root/mm-rollout-backups/management-88d0dfc-20261024T115628Z`. Includes PostgreSQL custom dump validated by pg_restore listing, image IDs, binary, original changed web files, Compose/config/environment, Hub service/environment and peer store. Release scripts/logs reside in `/root/mm-management-88d0dfc`.

Rollback uses the captured API/web images, Compose and binary; retain additive schema and customer history. Never disable individual passes or reset grace dates as part of rollback. Do not restore the original Hub service while any app job is nonterminal or any Hub migration is staged/committed: recover/finish with the compatible coordinator first. Do not restore the database dump over newer activity. Original API was `mega-magic-api:0291edc`; original web was `mega-magic-web:0d085ea`.

## Remaining acceptance

No physical RouterBOARD or CHR execution was performed. Actual router commands, connectivity migration, expiry/failure behavior and the phone voucher journey require the unavailable spare router or an authorized isolated CHR. Deployment readiness is verified; hardware certification remains pending.
