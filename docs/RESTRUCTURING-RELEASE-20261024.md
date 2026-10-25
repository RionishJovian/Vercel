# Restructuring release — 24 October 2026

Base deployment: fa3b6ef. Owner confirmed that current monthly/annual passes belong to an individual login, not a whole business.

## Current account passes

`app_account_passes` is a separate per-user ledger with explicit plan, activation/expiry, revocation and revision. No Store purchase, account tier, business trial or old Hub pass is copied into it. Platform DEVELOPER may record explicit terms with optimistic concurrency and an append-only audit event in the same transaction. Customer roles cannot record their own passes. `/me/app-pass` reads only the caller's login. Platform account reports show current pass records separately from historical compatibility data and shared business counters.

This is a recording/reporting rollout. It does not verify bank settlement or automatically enforce a new commercial policy. Existing access rules remain unchanged. Current per-login feature limits, prices, trial/grace policy, checkout and enforcement rollout still require a defined policy; unset limits are not portrayed as configured or enforced.

## Legacy isolation

Guest revenue no longer fetches, displays, exports or decides old business-pass requests. In current compatibility mode, Store shows the caller's current individual pass plus read-only old Store history, with distinct unavailable/empty states. The old subscription-only recovery UI remains reachable only when the existing runtime explicitly uses that mode, to avoid locking existing users out. Legacy API/data/authorization compatibility is retained, not promoted into the new pass authority. Team navigation now matches the backend OWNER+ requirement.

## Jellyfin

Hosted guest-page publication is separately enabled by `media.publishing_enabled` and the publication migration; it does not need a paired connector. `/media/capabilities` distinguishes publication availability, local connector enablement and paid access acceptance. Connector routes/keys and pairing are unchanged. Paid access remains unavailable. The editor labels sales settings as inactive design settings, and dashboard library actions stay disabled when the connector is unavailable.

Production rollout enables hosted publication only; connector and paid-access flags remain false. No guest page is published by deployment, no computer is paired, and no customer pass/payment is created or decided.

## Verification and rollout

Affected handler, server, database and config packages pass race-enabled tests with disposable PostgreSQL fixtures. Tests prove individual-vs-business isolation, self-recording denial, stale update rejection, audited revocation, old Store expiry independence, own-account reads, publication persistence without a connector, stale publication rejection and VIEWER write denial with database readback. Critical UI: 78 pass, zero failures/skips.

Linux AMD64: Go 1.26.5, CGO_ENABLED=0, -trimpath -buildvcs=false. Binary SHA256: 91f5a783a9b547f0018f55d53f8df75a875e322d85d6563ed37b3be538383076.

Before rollout, back up Compose/config/current binaries and image IDs, create/validate a fresh DB dump, then apply the account-pass migration and (only if absent) the publication migration. Build immutable API/web images; replace only those services. Back up and update exactly three web assets (app.html, media-appearance.js, media-dashboard.js). On readiness failure restore old Compose/config/binary/assets and old images. Do not restore the whole database for an application rollback; these migrations are additive and contain no historical data import.

Physical RouterBOARD/phone acceptance, local Jellyfin delivery/playback acceptance, paid router/database reconciliation and the remaining site-editor/capacity consolidation are not certified by these tests.

Local evidence: /Users/aces/Documents/Codex/2026-10-21/asd/work/restructure-20261024/.


## Verified production receipt

Owner additionally confirmed that monthly/annual passes have the same app features and differ only in validity. Numerical usage caps and enforcement remain a separate pending policy; no restrictions were activated.

API source: 066f46a70c0fee17d2176788ae582702c1990a6b, image tag mega-magic-api:066f46a. Final frontend source: af4282c (including 0e08b05 wording correction), image tag mega-magic-web:af4282c. Independent VPS build matched the Linux binary SHA256 above. All three deployed public web files matched their committed source. Readiness database/NATS/Redis OK; all five existing application containers running.

Backups:
- /root/mm-rollout-backups/restructure-066f46a-20261023T192510Z — validated DB dump, prior Compose/config/binary/assets and image IDs.
- /root/mm-rollout-backups/frontend-af4282c-20261023T193205Z — frontend follow-up rollback.

Source/build/deployment receipts and logs: /root/mm-restructure-066f46a/ and its frontend-af4282c/ subfolder.

Production readback: 0 current app passes, 0 pass audit events, 0 media publications after rollout. All three new tables have ENABLE + FORCE RLS. No historical purchase was imported, pass issued/revoked, payment approved, media page published or computer paired. Signed-in current pass/legacy history screen loads; platform account report loads all 13 accounts with per-login recording forms; Jellyfin reports publishing available, connector disabled and paid playback pending acceptance. Publication editor loads a real unpublished state with Publish available, replacing its former 404. Library reads stay disabled when connector readiness is unavailable, verified by an additional DOM regression.

78 critical UI tests pass; explicit media library-selection/disabled-connector regression passes; affected backend race tests pass; go build ./... passes. Physical/router and local playback acceptance remain pending.
