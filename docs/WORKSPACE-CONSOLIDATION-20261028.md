# Default consolidated workspace — 28 October 2026

Use `/Users/aces/GitHub/Mega-Magic` for the app. Origin is
`https://github.com/RionishJovian/Vercel.git`, default branch `main`.

## Inputs and merge decisions

The committed baseline is current Vercel main
`14b20de6e0a2bb8def27a912ccda5bf63107163d`, recovered through an independent
Git clone. The local recovery branch `bbf3ed1` had the same committed tree.
Its newer account model, voucher issuance/recovery, verified printing,
subscription, management migration, Vercel proxy/build and release checks remain.

October 28 changes from the former Mega-Magic checkout add the RouterOS
telemetry collector, bounded retained-reading service, authenticated handler,
device workspace, guest access flow, topology context/race protection and
portal/media rollback improvements. Conflicts were reconciled rather than
replacing newer files with whole older versions. Telemetry uses Admin and the
existing live configurator; the removed Operator role and old inventory wiring
were not reintroduced. The router-local CHAP/PAP portal stays intact.

The frontend starts from the seven files in the recorded October 28 telemetry
release, which had been assembled against the then-current live frontend to
retain account and voucher safeguards. The latest local topology continuity
change was then restored. Local marketing and new widget assets were retained.
The original release, additional source handoffs and evidence are retained
privately under `.local/` and excluded from Git and frontend output.

The 20-folder inventory and cleanup manifest identify exactly which older
directories were removed from their working locations. Recoverable copies go
to the Mac Trash. Two worktree pairs shared metadata; this new Git checkout has
its own object store and does not depend on either old pair.

## Verification and operating boundary

Passed before installing this folder: `go test ./...`, focused telemetry race
checks, `npm run build`, all 107 critical UI tests, all eight Vercel proxy tests,
both deployment-transform tests, and the separate device/topology and guest-flow
checks. Database tests that require
an external test database retain their normal environment-gated skips.
These checks do not certify production migration or physical-router behavior.

Private records include the original tracked local diff, original source identity,
staging Caddy change, the archived original Mega-Magic Git branch, and readable
release workspace files. iCloud placeholder paths are recorded separately and
remain in the recoverable original Trash copies; they are not silently counted
as locally copied complete artifacts.

The prior deployment note identifies VPS source `/root/mm-telemetry-20261028`
and images `mega-magic-api:telemetry-20261028` /
`mega-magic-web:telemetry-20261028`. Consolidation does not redeploy those images
or change production. Verify current source, configuration and migration
identity separately before a future release.

## Future work

Read the current identity first, work in this folder, and use the Vercel origin.
Treat the older handoffs and historical repository names as provenance.
Do not use a Trash copy or a private recovery artifact as the default source.
