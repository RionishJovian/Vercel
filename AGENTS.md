# AGENTS.md — Mega Magic

Last audited against the repo and production: 2026-11-06. Statements marked
**(owner decision)** are product rules; statements marked **(verified)** were
checked against code, CI or the live server on that date; **(not verified)**
means no check was made. Dated notes are evidence of that day, not live state.

## 1. Source identity

- Local workspace: `/Users/aces/GitHub/Mega-Magic`. Remote: `https://github.com/RionishJovian/Vercel.git`; default branch `main` (verified). Read `.repo-identity.json` and `docs/WORKSPACE-CONSOLIDATION-20261028.md` first.
- Do not restore or use older folders (for example `/Users/aces/123/...` or `~/GitHub/Mega-Magic-App`) based on their names or on older docs. `docs/HERMES-BUILD-CONTEXT.md` and `docs/HERMES-CONTINUATION-20261021.md` describe that old checkout and its remote (`RionishJovian/Mega`); they are historical.
- Never force-push or replace `main` with another history. Work on a branch and merge reviewed PRs normally. (On 2026-10-25 `main` was force-replaced by an October 7 history; recovery notes are in git history and `docs/`.)
- `.local/` holds private recovery records. It is git-ignored; never publish, package or treat it as source (verified: in `.gitignore`).

## 2. Layout (verified 2026-11-06)

| Path | What it is |
|---|---|
| `cmd/server`, `internal/`, `pkg/` | Go API (module `mega-magic`) |
| `app/` | React console, built with Vite and served at `/app/` (path routes such as `/app/net`; moved from `/v2/#/…` on 2026-11-12, old links redirect) |
| `web/` | Landing, pricing, sign-in, media and guest portal pages plus shared assets. Clean URLs `/signin` and `/pricing` (old `.html` links 301 with the query kept). The old `web/app.html`, vanilla `web/v2` and their tests are gone; `/app.html` redirects (302) to `/app/home` |
| `deploy/`, `Dockerfile.prod`, `Dockerfile.web.prod` | Release scripts, Caddy config, container images |
| `vercel.json`, `proxy.js`, `vercel-public/` | Vercel frontend; `/api/v1` is proxied to `https://megamagic.tech` |
| `src/`, `worker/`, `.openai/hosting.json`, `scripts/prepare-sites-build.mjs`, `tests/sites-worker.test.mjs` | Separate Sites prototype and historical Python/prototype material. Keep intact; it is not the production app and its build is not proof the app works |

Older notes that name `web/app.html` or ask for UI in `src/` are obsolete: new console UI goes in `app/`.

## 3. Build, test, release

- CI is `.github/workflows/deploy.yml` ("Validate and release Mega Magic") plus `codeql.yml`.
- `validate` (every push and PR) runs: Go toolchain check, `npm run test:deployment`, `test:vercel`, `npm run build`, `test:critical-ui`, `app/` lint/test/build, `go test -race ./internal/...`, `go build ./...`, a reachable-vulnerability scan, deploy/backup script syntax checks and the migration-runner test.
- `npm run build` builds the Vercel frontend (`web/` plus `app/` at `/app/`) via `scripts/prepare-vercel-build.mjs`. The Sites prototype uses `npm run build:sites` and `npm run test:sites`; run those only for a Sites handoff.
- **Deploy (owner decision 2026-11-06, PR 134):** every push to `main` that passes `validate` builds images and runs `deploy-vps` automatically. Manual dispatch with `deploy_vps` still re-releases. This replaced the manual-only gate from 2026-10-25. Consequence: merging any PR, including Dependabot bumps and PRs marked "needs a router test", goes live. Check write gates (section 5) before merging.
- The deploy script backs up, applies migrations, verifies public `/ready` and security headers, keeps a rollback copy and rolls back on failure.
- Production host: Vultr Singapore `magic-app` (`207.148.76.172`); `megamagic.tech` resolves there (verified). Hostinger (`srv2032937`, `156.67.216.128`) and the Z.com servers are discontinued; do not use or restore them. SSH uses the key recorded in the owner's SSH config, not a password. Never store credentials in this file.
- Proof of what is live: the API container image carries the label `org.opencontainers.image.revision` (matched `2ff77b1` on 2026-11-06), and served console assets can be hashed against a local `app/` build (matched on that date). `/api/v1/version` returns `0.1.0-dev` and does not identify the commit; an image tag alone is not proof of served content.
- Do not edit files inside running containers.
- Local tests: database tests need an explicitly selected disposable database (`MM_TEST_DB_URL`); never reuse production connection settings. Skipped database tests are not integration coverage. Do not start `deploy/compose.go-restore.yaml` for tests.

## 4. Product rules (owner decisions)

**Console and navigation**
- 2026-10-18: the revamp reuses the existing app, services, auth, tenant isolation and data; no rebuild or data reset. Navigation is business-task based: Overview; Business (Revenue, Internet Vouchers, Jellyfin dashboards); Operations; Guest Portals; Store; Management. Operations covers Sites (required before router onboarding), Magic Hub one-paste connection, Devices, Site Topology, Guests, Fleet Health and guided Internet Setup.
- 2026-10-25: keep short task labels and minimal descriptions; group related screens into shared workspaces (Magic Hub owns site, connection, discovery, network and maintenance; Vouchers owns internet guest-page design; Jellyfin owns its dashboard and separate guest page). Preserve old deep links, API authorization, role boundaries and review/apply confirmations. Never hide errors, live status, limits or consequential warnings.
- Internet Hotspot Portal (data/time sales and vouchers) stays separate from the Jellyfin Guest Portal.
- 2026-11-12 (owner decision): public URLs are `/` (landing), `/signin` (sign-in and `?register=1`), `/pricing`; the console is `/app/<page>` (`/app/home`, `/app/net`, `/app/vouchers/create`, …). Old `/v2/#/…` bookmarks, `/v2/…`, `/app.html`, `signin.html` and `pricing.html` keep working through redirects that keep the query. `/api/v1` is unchanged. The sign-in `next=` return path accepts only console route characters and is rebuilt from a fixed `/app/#/` prefix. The Network tab is `/app/net`; `/app/network` remains Internet setup. Handoff: `docs/HANDOFF-PUBLIC-PAGES-URLS-20261112.md`.
- 2026-10-22: third-party products are server-side API engines behind Mega Magic's own screens and accounts. No redirects, iframes, embedded dashboards or separate customer logins; keep service credentials out of browsers.

**Visual design**
- 2026-11-12 (owner decision, Notion spec 11): MegaMagic mobile-first redesign. Emerald, mint and ivory tokens (Day = ivory work surfaces, Night = deep emerald); solid panels with light depth, translucency only on bars; header logo is the wand-with-sparkling-stars mark (`app/src/components/Brand.tsx`); the owner approved this logo (option A) on 2026-11-12, and the "M" monogram (option B in `docs/design/megamagic-logo-review.html`) is not adopted. Phone navigation is Home, Network, Vouchers, Activity, More; desktop uses a grouped left rail. This supersedes the Jade Glass silver surfaces below; the other rules (Day/Night, readable status, reduced effects, keyboard/touch, no scenic photos) still apply. Product name in UI copy is "MegaMagic". Handoff: `docs/HANDOFF-MOBILE-REDESIGN-20261112.md`.
- 2026-11-13 (owner decision, revised the same day after review of the first pass): the public pages (`/`, `/signin`, `/pricing`), the console sign-in screen and the guest portals use the **layered deep-slate system**, with a Day twin. It replaces the flat `#09090B` Obsidian first pass, which felt flat and harsh, and the emerald/mint/ivory palette for these screens; the rest of the console keeps the 2026-11-12 system until the owner extends this decision. Night: canvas `#0B0E14` with an ambient radial mesh at the top of hero, sign-in and portal views (`radial-gradient(ellipse 75% 45% at 50% -10%, rgba(56,189,248,.09), transparent)`), elevated surface `#121620` (1px `rgba(255,255,255,.09)` border, top-edge highlight `inset 0 1px 0 rgba(255,255,255,.06)`, shadow `0 20px 40px -15px rgba(0,0,0,.6)`), recessed input `#0E121A`. Text: primary `#F8FAFC`, secondary `#94A3B8`, caption `#7B8BA3` (the requested `#64748B` is only 3.8:1 on the surface, so captions use `#7B8BA3`; `#64748B` is used only for input edges). Input edges are `#64748B` (about 3.9:1), not the 14% white hairline (1.5:1), because a control's boundary needs 3:1; the 9% and 14% hairlines are for panels and dividers. Focus: border sky plus `0 0 0 3px rgba(56,189,248,.18)`. Accent: sky cyan `#38BDF8` with dark ink `#06131F` (8.8:1), button sheen `linear-gradient(180deg,#38BDF8,#0EA5E9)` (ink 6.8:1 at the bottom); status colours stay emerald `#10B981`, amber `#FBBF24`, red `#F87171`. Day: canvas `#F8FAFC`, cards `#FFFFFF` with `0 1px 3px rgba(15,23,42,.06), 0 8px 24px -8px rgba(15,23,42,.08)`, borders `#E2E8F0`, text `#0F172A` / `#475569` / `#5B6B80` (`#64748B` is 4.3:1 on the recessed `#F1F5F9`), accent `#0369A1` with white ink (5.9:1), input edges `#64748B`. **Logo:** the green wand tile is replaced everywhere by the signal-and-spark mark: a dark tile, two 1.75px sky-cyan arcs and a white node, and a white four-point spark (`web/assets/images/megamagic-mark.svg`, inline in the public pages, `app/src/components/Brand.tsx` in the console; icons `megamagic-logo-b-64/180/512.png`; share image `og-card-20261114.jpg`). This supersedes the 2026-11-12 approval of the wand (option A). The console wordmark "Magic" is sky (`#38BDF8` Night, `#0369A1` Day). Type: Instrument Sans for text and headings; JetBrains Mono only for data values (IPs, WireGuard `/32` addresses, router models, voucher codes). Day/Night stays. Burmese uses 1.75 line height (1.6 for headings), and only the active language is shown, never both in one string. Links are controls, not raw underlines: nav, footer and Telegram links are pills or buttons, and Sign in / Create account is a segmented control (on the console sign-in the current side is a label and the other a link). Public pages: tokens in `web/assets/css/landing.css`; console sign-in: tokens scoped to `.signin` in `app/src/styles/signin.css`. `scripts/test-public-pages-ui.mjs` (part of `test:critical-ui`) enforces 4.5:1 text, 3:1 input borders, that every file shares one palette, one logo artwork and no underline styles. Sample figures on the landing router panel are illustrations and must stay labelled as sample data. Extended the same day to the guest portals: the hosted Internet Hotspot portal (`web/portal.html`, `web/assets/js/portal.js`) and the router-served voucher login page (`internal/provisioner/routeros/portal_deploy.go`, `portal_design.go`). The Jellyfin/Magic Media guest page (`web/media.html`) keeps its own owner-chosen appearance themes, and the owner-facing Guests dashboard stays on the console system, until the owner extends the decision again. **Step 3A (2026-11-13): the whole console shell and tokens moved to this system.** `app/src/styles/tokens.css` now holds the Deep Slate / Sky Cyan values under the existing token names (`--jade` is the sky accent), with radii 12/8/8, Instrument Sans for text and JetBrains Mono (`.num`, `.mono`) only for data values; Revenue uses a flat ledger hero, a hairline period strip (stacked on phones so figures never wrap) and a ruled status legend. The old `scripts/verify-revenue-ui.mjs` and the legacy Platform Appearance engine were removed with the old console (commit 9be41fa); Revenue is covered by `app/src/pages/Revenue.test.tsx`. Active bottom-nav pair in Day is white on `#0369A1`→`#075985` (4.5:1 or better).
- 2026-11-13 guest portal rules (owner decision, step 2): the hosted portal is mobile-first with Instrument Sans, JetBrains Mono for values (voucher code field, prices, account numbers, countdowns) and Noto Sans Myanmar bundled so Burmese renders correctly on any phone. It talks only to `/api/v1/portal/*` (config, voucher redemption, guest purchase, receipt, media tiles) and shows no private SME network fields. The owner's brand colour recolours only the primary button and a faint panel wash, and its ink is chosen by WCAG contrast (white on sky cyan would be 2.0:1); focus rings, text and status colours never depend on it. A saved colour is kept; with none saved the portal uses cyan. The pay screen shows the plan amount, the owner's MMQR image and bank details (account number copyable) and the owner's own payment instructions text; KBZ Pay, Wave and slip upload are not separate integrations, they are what the owner writes in those instructions plus the receipt photo, and every payment still waits for the owner's approval. The active-pass screen and its countdown appear only after the gateway confirms the voucher. The router page cannot load fonts or images (guest has no internet yet, 24 KB budget), so it uses system fonts and an inline SVG, in English. Guest-portal tokens live inline in `web/portal.html` and `portal_deploy.go`; `scripts/test-public-pages-ui.mjs` checks both for 4.5:1 text and 3:1 control edges in Night and Day. `web/assets/images/jade-portal-*.webp` and the same files under `internal/provisioner/routeros/assets/` are no longer used by any page; they were left in place so cached pages keep working. The Python `backend/appearance.py` is historical prototype code, not a Platform Appearance engine in the live Go server.
- 2026-10-23: Jade Glass — cool silver surfaces, emerald accents, precision geometric navigation icons, smooth object-free forest-toned background. No scenic photos, abstract silk, organic pebble icons or brass accents. Day/Night, readable status colours, reduced effects, keyboard/touch access and mobile navigation are required. The theme emits `day`/`night`.
- Guest voucher screen (2026-10-23; **look superseded 2026-11-13**, behaviour rules still apply): was a dark emerald liquid-glass card with a wallpaper. It is now the deep-slate and sky-cyan panel below: no wallpaper, no glass, Night by default and the Day twin on light phones (`prefers-color-scheme`). One renderer for owner preview and router deployment (the owner removed the separate "Voucher page" preview on 2026-11-06, so the editor now previews the hosted Guest Portal, not the router page); bundle assets locally; preserve CHAP/PAP, consent, error/retry and first-use behaviour.

**Routers and guests**
- 2026-10-29: router actions open on the current Magic Hub router. Do not repeat device selection per setting; use a single registered router automatically; legacy multi-router workspaces choose context once; keep the target visible, reject stale or foreign selections, lock context during writes; keep explicit review/apply gates. Reuse saved Magic Hub access everywhere; review/apply is for intentional changes, not monitoring.
- New Internet guest voucher codes are 8 characters from `ABCDEFGHJKMNPQRSTVWXYZ` (no I, L, O, U) (verified: `internal/voucher/voucher.go`). Preserve existing codes, batches, quotas and first-use evidence; distinguish installing codes on the router from refreshing usage back into the app. The portal editor stays under Vouchers → Guest page with the `#portal` deep link.
- Topology (2026-10-29): a visual device-image diagram with connection lines, interface labels, status and a clickable inspector, in Jade Glass. Use real model imagery when identified. Distinguish observed associations/forwarding ports from unknown physical attachments; never invent switch hops or online status.
- Guests is an automatic session dashboard across business sites, not onboarding. Gateway Hotspot sessions do not prove physical AP attachment. Friendly discovery labels are metadata, not authentication or proof of online state.
- Telemetry uses the existing Admin role and EnableLive read interface. Preserve tenant checks, voucher batch recovery/printing guards, account pass checks, typed router operations and explicit write gates.
- Never claim discovery, device actions or connectivity work without live evidence. Manual estimates or topology never count as discovered telemetry or verified enforcement.

**Accounts and passes** (2026-10-24; the code in `internal/subscription` and handlers was not re-audited line by line — **not verified**)
- The platform administrator manages client accounts. Each client has one authorized login, one private internal workspace, one site and one MikroTik router managed through Magic Hub. No staff users or invitations. Keep `business_id` tenant isolation internally; preserve existing users/history and do not guess the login in ambiguous legacy workspaces.
- Individual monthly and annual passes share features; only validity differs. A seven-day transition starts once when enforcement deploys; the platform administrator is exempt. Expiry blocks app management, not already-running guest internet. Old Store purchases do not activate passes.

**Other**
- 2026-11-07 (owner decision): keep one Telegram setup page under Your profile. A platform-admin binding handles app-pass/platform purchase approvals and platform incident alerts; a customer binding handles only that business’s guest-payment/Jellyfin approvals. Reuse encrypted saved tokens, derive scope from server-side roles, and show receiver/token/approver/delivery evidence separately. Saving a token is not proof a receiver is running; never send a test notification automatically.
- Site WAN/budget and topology editors share one revision; saving one must preserve the other.
- SME sidebar guidance is named "Did you know?"; platform publishers manage sourced cards. Keep compatibility "unchecked" until trusted router discovery exists. VPN, HotSpot/portal and RADIUS are separate roles. Local User Manager is a proposed option only.
- Read-only router inventory is gated by `provisioner.inventory_enabled` and must not enable the applier, guest disconnection or network writes.

## 5. Write gates (state read from production on 2026-11-06)

Writes to routers are fail-closed: each needs an env var that is exactly `"true"`, and the deploy script sets none of them. Some gates live in the server's `deploy/compose.go-restore.yaml` rather than `.env.production`, and deploys keep them, so git does not show the live state.

| Gate | Production | Notes |
|---|---|---|
| `MM_NETWORK_CONTROLS_ENABLED` | on (owner confirmed) | base gate for network, guest and device controls |
| `MM_NETWORK_ADDRESS_CONTROLS_ENABLED` | on (owner confirmed) | |
| `MM_MANAGEMENT_MIGRATION_ENABLED` | on (owner confirmed) | |
| `MM_GUEST_ACTIONS_ENABLED` | on (owner decision 2026-11-06) | Kick/Ban/router Delete. Most of the trial passed; see `docs/HARDWARE-TRIAL-RESULTS-20261106.md` |
| `MM_DEVICE_CONTROLS_ENABLED` | on (owner decision 2026-11-06) | Topology device controls. Trial partly run; see the same record |
| `MM_BROADBAND_WRITES_ENABLED` | off | paid PPPoE writes locked since the 2026-11-04 incident until a router trial |
| `MM_FIRMWARE_UPGRADE_ENABLED` | off | |
| `MM_NETWORK_WIFI_CONTROLS_ENABLED` | on (owner decision 2026-11-06) | physical-radio trial not run yet |
| `MM_WEBFIG_ACCESS` | off (unset) | Values `open` and `locked`. Also needs `MM_WEBFIG_HOST`; see `docs/WEBFIG-CLOUD-ACCESS.md` |

The owner asked for server-wide router read/write/edit/delete on 2026-11-06. These gates apply to every customer on the server. Re-read the server environment before relying on this table; it is a dated observation. Because pushes to `main` deploy, a PR that adds or depends on a gate must default to off.

## 6. Working rules

- Run and drive the local app yourself and verify changes in a browser; do not hand the user server-start instructions.
- Real-router testing happens after the app itself is ready; do not claim live connectivity, physical readings or router state without evidence.
- Third-party router behaviour you are unsure about: read forum.mikrotik.com and the RouterOS manual, then confirm on a router.
- Consolidation, docs and design notes are source changes. They do not deploy code, migrate a database, modify a router or certify live state. (Note: a merge to `main` now does deploy; see section 3.)
- Record durable design feedback and owner decisions here with a date.

## 7. Open items and known drift (as of 2026-11-06)

- **2026-11-12 PR #201 continuation:** retain the exact 66 public console assets from `5931ed0` at `/v2/assets/*` during the clean-URL migration. Both builds verify the pinned archive and SHA-256 manifest. No automatic chunk-error reloads; map failures stay inline and explicit Reload confirms after editing. The optional rollback web layer serves new `/app/*` links in place to avoid cached-redirect loops. See `docs/HANDOFF-PUBLIC-PAGES-URLS-20261112.md` for evidence and the still-pending browser transition test. Claude's screenshot showed unpushed work; sync the branch before resuming that working tree. This checkpoint does not authorize a merge or deployment.

- WebFig Cloud Access has never signed in to a real router. AGENTS section 4 (2026-10-22) bars embedded third-party dashboards and separate logins; the owner has not yet recorded whether WebFig, in its own tab with the router's own login, is an accepted exception.
- Real-router results and open bugs: `docs/HARDWARE-TRIAL-RESULTS-20261106.md`. The owner's hAP ax2 is no longer at factory settings.

- `/Users/aces/AGENTS.md` (workspace file outside this repo) still names the Hostinger host and `ssh mikromagic-hostinger`; it is out of date and not changed here.
- Dependabot PRs 91, 92, 93 (Vite, `@vitejs/plugin-react`, TypeScript 7) fail `npm ci` on peer-dependency conflicts; PR 87 (Go modules) needs a rebase onto the `ENVIRONMENT` binding fix. PR 45 says it needs a router test before deploy. Each now deploys on merge.
- Lint and tests for `app/` run in CI; the Go binary is tied to a commit only through the image label.
