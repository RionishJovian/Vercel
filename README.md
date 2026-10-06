# MikroMagic development build

Three-role React workspace plus Python/SQLite development API. Read BLUEPRINT.md for the intended product and STATUS.md for actual implementation/remaining work.

## Local development
Requirements: Node.js 20+, npm, Python 3.12+.

```
npm ci
python3 -m venv .venv
.venv/bin/python -m pip install -r requirements.txt
npm run dev -- --host 0.0.0.0 --port 4173 --strictPort
```

The dev launcher starts the demo API at 127.0.0.1:8787 and account API at 127.0.0.1:8788 alongside Vite; /api and /v1 are proxied. When DATABASE_URL is absent, it migrates a separate accounts.local.sqlite3 database. With an explicit DATABASE_URL, run migrations separately before starting. SQLite is created as demo.sqlite3. Demo role selector creates short-lived in-memory sessions. Real payment/network/media/AI adapters are not connected. Do not expose this development app as a production service.

## Checks
```
.venv/bin/python -m unittest discover -s tests -p 'test_*.py' -v
npm run build
npm run test:sites
```

## Manual-review callback
Defaults to disabled. Configure PAYMENT_HOOK_SECRET and PAYMENT_HOOK_TENANT only in a private development environment. POST /api/hooks/payment-decision with JSON {id, decision, reason}. X-Timestamp: unix seconds; X-Nonce: at least 16 characters; X-Signature: lowercase hex HMAC-SHA256 of timestamp + '.' + nonce + '.' + raw body. Maximum skew 300 seconds; repeated nonce rejected. This endpoint is tenant-scoped, but real platform reviewer mapping/adapters are still required.

## Deployment
User plans separate webapp and WireGuard hub VPS instances. Production authentication, infrastructure adapters, evidence uploads, accounting and streaming gateway must be completed and tested before deployment. The bundled Sites worker only serves frontend assets; it does not contain the Python API. Do not deploy it alone and describe this as a functioning backend.

## Increment 3: account-backed browser workflows

The default screen is account sign-in. Account role and tenant come from `/v1/me`; there is no account role selector. Tokens stay in memory, expire server-side and are revoked on sign-out. Reload requires another sign-in. Demo exploration is separate and does not copy demo records into accounts.

SME package creation, payment review, evidence requests, refund recording, revenue summaries and audit history use `/v1`. Guests create retry-safe purchases, resubmit evidence and view stored passes. Platform accounts see businesses and audit events. Other account modules clearly list pending work. No seeded accounts or default passwords are shipped. Provision using backend/provision.py with the same DATABASE_URL as the service.

For restricted preview runtimes without access to a virtual environment, install dependencies inside the checkout with `python3 -m pip install --target .runtime/python -r requirements.txt`; the launcher uses this path when no accessible .venv exists. These dependencies are excluded from the source archive.

Backend tests use isolated SQLite databases. PostgreSQL runtime tests and authenticated browser journeys are pending. Browser checks cover sign-in layout, demo entry and return; authenticated journeys and the account service proxy remain unverified in the browser. Client tests cover expired-session clearing and validation error display.

```
node --test tests/account-client.test.mjs
```

## Increment 4: captive portal configuration

SME `/v1/portal` draft/publication endpoints now power the portal editor. `/v1/portal/public` returns published guest content to signed-in guests without network fields. New purchases must pass the published portal revision and accepted method. Payment instructions are copied into `orders.payment_context`; migration 002 leaves that field null for older orders. Never edit migration 001 to add the new schema.

Portal publication updates app content; it does not apply RouterOS rules. This first configuration is per tenant, with per-site records still pending. Guest account resolution before hotspot login remains pending.

```
node scripts/verify-portal-ui.mjs
```

This check renders the actual React components and verifies purchase closure, content escaping and separation of private network fields. Its `_qa/portal-preview.html` output is a static sample with disabled controls, not a second application or authenticated session.

## Increment 5: vouchers

Account-backed `/v1/vouchers` provides SME issuance/inventory/revocation and guest redemption. Full random codes appear once; database records contain hashes and masked hints. Retried issuance returns metadata without the original code; revoke/reissue if it was lost. Published voucher login gates new redemption. Redemption and revocation update normalized entitlements with explicit order/voucher origins. Voucher metrics count issued/redeemed codes; there is no implied payment or collection.

Migration 003 refuses to downgrade if vouchers exist; use a forward migration to preserve records. Voucher sales/payment linkage and all live network/media enforcement remain pending.

```
node scripts/verify-voucher-ui.mjs
```

## Increment 6: voucher counter payments

SME inventory opens a payment record form. `/v1/vouchers/{id}/sale` records confirmed full-price receipt; `/sale/refund` records a full refund already handled outside the app and revokes the stored pass. Sale records are unique per voucher, preserve policy/receipt/staff snapshots and have retry/conflict behavior. Revenue combines confirmed voucher payments and approved orders without counting redemption. Migration 004 refuses to discard accounting records on downgrade. Period reporting, reconciliation, discounts, notifications and live adapter enforcement remain pending.

## Increment 7: sites and MikroTik registry

SME `Sites` and `Routers` screens use tenant-scoped `/v1/sites` and `/v1/routers`. Platform reads fleet metadata; guests cannot. Router metadata is owner-reported, with an unverified connection and no hub/handshake. `/v1/sites/{id}/portal` has independent draft/publication endpoints. Guests list published branches through `/v1/guest-sites`, then read `/v1/portal/public?site_id=...`. Orders/vouchers/pass records retain optional site identity; legacy null assignments remain unchanged. Shared business packages, financial site filters, router portal assignment, actual hub onboarding and automatic guest-network resolution remain pending.

```
node scripts/verify-network-ui.mjs
```

## Increment 8: Magic Hub onboarding

Platform `Magic Hubs` registers immutable hubs and assigns them to businesses. SME `Routers` creates a management-only structured connection plan with atomic /32 allocation, public keys, endpoint, routes and required application checks. Reviewed plans await the adapter; connectivity stays unverified. Exported JSON is not an executable configuration. Private keys stay on devices; full live addressing/firewall/capability validation, hub provisioning, command application and trusted evidence are pending.

Migration 006 adds magic_hubs, hub_assignments and router_plans. Dedicated aligned private /24 pools are unique. Exact router-plan retries reuse allocation; keys and per-router plans are unique. API review records SME acknowledgement without networking side effects.

```
node scripts/verify-hub-ui.mjs
```

The generated Magic-Hub-preview.html uses sample records and disabled controls, rendered from the production components. WireGuard fields follow the [official MikroTik manual](https://manual.mikrotik.com/docs/virtual-private-networks/wireguard/). Configure and test actual device capabilities before generating/applying commands.

## Increment 9: SME revenue reporting

Live router configuration/verification are deferred at the user’s request. SME Revenue now supports site/date/timezone filters, gross/refunds/net cash KPIs, receipt averages, payment-review workload, and service/source/site/method/day breakdowns. Refund events belong to their own recording period; prior periods retain the original receipt. Financial events are created transactionally with approval and counter-sale workflows.

`/v1/revenue/ledger` is SME-only with the same filters and bounded pagination. CSV export includes signed amounts, source identifiers and timestamp basis; 10,000-event export cap requires narrowing larger periods. Operating costs/profit and independent cash reconciliation remain pending.

Migration 007 backfills legacy payments using available audits, otherwise explicitly labelled submission-date estimates. It refuses downgrade with financial events present. PostgreSQL SQL generation passes; local testing uses SQLite.

```
node scripts/verify-revenue-ui.mjs
```

Revenue-dashboard-preview.html renders the actual React filters and report with illustrative sample records and disabled controls.

## Increment 10: platform appearance

Platform Appearance saves light/dark palette drafts and publishes to selected platform/SME/guest workspaces. Platform scope also controls shared sign-in colors. Unselected roles use default palettes; per-tenant overrides are pending. Draft/previous palettes remain platform-only. Effective colors load on refresh or sign-in.

Revision checks prevent stale writes; previous publication restore preserves a reversible restore point. Twenty checked color pairs must meet 4.5:1 before publication, while low-contrast drafts can be saved for correction. This does not replace complete browser accessibility testing. Migration 008 preserves appearance records on downgrade.

```
node scripts/verify-appearance-ui.mjs
```

Appearance-preview.html renders the actual editor and light/dark sample previews with disabled controls.

## Increment 11: media catalogue and sales

SME Media manages title metadata with draft/publish/unpublish/archive/restore. Edits return titles to draft. Guests browse only their own business's published titles. No files, stream URLs or server credentials are stored here.

Packages now supports media library/title/hybrid modes; hybrid has independent library hours and title_hours. Title packages reference an owned catalogue ID and snapshot its metadata into purchase/pass history. Published availability gates new orders, approvals and voucher issuance. Media-only library sales need a published title. Existing historical purchases/vouchers are retained after withdrawal. Playback/activation remains pending.

Migration 009 adds media_titles and refuses destructive catalogue downgrade. Site-specific media-source mapping, Jellyfin synchronization, uploads/artwork/transcoding, playback authorization and per-segment enforcement remain pending.

```
node scripts/verify-media-ui.mjs
```

Media-catalogue-preview.html renders actual owner and guest components with sample titles/passes and disabled controls.
