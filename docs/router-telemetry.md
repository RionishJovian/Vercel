# RouterBOARD telemetry

Implemented in the existing Go API and `web/` application. Mega Magic reads the
router through its saved management connection; RouterBOARD remains responsible
for captive login, guest access and time/data enforcement. This collector installs
no agent or scheduler and makes no router configuration changes.

## Data and access

`GET /routers/:id/telemetry` requires an authenticated business and at least the
Admin role. Ownership is checked on every request, including cache hits.
The response identifies the device, source, last check, last successful resource
reading, individual metric timestamps, retained snapshot and resource history.
It never returns credentials or raw SSH errors.

The reader uses the existing per-device credential resolver, destination policy
and pinned SSH host key. This consolidated source attaches the read interface
through the current `EnableLive` configurator, matching the newer role model.
Telemetry itself retains only the `ReadTelemetry` interface and performs no
configuration or guest-action writes. An unconfigured collector returns HTTP 503 with a setup code.
No database migration or additional router permissions are automatically applied.

Four fixed reads collect the data:

- `/system/resource print`: CPU, memory, uptime, board and RouterOS version.
- `TelemetryInterfacesCommand` in `internal/provisioner/routeros/telemetry.go`:
  read-only RouterOS script printing interface identity, flags and byte counters.
- `/ip hotspot active print count-only`: current Hotspot sessions.
- `/ip dhcp-server lease print count-only where status=bound`: bound DHCP leases.

Interface rates use two confirmed counter samples, with a 1–120 second interval.
Counter decreases, uptime regression, identity changes and long gaps suppress the
derived rate. Counters remain exact decimal strings in JSON. Bridge/member counts
can overlap; hardware-offloaded traffic may be outside these counters. The UI
does not sum interfaces into an invented Internet usage figure.

## Retention and refresh

The backend coalesces overlapping reads and samples on demand at most once per
15 seconds per business/device. Collection has a 12 second request deadline. The
process retains at most 64 devices and 60 resource points per device; eviction or
process restart loses this short history. This is not a persistent monitoring
database or an unattended background collector.

Failed reads retain confirmed values and their original observation times.
Partial failures retain only the affected sources. Missing CPU/memory samples
remain gaps in the chart. Real zeroes remain zeroes; errors never become zeroes.
The dashboard labels connection/measurement/retry progress instead of using an
"unavailable" telemetry placeholder or claiming stale data is live. It refreshes
automatically, supports pause/manual refresh, and stops on hidden documents or
navigation away from the widget.

## Verification and deployment boundary — 2026-10-28

Collector/parser, retention/rates/concurrency, authorization and UI regressions
passed locally, including the Go race detector. The loopback-only fixture at
`scripts/fixtures/telemetry-preview` exercises the actual collector, service and
UI with explicitly synthetic RouterOS command responses. It has no write routes
or real router transport. Its numbers are not physical-router acceptance.

The owner will reconnect the hAP ax² later. Fresh hardware collection remains
pending. The initial local build was subsequently deployed with explicit owner
authorization; see the deployment record below.

The running app contains newer source than this checkout, including voucher
installation safeguards and staging site resolution. Before deployment, recover
the current running source and apply these changes additively. Do not overwrite
the live API or web app with this checkout. Preserve the staging authoritative
`/vps/peers/:id/site` adapter, voucher retry/idempotency protections, tenant/role
boundaries and existing configuration gates.

The additive integration points are the telemetry reader/service/handler files,
one authenticated server route, the retained read service field and wiring in
`SecurityHandler`, the telemetry assets in `web/app.html`, and the Monitor mount,
dispose and layout changes in the existing device workspace. Deploy the matching
API route and assets together only after reconciling the current source. Do not
change production flags as part of a frontend preview.

## Production deployment — 2026-10-28

Telemetry is deployed at `https://megamagic.tech/app.html?release=telemetry-20261028#internet`.
The actual running API source was recovered on the VPS at
`/root/mm-reseller-60fb7bb`; its binary matched the running executable. The new
API was built additively in `/root/mm-telemetry-20261028`, preserving the deployed
backend's newer features. Its role model has no Operator constant, so the
production route uses the existing Admin gate. That version attaches the reader
through its existing `EnableLive` configurator; no role constants, configuration
flags or router write gates were changed. The local inventory-only wiring above
describes this checkout, not an additional production flag change.

The frontend preserves the captured live baseline and staging saved-site/voucher
adapters. Images `mega-magic-api:telemetry-20261028` and
`mega-magic-web:telemetry-20261028` are pinned in the persistent Compose file.
Current-source Go regressions passed on the VPS. Public HTML and seven file
hashes, API health, HTTP 401 for anonymous telemetry and HTTP 200 for signed-in
telemetry were verified, together with the existing one-unused/ten-unconfirmed
guest voucher summary. The actual UI displays Reconnecting to RouterBOARD.

Exact prior running containers were captured as local rollback images because
their original image IDs were no longer available on disk. The deployment plan,
source, build/test logs, original Compose backup and rollback image references
are retained under `/root/mm-telemetry-20261028`. Runtime environment, application
configuration, ports, mounts, security options and data-service containers were
verified unchanged. No fixture, physical acceptance, payment operation or router
configuration change was included in this deployment. Reconcile this production
source before any future full API release from the local checkout.
