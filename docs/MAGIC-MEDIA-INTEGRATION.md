# Magic Media: local Jellyfin integration

Updated 2026-10-01. This document describes implemented code, not a completed
customer-facing connection or installer.

## Dashboard extension (deployed 2026-10-01; paired-computer acceptance pending)

The new Overview tab queues the fixed read-only `dashboard` operation through the
same tenant-bound connector. The local runner reads Jellyfin `/System/Info`,
`/Items/Counts`, recent `/Sessions`, public server status and library mappings.
Only typed selected fields are returned, never raw responses, API keys or private
client addresses. Catalog counts cover movies, series, episodes, songs and albums;
active users are distinct user IDs within the returned recent sessions, not all
registered accounts. IDs themselves are not returned. Missing values are nullable
and displayed as Unavailable; a real zero remains zero.

Native measurements use pinned `github.com/shirou/gopsutil/v4 v4.26.8`: CPU and RAM
are whole-computer readings, not Jellyfin process consumption. Drive capacity/free
space comes from local mounted volumes that contain configured library paths,
deduplicated by longest matching mount. Used space includes all files on the
volume, not just media. UNC paths and remote filesystem types are excluded;
Windows probes only fixed/removable local drives. No files are enumerated/uploaded.
Snapshots are timestamped and manually requested, not historical utilization,
uptime, or recent-activity charts. The owner should understand that these readings
and the selected user/device/title summaries travel to their tenant's cloud account.

The new API requires `20261001140000_magic_media_dashboard.sql` after the deployed
pilot migration and checks the constraint at startup when opted in. Both cloud
API/web and the local runner must be updated; old runners cannot collect this job.
Focused race/frontend checks, Windows/macOS builds and three consecutive isolated
PostgreSQL race runs passed, including on the VPS release source. A schema-only
production rehearsal passed, followed by production migration and scoped API/web
deployment after the user's continuation approval. Health/landing/app returned
200, unauthenticated owner/connector routes 401, browser-Origin connector requests
403, and versioned CDN modules matched release source. Actual paired-Mac readings
have NOT been verified. No customer Jellyfin libraries or payments were changed.
Backups: `/home/ubuntu/deploy-backups/media-dashboard-20261001-rollout/` (private
database/source/frontend archives). Keep the additive constraint during binary
rollback; do not restore the full database without a separate recovery decision.

The guest design keeps Forest, Midnight and Cinema plum, branding/accent controls,
and now Comfortable/Compact poster layout. It remains a browser-only preview.
Future real movie-library views must reuse the tenant's theme/layout with actual
Jellyfin artwork; preview success is not published themes, paid media or playback.

## Outbound connection pilot (deployed 2026-10-01; live pairing pending)

The pilot now includes `internal/media/bridge`, `internal/media/connector`,
`cmd/media-connector`, tenant handlers/routes, and the Magic Media Connection /
Libraries / Guest design tabs. It is disabled by default via
`media.connector_enabled: false` (environment: `MEDIA_CONNECTOR_ENABLED`).
Enabling it without both migrated tables and forced RLS fails server startup.
Production was explicitly enabled on 2026-10-01 through the systemd
`98-magic-media.conf` drop-in (`MEDIA_CONNECTOR_ENABLED=true`). The default stays
off for other environments. API/web rollout and basic authenticated dashboard
verification passed; real local Jellyfin pairing has not been performed.

Owner/admin API routes use the existing authenticated session, same-origin write
guard and active-business check. Pairing codes have 256 bits of entropy, expire
after ten minutes, and redeem once under a database transaction. Only their hashes
are stored. A redeemed connector receives a 24-hour bearer token, also stored only
as a hash. Replacement/revocation cancels queued/running jobs. A running local
operation may already have taken effect before revocation; revocation is not a
rollback. Never promise otherwise.

Connector requests reject browser Origin headers/cookies as authentication,
refuse credential-bearing redirects, and use outbound HTTPS. Loopback HTTP is
permitted only for local tests. Polling carries a fixed allow-list of status,
library listing, library addition and scan requests, not shell commands. Jobs have
two-minute deadlines, at-most-once delivery and no write retry. Unknown outcomes
must be inspected before another change is submitted. Claims re-check the actor's
current active state, business and admin/owner/developer role. All connections and
jobs are tenant scoped, including explicit WHERE predicates and forced RLS.

The pilot permits one media connection per business, ten outstanding jobs and
1,000 jobs per rolling 24 hours. It uses a bounded process-local IP request limit
for connector authentication (30/minute); shared NATs/multiple API instances need
a shared, authenticated-connector-aware proxy limit before a broad rollout. Job
retention/cleanup and durable OS credential storage are not implemented.

Two secret-hash identity lookups deliberately use the privileged owner pool to
resolve an anonymous connector's tenant. Subsequent operations enter WithTenant
transactions. This relies on the existing owner pool's lookup privileges; do not
silently replace it with an unscoped restricted pool or bypass tenant transactions.

### Local connector entry

On the computer running Jellyfin, the development runner is:

```sh
go run ./cmd/media-connector
```

It asks for the webapp pairing code and Jellyfin API key using **hidden local
terminal prompts**. It checks local library access before consuming the code.
No secrets are written to disk, browser storage or logs. Keep the terminal open;
Ctrl-C stops the process. Restarting requires a new pairing code. It is not a
Windows installer/service or a finished non-technical customer setup flow.
To permit confirmed add/scan requests explicitly:

```sh
go run ./cmd/media-connector -allow-library-changes
```

The webapp displays the pairing code only for the current view (no localStorage).
The user enters the Jellyfin key only on their media computer, never in the cloud
webapp or chat. Existing local credentials were not read or changed for testing.

For this Mac development machine, the updated local build is
`build/magic-media-connector-mac`. Run it in a normal Terminal after generating a
pairing code in Magic Media → Connection. It remains an unsigned terminal pilot,
not an automatic installer or background service. Dashboard requests report
whole-computer CPU/RAM, mapped local drive capacity, and selected session/title
summaries to the paired business account; no media files are uploaded. Secrets
must be entered locally by the owner. This build was compiled, not automatically
launched or paired.

### Checks and rollout gate

Focused race-tested adapter, connector mock round-trip, secret validation and
handler tests pass. Handler tests prove submitted business/actor IDs are ignored
in favour of authenticated context and insufficient roles never reach the store.
Browser acceptance with mock data verifies connection display, library listing,
confirmed library-add submission and guest design tab, with no console errors.
These are not live database or Jellyfin mutation acceptance.

`postgres_integration_test.go` includes disposable-local-DB checks for single-use
redemption, concurrent at-most-once job claim, RLS/cross-tenant denial, completion
replay rejection, expiry, revocation and business suspension. Additional checks
cover actor deactivation/role/tenant changes, expired jobs/tokens, connector
replacement and cross-tenant inserts. On 2026-10-01, all three tests passed five
consecutive runs with race detection on an isolated PostgreSQL 16 VPS container.
The first real DB run found SQL NULL result scanning unsupported for
`json.RawMessage`; scanning through nullable `[]byte` fixed queued job reads.

Verification command: `go test -race -count=5 -v ./internal/media/bridge -run TestPostgres`.
The test database was `mm_media_test`, exposed only on VPS `127.0.0.1:15439` in
container `mm-media-integration-test`, separate from the production container.
Only synthetic business/user fixtures were used. The migration ran in isolated
schemas which the tests cleaned. This does not verify a full production schema
upgrade or authenticated live Jellyfin operations. The disposable container is
removed after testing; no production migration, restart or feature enablement.

Without an explicit DSN these tests still SKIP, not pass. To rerun, explicitly
provide `MM_MEDIA_TEST_DSN` as a `postgres://` URL to a literal loopback IP and
dedicated `mm_media_test` database with an `authenticated` role. The test creates
and cleans an isolated schema; it refuses other databases and external hosts.

Before production, obtain authorization for the migration, API restart/deployment
and feature enablement. Back up the production database and relevant source; apply
`migrations/20261001120000_magic_media_connector.sql` with the established migration
guide. Do not run local test fixtures against production. Verify RLS/grants and
two-owner isolation on a disposable PostgreSQL environment first (the focused
synthetic-fixture checks now pass; full production-schema rehearsal remains). Then verify
pairing/expiry/revocation and one real owner's read-only library path. Live library
changes need that owner's explicit confirmation. Leave the flag off if any gate
fails. On 2026-10-01 the schema-only production rehearsal passed, then the owner
approved production migration/deployment. Both tables were migrated and checked
for enabled/forced RLS and tenant runtime grants; `./deploy.sh all` deployed the
scoped API/web changes. Landing, app and health returned 200; public module MIME
was JavaScript. Unauthenticated owner/connector requests returned 401 and
connector browser-Origin requests returned 403. The signed-in Nish dashboard
loaded all three tabs and correctly showed Not connected. No real pairing,
Jellyfin library change, guest entitlement or payment was tested.

Protected release backups on the VPS:
`/home/ubuntu/deploy-backups/media-connector-20261001-rollout/` contains the
custom-format database dump, pre-release source/binary/config archive and served
frontend archive. Keep these private; the database/source backup contains secrets
or customer data. Rollback disables/removes this release's systemd flag, restores
the pre-release binary/source and served frontend, reloads systemd and restarts
API/reloads nginx. Retain additive media tables rather than deleting customer
records during rollback. Do not restore the full database dump over live data
without a separate recovery decision and authorization.

## Implemented now

`internal/media/jellyfin` is the local Jellyfin API adapter for a future paired
Magic Media connector. It supports:

- Public server status and setup completion checks.
- Authenticated library listing, including an empty library list.
- Adding a library with server-side folder paths and a collection type.
- Requesting a library scan. Acceptance does not mean scanning has finished.

Credentials are sent in the supported `Authorization: MediaBrowser` header, never
URL parameters. The connector's old `X-Emby-Token` header caused Jellyfin 12.1.0
to reject the owner's local access check; it was replaced and the Mac connector
rebuilt. This compatibility fix does not prove authenticated live acceptance:
the owner must retry locally with their existing key. No legacy authorization
setting was enabled and no key was read, reset or requested in chat. Responses
are bounded; upstream errors do not expose response bodies, credentials or paths.
Redirects and environment proxies are disabled. Endpoints must use literal
loopback IPs and explicit ports. DNS names and arbitrary LAN/cloud URLs are not
accepted by this local adapter. TLS verification remains enabled for HTTPS.

The adapter is NOT exposed as an HTTP API. It has no tenant authorization itself:
the future connector must enforce the pairing identity and authorized operation.
Do not mount it behind a generic endpoint accepting a user-supplied address/key.
Do not use the cloud VPS's loopback address to represent the customer's computer.

Library paths refer to the computer running Jellyfin, not the browser's filesystem.
Windows and POSIX absolute paths are supported. JSON PathInfos preserve commas
in folder names. Adding libraries does not create media folders or upload content.
Writes are not automatically retried after network failure: inspect server state
before retrying to avoid duplicate work. No delete operation is implemented.

## Read-only local check

From the repository on the computer running Jellyfin:

```sh
go run ./cmd/media-check
```

Default address: `http://127.0.0.1:8096`. To use another local port set
`MM_JELLYFIN_LOCAL_URL` in the local process environment. Without a credential,
the command checks only public server information. It never installs software,
creates accounts, adds libraries, scans, or changes settings.

An optional `MM_JELLYFIN_LOCAL_TOKEN` permits a read-only library check. Do not
paste a token into chat, commit it, or pass it as a command-line argument. Secure
OS credential storage and a local entry screen are still to be implemented; do
not ask customers to configure environment variables as their onboarding flow.
The command explicitly reports `connected_to_mega_magic: false`.

## Verified on the owner's Mac

The unauthenticated status probe and new checker returned Jellyfin Server
12.1.0 with `StartupWizardCompleted: true`. No administrator password or key was
read. Library access and mutations on this real server remain unverified.
No movies are needed for server readiness; eventual playback testing can use
an owner-created phone video instead of copyrighted sample content.

Tests: `go test -race ./internal/media/jellyfin ./cmd/media-check`.
They cover authenticated wire requests against mock servers, safe URL handling,
redirect refusal, credential-free public probes, error redaction, empty/malformed/
oversized responses, invalid inputs, cancellation and absence of write retries.
Mock request correctness is not live Jellyfin mutation acceptance.

## Next implementation steps

1. Finish a signed Windows-first installer/local connector with a Mac development
   target. Detect an existing Jellyfin installation and never overwrite it.
2. Validate the implemented short-lived, single-use, tenant-bound pairing with revocation against PostgreSQL. The connector
   makes outbound authenticated TLS requests; no public inbound admin ports.
3. Replace the development terminal prompts with local secure graphical credential entry and OS credential storage. Credentials
   must not enter browser localStorage, logs, diagnostic exports or guest pages.
4. Complete acceptance of the implemented authorized endpoints and dashboard forms for list/add/scan.
   Resolve the tenant from authentication, not submitted IDs. Test cross-tenant
   denial, revoked pairings, disconnected computers and partial failures.
5. Add server-folder selection and permissions checks, explicit apply confirmation,
   API readback, friendly error recovery and progress display.
6. Integrate real catalog/artwork and guest playback. Keep admin credentials out
   of guest requests. Add separate media purchases, approval, entitlement expiry
   and session/network revocation; internet plans do not imply media access.

Installer installation, account/key creation, live folder/configuration changes,
router/firewall modifications and production migrations need their applicable
owner approval. Paid cloud infrastructure requires explicit approval. Publishing
frontend code does not authorize changing customer hardware.

## Sources

- [Official library API](https://typescript-sdk.jellyfin.org/classes/generated-client.LibraryStructureApi.html)
- [Official add-library parameters](https://typescript-sdk.jellyfin.org/interfaces/generated-client.LibraryStructureApiAddVirtualFolderRequest.html)
- [Library controller source](https://github.com/jellyfin/jellyfin/blob/master/Jellyfin.Api/Controllers/LibraryStructureController.cs)
- [Jellyfin library documentation](https://jellyfin.org/docs/general/server/libraries/)

Recheck the installed server's API contract before production acceptance; upstream
master and generated documentation may differ from the installed version.
## Guest design editor follow-up (2026-10-01)

**Rollout update:** editor, editable pictures and neutral gradient-free presets
are now deployed frontend-only. Public appearance JS/CSS 5 and dashboard JS 6
match the source; site/health checks pass and the API process was not restarted.
Backup: `/home/ubuntu/deploy-backups/media-design-20261001.YJtjrM/`.
This supersedes pending-deployment notes below, not the preview-only limitations.
Authenticated production editor acceptance is still unverified.

The owner subsequently rejected gradients and AI-looking artwork. Current
presets are Cinema (charcoal), Editorial (warm white), and Screening room (slate).
All use solid surfaces and conventional type-led layouts. Uploaded artwork is
not tinted, blurred or covered by overlays; captions sit below pictures. Missing
artwork is a text-only placeholder, not simulated movie art. Old palette drafts
are normalized without losing uploads. This supersedes the Forest/Neon design
initial implementation; server publishing remains pending.

The deployed implementation provides ready-made Cinema, Editorial and Screening room
layouts to Guest design. Category tiles support custom pictures, titles and
captions; background and four sample artwork cards can also be customized.
Template selection preserves custom pictures and wording. Wide shelves are
inspired by Jellyfin, not a copy or modification of its native web client.

Pictures stay in the business/account-scoped browser draft after local resizing
and JPEG encoding. SVG and remote image URLs are not accepted. This remains a
preview: it does not publish guest pages, edit real Jellyfin item artwork, or
enable payments/playback. Local tests and browser save/reload/upload/removal checks
passed. Authenticated production editor acceptance remains pending.
