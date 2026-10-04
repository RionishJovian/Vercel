# Mega Magic

Mega Magic is a multi-tenant web application for MikroTik network operators and
hotspot businesses. Customers should be able to configure their own networks,
manage guests and vouchers, review payments, track approved revenue, and perform
remote operations through practical browser tools. Each business owns its
devices, settings, purchases and Telegram bot binding.

**Current agent handoff:** [docs/CURRENT-HANDOFF.md](docs/CURRENT-HANDOFF.md)

**Latest hosting/media status (2026-10-04):** HANDOVER records migration to
Hostinger; current live state requires verification. The owner confirmed
LAN-only media: videos stay on the owner's PC/business RouterBOARD network.
The old cloud video relay is superseded; source rejects cloud video transport.
Local paid playback acceptance remains pending. See the current handoff.

**2026-10-03 audit repairs:** [scope and validation](docs/AUDIT-FIXES-20261003.md).
These repairs do not enable held Wi-Fi writes or unfinished paid media access.

**Status date:** 2026-10-03 (Asia/Yangon). Deployment claims describe recorded
results on that date, not continuous health monitoring.

## Actively maintained application

This repository contains the current Go API and static browser frontend,
deployed at <https://mikromagic.com>.

- `cmd/server/`, `internal/`, `pkg/`: authentication, tenant repositories,
  payment/quota services, monitoring, Telegram integration and RouterOS control.
- `web/`: deployed frontend. `app.html` is the authenticated console;
  `portal.html` is the guest captive portal; `index.html`, `signin.html` and
  `pricing.html` are public pages. Shared assets live in `web/assets/`.
  Portal branding/plans are saved to the hosted portal; the separate reviewed
  **Deploy to router** workflow installs its HotSpot entrypoint. See
  [portal deployment and verification boundaries](docs/PORTAL-ROUTER-DEPLOYMENT.md).
- `database/`, `migrations/`: database baseline and schema changes. Migration
  files require explicit application and checks in each environment.
- `scripts/`, `lab/`, `deploy.sh`: validation, migration helpers, CHR lab tooling
  and the canonical production deployment script.
- `docs/`, `specs/`: contracts, guides and dated development records. Use the
  current handoff to interpret older status claims.

The runtime uses Go/Gin (`go.mod` requires Go 1.26), PostgreSQL 16, Redis 7,
NATS 2 and RouterOS over SSH with pinned host keys. WireGuard provides management
transport. The maintained frontend is HTML/CSS/JavaScript; inherited React and
TanStack references are not the deployed console.

## Current development phase

The guided **Network setup** workspace separates customer LAN/DHCP/Hotspot
configuration from the Gateway device diagnostics. It suggests private LAN
addresses after reading existing addresses, routes and subscriber/client pools; preview and apply repeat
those checks. Users must choose actual uplink and customer port roles explicitly.
The new task-focused WAN, PPPoE and existing-network controls implement reviewed,
backed-up router changes. Two disposable CHRs verified WAN failover/recovery,
equal-cost routing, PPPoE authentication/internet and DHCP reservation delivery.
SSID editors are implemented but separately held for physical-radio acceptance;
CAPsMAN configuration and a cloud WebFig proxy remain outside this slice. The
tested controls were deployed at `7f432ad`; production Wi-Fi writes remain off
until approved physical AP acceptance. No customer router was changed by deployment. See
[network workflow scope and acceptance](docs/NETWORK-SETUP-WORKFLOW.md).

The project is in **functional workflow completion and production acceptance**.
The API, database and console exist. Work now completes network and business
operations and proves that customer actions change the intended state, survive
reloads and respect tenant boundaries. The entire product is not yet certified
as complete.

The latest deployed batch includes customer-network setup (existing uplink,
LAN/optional VLAN, DHCP, client DNS, scoped NAT, optional Hotspot and an existing
RADIUS server), secure WebFig service setup, authentication-session hardening,
and frontend behavior repairs. Go tests, static UI checks and browser acceptance
passed. Production schema preflights, page publication, protected routes and
health were checked.

The new gateway and WebFig mutation paths were **not executed on the live CHR**
during rollout. They require disposable-lab acceptance and real connectivity
checks. WebFig service configuration also does not by itself provide a cloud
browser proxy to the private management address. Detailed evidence and outstanding
work are in the [current handoff](docs/CURRENT-HANDOFF.md).

The Magic Media connection/library pilot was migrated and deployed on
2026-10-01 with approval. PostgreSQL isolation/concurrency tests and a
production-schema rehearsal passed; live dashboard and security smoke checks
passed. Real local Jellyfin pairing is still pending. The connector is a local
terminal pilot, not yet a one-click installer; paid guest media and playback are
not implemented. See [Magic Media integration](docs/MAGIC-MEDIA-INTEGRATION.md).

The follow-up media overview was tested, migrated and deployed on 2026-10-01.
It collects Jellyfin counts/sessions and computer CPU,
memory and local-drive readings. Guest themes are preserved, with a new poster
layout selector. The dashboard
uses `20261001140000_magic_media_dashboard.sql` and requires an updated local
connector. Production health/access-control and CDN asset checks passed; real
paired-computer readings remain unverified.

The 2026-10-03 media publishing slice adds server-saved guest designs, planned
pass pricing, public sharing links and unpublishing. It requires the additive
`20261003120000_magic_media_publications.sql` migration. This is hosted-page
publication, **not paid playback**: public pages omit sample movies and keep
purchases disabled. See [media publishing](docs/MEDIA-PUBLISHING.md) for rollout
evidence and the remaining timed-access work.

Paid-access continuation adds guest bank/MMQR checkout, private receipts,
tenant-owned Telegram approval and a timed-pass ledger, plus local Jellyfin
streaming primitives. Guest catalog/playback transport still needs integration.
Production remains the publishing slice above; see
[paid-access continuation](docs/MEDIA-PAID-ACCESS.md) for acceptance boundaries.

## Delivery stages

Track every feature or release slice as:

**implemented → tested → migrated → deployed → verified**

1. **Implemented:** source exists and the intended UI/API is wired.
2. **Tested:** record suites, results and whether they used mocks, a disposable
   database, CHR or physical hardware. Skipped tests remain unverified.
3. **Migrated:** required schema/data/permission changes were applied and checked
   in the named environment. Use `N/A` if none are needed.
4. **Deployed:** the tested release reached the named environment; record the
   revision, date, backup and deployment result.
5. **Verified:** record the actual behavior observed in that environment.
   Health `200` proves availability, not payment activation, guest internet,
   RouterOS mutation, rollback or tenant isolation.

Use **partial**, **awaiting**, **unverified** and **N/A** where appropriate. Never
replace these stages with a single “done” checkbox. Code completion is not
production completion. Update the handoff after each release with concrete
evidence and remaining checks.

## Development and validation

Use a development configuration and protected local secrets. Service templates
are in `config.yaml.example` and `docker-compose.yml`. Compose is a development
template, not the production release procedure; review its credentials before use.

Run from the repository root:

```sh
go test ./...
node scripts/auth-client.test.cjs
node scripts/audit-web-ui.mjs
node scripts/e2e-portal-ui.mjs
git diff --check
```

Browser acceptance uses a loopback mock API and temporary Chrome profile;
`CHROME_BIN` can select another Chrome/Chromium installation. Database integration
tests require `MM_TEST_DB_URL` pointing to a disposable-test-capable PostgreSQL
server. RouterOS certification uses the separately configured lab described in
[HARDWARE-CERTIFICATION.md](docs/HARDWARE-CERTIFICATION.md). Report skipped
integration/hardware tests explicitly even if the overall Go suite passes.

The inherited `package.json` contains Vite/Vitest commands from the first app.
Those commands do not validate the deployed Go/static application.
`test:portal-ui` is a current browser test entry point.

## Production layout and release procedure

The recorded layout is:

- Application source/API binary: `/home/ubuntu/mega-magic-project` on
  `mikromagic-app`; systemd service: `mega-magic-api`.
- Served frontend: `/opt/mega-magic-web`, published from the repository's `web/`.
- Traffic: Cloudflare → nginx → static pages/API at `127.0.0.1:8080`.
- Separate management hub: `mikromagic-hub`.

Follow [DATABASE-MIGRATIONS.md](docs/DATABASE-MIGRATIONS.md), then run the canonical
script on the production host:

```sh
./deploy.sh api
./deploy.sh web
# Or publish both:
./deploy.sh all
```

API deployment validates production configuration and required schemas before
building/restarting; it does not apply migrations automatically. Web deployment
stamps shared asset URLs for cache invalidation. Deployment must be followed by
production smoke and feature-specific checks. Existing authorization and actions
requiring further owner authorization are recorded in the handoff.

## Historical source

This repository began on 2026-09-23 as a rewrite using material from the first
MikroMagic project. `/opt/mikromagic-app` and
`/Users/aces/Nish/mikromagic/app` identify historical source, not the current
application's maintained production paths.

`lib/`, `shared/`, `ui-reference/`, inherited React/TypeScript build configuration
and older provisioning assets preserve first-project behavior and migration
context. Historical Supabase migrations coexist with current migrations; do not
execute the entire directory as a fresh-install migration sequence.

The previous README's “backend to build”, proposed React frontend, weekly phase
schedule and “isolated from production” statements are superseded.
[PROGRESS.md](specs/PROGRESS.md), [ROADMAP.md](specs/ROADMAP.md) and recovery audits
contain dated claims. Preserve their recorded requirements and decisions, but
use current code, the handoff and environment checks for present delivery status.

Useful references:

- [Current agent handoff](docs/CURRENT-HANDOFF.md)
- [Magic Dude provider/key setup](docs/MAGIC-DUDE-PROVIDER.md)
- [Database migration procedure](docs/DATABASE-MIGRATIONS.md)
- [RouterOS certification](docs/HARDWARE-CERTIFICATION.md)
- [Acceptance requirements](docs/ACCEPTANCE-TEST-REGISTER.md)
- [Security contract](docs/SECURITY-CONTRACT.md)
- [First-project recovery audit](docs/FIRST-PROJECT-RECOVERY-AUDIT.md)
