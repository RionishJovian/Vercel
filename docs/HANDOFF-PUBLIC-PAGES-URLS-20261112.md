# Handoff: public pages and clean URLs (2026-11-12)

Release candidate: PR #201 on branch `claude/dazzling-feynman-chu5tf`. It is not merged; a merge to `main` deploys (AGENTS.md §3).
Production `main` at the time of writing is `5931ed0` (PR #200, capsule bottom navigation).

## What changes
1. **Public pages (MegaMagic design system).** Landing, sign-in/create-account and pricing use the emerald/mint/ivory tokens and the approved logo A. Sign-in has inline validation, a busy state, show-password, a Caps Lock hint and mapped errors (401, 403 suspended, 409 already registered, 429, network). The landing page lists only features that exist today.
2. **Clean URLs.**

| URL | Serves |
|---|---|
| `/` | landing |
| `/signin`, `/signin?register=1` | sign in, create account |
| `/pricing` | pricing |
| `/app/<page>` (`/app/home`, `/app/net`, `/app/vouchers/create`, `/app/customers?type=pppoe`, …) | console |
| `/api/v1/…`, `/health`, `/ready` | unchanged |

The Network tab is `/app/net`. `/app/network` stays the existing Internet setup screen, so old `#/network` links keep their meaning.

## Compatibility redirects (Caddy `deploy/Caddyfile.go-staging` and `vercel.json` match)
| Old | New | Code |
|---|---|---|
| `/signin.html?…` | `/signin?…` | 301 |
| `/pricing.html?…` | `/pricing?…` | 301 |
| `/v2`, `/v2/`, `/v2/index.html` | `/app/` (query kept) | 301 |
| `/v2/<page>` (except `assets/`) | `/app/<page>` | 301 |
| `/v2/assets/<hash>` | retained previous-release file | 200; absent files 404 |
| `/app.html?…` | `/app/home?…` | 302 (as before, new target) |
| `/app` | `/app/` | 301 |
| `/v2/#/vouchers` bookmark | `/app/vouchers` | browsers keep `#…` across the 301; the console then rewrites `/app/#/vouchers` to `/app/vouchers` with `history.replaceState` (no extra Back step) |
| `/app/#portal` | `/app/vouchers/guest` | console |
| unknown `/app/<x>` | `/app/home` | console |

`/app/assets/*` is served as files only: a missing hashed file returns 404, never the page HTML. Pages outside `/app/` keep the earlier fallback (unknown paths show the landing page).

In the console, links are now real paths (`href="/app/net"`). A plain click navigates without reloading. Ctrl/Cmd-click, middle-click, `target` and download links keep the browser's behaviour. Legacy `href="#/…"` and `location.hash` code still work through the same layer (`app/src/lib/route.ts`).

## Return-to-page security
- **Console sign-in.** A signed-out visit to any `/app/` page shows the console sign-in on that same URL, and signing in reloads that page. The "Create account" link passes `next=<that page>`.
- **Public sign-in.** `/signin?next=…` accepts only `/app/<route>` or an old `/v2/#/<route>`. A route is a lowercase name, an optional sub-segment and a query of `[A-Za-z0-9=&_-.%]`. Anything else opens `/app/home`. The URL is always built as the fixed `"/app/#/" + route`, so the value cannot change the site, path or scheme. CodeQL showed no alerts after this pattern was introduced in `2c6a8be`.
- **Retested and rejected** (all open `/app/home` on the same origin): `//evil.example`, `https://evil.example`, `javascript:alert(1)`, `/app//evil.example`, `/app/%2F%2Fevil`, `/app/\\evil.example`, `/app/home#x`, injected markup in the query, and an empty value.
- **Server redirects stay on the site.** For example, `/v2//evil.example` goes to `/app/evil.example` on the same host. Path traversal such as `/v2/..%2F..%2Fetc%2Fpasswd` shows the landing page.

## Deploy impact
- The new console is served from `/srv/app`. All 66 previous-release JS/CSS/font files remain at their original `/v2/assets/*` URLs. Each file was compared byte-for-byte with production at `5931ed0`; Docker and Vercel builds verify the archive and individual SHA-256 manifest before installing it. The old entry HTML is not retained, so new visits migrate to `/app/…`. Page HTML is `no-cache`.
- The app never reloads automatically on a chunk failure. A failed lazy map stays inside its own boundary, preserving an adjacent form. The explicit Reload button asks for confirmation after input/change events. Only an edited boolean is retained, never field values. Ordinary application errors still reach the page boundary.
- No API, database, environment or write-gate change. `MM_BROADBAND_WRITES_ENABLED` stays off.
- Old links shared in Telegram or saved as bookmarks keep working through the redirects above. The Go API generates no `/v2/` or `.html` links (checked with grep).

## Earlier test evidence (Claude, before the takeover)
The production Caddyfile was run with Caddy v2.10.2: `caddy validate` passed. It served a `/srv` tree built exactly as `Dockerfile.web.prod` builds it, against the local Go API, Postgres, NATS and Redis. A Playwright flow ran 63 checks, and 62 passed:
- **Direct visits and refresh:**
  - Direct visit and refresh of `/`, `/signin` and `/pricing`, with no horizontal overflow.
  - Direct visits to `/app/home`, `/net`, `/vouchers`, `/activity`, `/more`, `/customers?type=pppoe`, `/vouchers/guest` and `/network/new`.
- **Back and Forward:**
  - Across landing → pricing → sign-in.
  - Inside the console (bottom nav → `/app/net` without a reload, with `aria-current`).
- **Sign-in and registration:**
  - A signed-out deep link to `/app/vouchers/create` → sign in → the same page → refresh keeps it.
  - Sign out on `/app/more` → the sign-in form on the same URL → sign in → `/app/more`.
  - Public sign-in with `next=/app/customers?type=pppoe` and with `next=/v2/#/activity`, plus the 8 hostile values above.
  - Registration in Burmese from `/signin.html?register=1&next=%2Fapp%2Fnet` → `/app/net`.
- **Old URLs:** the redirects in the table above.
- **The 1 failure:** the topology map could not fetch `tiles.openfreemap.org`, which this container's network blocks. It is unrelated to the migration.

Automated suites:
- `app/`: `tsc`, `npm run lint`, and vitest 401/401, which includes the new `src/lib/route.test.ts`.
- Root: `npm run build`, `test:deployment` 18/18, `test:vercel` 8/8, `test:critical-ui` 22/22.
- `path-to-regexp` checks of the Vercel patterns.

Screenshots: `docs/design/public-pages/` (`app-deeplink-signin-390.png`, `app-net-390.png`, `register-390-my.png`, plus the earlier public-page set).

## Not verified (outstanding)
- **Live production:** `megamagic.tech` has not been checked with these URLs, because nothing is deployed yet.
- **Vercel preview:** the redirects there are exercised only once the preview builds this commit.
- **Assistive technology and devices:** no screen-reader pass (VoiceOver/TalkBack), and no check on a real iOS Safari or Android Chrome device. Password-manager autofill across the `.html` → clean-URL change was not tested on a device.
- **Burmese copy:** the new and changed text still needs a native-speaker review.
- **Hardware trial (unchanged by this work):** router write gates and the PPPoE writes acceptance are separate; PPPoE writes stay off.

## Rollback
- **Before merging:** create `rollback/pre-public-pages-20261112` at `5931ed0` (needs owner approval, as for the earlier rollback branches). Also record the running web and API image digests.
- **Original-image fast path:** redeploy the previous web image. The API is unchanged, so only the web container needs it. This restores `/v2/`, but the original image alone cannot serve new `/app/*` links.
- **Clean-link-compatible rollback:** `Dockerfile.web.rollback` layers only the route bridge and compatibility Caddyfile onto the recorded previous web image. Build with its full verified digest, not a floating tag:

  ```sh
  : "${PREVIOUS_WEB_IMAGE:?Set the recorded previous web image including @sha256 digest}"
  docker build -f Dockerfile.web.rollback --build-arg PREVIOUS_WEB_IMAGE="$PREVIOUS_WEB_IMAGE" -t megamagic-web-rollback:5931ed0 .
  ```

  Use the normal reviewed web-container replacement procedure with this image; do not change the API, database or write gates. `/app/activity?x=1` is served in place by the old console; the external route bridge adds `#/activity?x=1` for its hash router. Serving in place avoids a loop through a cached permanent `/v2/` → `/app/` redirect. `/signin` and `/pricing` also work. This optional rollback image has not been built or deployed in this environment (Docker is unavailable); its server rules have passed the local Caddy HTTP check.
- **Source path:** revert the squash commit on `main` in a pull request (deploys through CI). Never force-push `main`.
- Verify a shared `/app/activity` link, refresh, sign-in return, `/v2/#/activity`, `/signin`, `/pricing` and `/ready` after rollback. The browser route bridge still requires the browser transition check below before release approval.

## Takeover checkpoint (2026-11-12)

Claude's screenshot shows unpushed edits. Those files were unavailable here and were not changed. This continuation was reconstructed from published PR head `212f717` in an isolated checkout. Before Claude resumes, stop its auto-continue and sync the published branch; do not blindly resume its stale working tree or overwrite either copy.

Checks run on this continuation:
- App: 406/406 tests, lint and production build pass.
- Root: production frontend build, deployment tests 18/18, Vercel tests 8/8 and legacy-asset tests 3/3 pass. Critical UI: 24/25 pass; the existing media-publishing test cannot launch Chrome in this sandbox.
- Caddy 2.10.2: candidate configuration validates. The HTTP transition fixture verifies all 66 old assets by SHA-256 at their original URLs, absent assets return 404, removing the compatibility snapshot produces the expected 404, old page redirects retain the query, and rollback clean paths return the bridged old HTML without a server redirect.
- *(Superseded by the browser acceptance below.)* Browser transition was **unverified** here: local Chromium is blocked by the sandbox, and the shared browser cannot connect to this local preview. No signed-in production checks or hardware checks were made. Earlier Claude browser evidence above does not cover this new recovery work.

Reproduce the transition check in an environment with Chromium, Playwright and Caddy. Build `app/` at both the previous release `5931ed0` and this candidate, then run the candidate root build:

```sh
CADDY_BIN=/absolute/path/to/caddy PREVIOUS_ROOT=/absolute/path/to/5931ed0-checkout node scripts/check-console-transition.mjs
```

`TRANSITION_HTTP_ONLY=1` runs the server-only checks and must not be reported as browser acceptance. The full test uses a disposable read-only API fixture. It keeps an old tab open while switching releases, opens a previously unloaded map chunk, checks unsaved input and document preservation, simulates a missing future map chunk, cancels Reload, and checks a clean link after rollback. Run it successfully before calling this release ready. Keep PR #201 draft until review and fresh CI are complete; no merge or deploy was performed during this takeover.

## Browser transition acceptance (Claude, 2026-11-13, on `257d5c3` plus the two fixes in the next commit)
Environment: Chromium (Playwright), Caddy v2.10.2 running the candidate `deploy/Caddyfile.go-staging`, the previous release rebuilt from `5931ed0` (its 66 files byte-identical to production), and phone width 390 px.

- **`scripts/check-console-transition.mjs` (full browser mode, read-only fixture API): 5/5 passed.**
  - An old tab opens the never-loaded map chunk after the upgrade, and both the typed input and the document survive.
  - Kept files return 200; absent files return 404.
  - The negative control fails as expected.
  - A missing new map chunk stays inline, and cancelling Reload keeps the typed input.
  - After rollback, the clean `/app/activity` link opens without a redirect loop.
- **End-to-end flow with the real local Go API and a real sign-in: 16/16 passed.**
  - **Previous release, old tab:**
    - A tab opened on `5931ed0` at `/v2/`, signed in, and left Vouchers unopened.
    - The server then switched to the candidate.
    - Vouchers opened from the bottom navigation, loading `index-DW6rR2Ih.js`, `DenseTable-*.js/.css` and `errors-*.js` from `/v2/assets/`, with no errors and no reload.
    - Reloading that tab lands on `/app/vouchers`.
  - **Negative control:** the same steps against the candidate without `/v2/assets` fail with 404s.
  - **Clean-page redirects:** all still in place (`/v2/`, `/v2/<file>`, `signin.html`, `pricing.html`, `/app.html`).
  - **Candidate tab when a later release replaces its files** (a rebuilt "next" release, with 40 hashed names changed):
    - Nothing reloads by itself, the tab bar keeps working, and a plain message with a Reload button appears.
    - The confirmed Reload opens `/app/vouchers` on the new release.
  - **Unsaved input:**
    - A new site's name is typed, then the map's file turns out to be missing after the release.
    - The map fails inline, and the form and the value stay. Nothing reloads.
    - Reload asks first ("Reloading may discard your unsaved changes"), and cancelling keeps the value.
- **Found and fixed in the next commit:**
  - Kept `/v2/assets/*` files were sent with `Cache-Control: no-cache`. They are now cached like `/app/assets/*`.
  - CodeQL "information exposure through a stack trace" in the test fixture: it now logs the error and returns a fixed message.
- **Noted, not changed (fails safe):** the edit flag is one boolean kept until a full reload. Typing into the console's in-place sign-in therefore makes a later Reload ask for confirmation even with no form on screen. That is an extra question, never lost input.
- **Suites on the same tree:**
  - App: tsc, lint and 406/406 tests.
  - Root: build, deployment 18/18, Vercel 8/8, critical UI 25/25 (the media-publishing test runs here).
- **Rollback branch:** `rollback/pre-public-pages-20261112` was created at exactly `5931ed011342ec1f9fa94f84024df42c371c9a0a`. Before creating it, I confirmed that no branch or tag of that name existed, and pushed with a lease that only succeeds if the name is absent. `main` has since moved to `2fe42e6` (PR #202), which changes only `CLAUDE.md` and `.gitignore`, so the application code is identical to `5931ed0`.

## Next concrete action
1. The owner reviews the preview and PR #201.
2. On approval, merge and follow the deploy (the rollback branch already exists, see above).
3. Verify the following on `megamagic.tech`:
   - `/`, `/signin`, `/pricing` and `/app/home`;
   - the `/v2/#/…` and `.html` redirects;
   - sign-in return-to-page;
   - `/ready` and the security headers;
   - that the image label matches the merge commit.

## Production release verification (Claude, 2026-11-13, after the merge)

**Result: `7f63fbfc2ea909ebded0a056df8222cf6c1c5b15` (PR #201) is live on `megamagic.tech`. Evidence label: Deployed, Production verified (signed-out and API level). The signed-in console checks ran on a local stack serving the exact release build, not in production.**

### Release record
| Item | Value |
|---|---|
| Merge | `7f63fbf`, squash of PR #201, 2026-11-13 05:40 UTC |
| Release run | [Actions run 43373581788](https://github.com/RionishJovian/Vercel/actions/runs/43373581788) (#672): validate, build and deploy-vps all succeeded |
| API image | `ghcr.io/rionishjovian/vercel/mega-magic-api@sha256:bf213532f6e1a641661e86f0c8c4fd0a3f5d25a03fdebff0138f08187251e737` |
| Web image | `ghcr.io/rionishjovian/vercel/mega-magic-web@sha256:d09c02b9299162d961adf2e6b04b76fdac7f8ced48083091b7b0398a5fea66a4` |
| Revision check | `deploy/deploy-vps.sh:27` requires both images' `org.opencontainers.image.revision` label to equal the release SHA; the log ends `Verified release 7f63fbf…` |
| Migrations | none pending (`nothing to apply`; the 9 post-baseline migrations were already applied) |
| Server rollback copy | `/root/mm-rollout-backups/ci-7f63fbfc2ea909ebded0a056df8222cf6c1c5b15-20261113T054717Z` |
| Images running immediately before | run 668 (`2fe42e6`, same app code as `5931ed0`): API `sha256:5320e4f038e2f112a32ec01e72a9b0d3d3fa3748ff08f7e0dfe75c427f4dcc7a`, web `sha256:376eaa99be36c1356c40710443062be481f21389e8dafd5ccd294b5d0755a78b` |
| Rollback branch | `rollback/pre-public-pages-20261112` at `5931ed0` |

During container recreation the readiness probe logged one `curl: (56) Connection reset` before passing; the same happens on run 668 and is the expected restart gap.

### Production checks (signed out, from this session)
- **Served content = this commit.** A clean local `app/` build of `7f63fbf` produced 66 files; all 66 are byte-identical (SHA-256) to `https://megamagic.tech/app/assets/*`, and `/app/home` references the same entry files (`index-Cb4vlmOU.js`, `index-CsuIgxv-.css`).
- **Previous release kept.** All 66 files listed in `deploy/compat/console-5931ed0/manifest.json` are served at `/v2/assets/*` with matching SHA-256. Absent files return 404 under both `/v2/assets/` and `/app/assets/`. Fingerprinted assets carry no `no-cache` (ETag/Last-Modified only); pages are `no-cache`.
- **Readiness.** `/ready` → `{"checks":{"database":"ok","nats":"ok","redis":"ok"},"status":"ready"}`; `/health` healthy. `/api/v1/version` still reports `0.1.0-dev` (does not identify the commit, as before). `/api/v1` itself is 404 (no index route; unchanged).
- **Security headers** on pages and API: CSP (`default-src 'self'`, `object-src 'none'`, `frame-ancestors 'self'`), HSTS `max-age=31536000`, `nosniff`, `X-Frame-Options: SAMEORIGIN`, `Referrer-Policy: strict-origin-when-cross-origin`.
- **Pages (200):** `/`, `/signin`, `/signin?register=1`, `/pricing`, `/app/home`, `/app/net`, `/app/vouchers`, `/app/activity`, `/app/more`, `/app/customers?type=pppoe`.
- **Redirects:** `/app.html` 302 → `/app/home` (query kept); `/signin.html` and `/pricing.html` 301 → clean URL (query kept); `/v2`, `/v2/` 301 → `/app/`; `/v2/vouchers` 301 → `/app/vouchers`; `/app` 301 → `/app/`; `/v2//evil.example` stays on `megamagic.tech`.
- **Browser (Chromium, 390 × 844, Day and Night):** no horizontal overflow on any page above; public pages have no console errors or failed requests. Signed-out `/app/*` pages show the console sign-in on the requested URL. Their only failed requests are the expected `401 /api/v1/me` and `400 /api/v1/auth/refresh` (no refresh cookie). `/v2/#/vouchers` → `/app/vouchers`, `/v2/#/activity` → `/app/activity`, `/app/#/net` → `/app/net`. Public Back/Forward (landing → pricing → sign-in) works. Hostile `next=` values keep the visitor on `megamagic.tech`.
- **API refusals:** `/api/v1/me`, `/sites`, `/guest-vouchers/config`, `/guest-vouchers/batches` return 401 signed out. A wrong password with the console's headers (`Origin`, `X-MM-CSRF: 1`) returns `401 invalid credentials`; without `X-MM-CSRF` it returns `403 same-origin request required` (the intended CSRF guard).

### Signed-in flow on the exact release build (local, not production)
Production Caddyfile run by Caddy v2.10.2, `/srv` assembled exactly as `Dockerfile.web.prod` does (including the verified 66 legacy assets), the Go API built from `7f63fbf`, and a disposable PostgreSQL 16 database built with `mm-migrate init` + `up` (baseline + 9 migrations), Redis and NATS (JetStream). 390 × 844 Chromium. 14 of 16 checks passed:
- Registration from `/signin?register=1&next=/app/net` lands on `/app/net`.
- The five tabs (Home, Network, Vouchers, Activity, More) open the right path with `aria-current="page"`, without a page reload and without horizontal overflow.
- Console Back/Forward stays in sync with the bottom navigation.
- Signed in, `/app/#portal` → `/app/vouchers/guest`, `/v2/#/activity` → `/app/activity`, `/app/nonsense` → `/app/home`, `/app.html` → `/app/home`, and `/app/customers?type=pppoe` opens directly.
- Sign-out on `/app/more` shows the sign-in on the same URL, and `/me` then returns 401. Signing in again returns to `/app/more`. Public sign-in with `next=/app/vouchers/create` lands there.
- **Not product failures:**
  - Network: the map tiles host `tiles.openfreemap.org` is blocked in this container (the same as on 2026-11-12).
  - Vouchers: `/guest-vouchers/*` returned 404 locally because the disposable config leaves `guest_wifi.voucher_access_enabled` and the applier off. Production registers those routes (401 signed out).
  - **But** the Vouchers screen then rendered an empty body with no message (screenshot reviewed). A failed load must show an error state; this is tracked for Phase 2.

### Not verified (remaining uncertainty)
- **A real signed-in production session.** No production account or approved browser session was available here. Owner action: sign in on a phone, open each of the five tabs, sign out and sign in from a deep link.
- **The live `MM_BROADBAND_WRITES_ENABLED` value.** It can only be read with SSH or from an authenticated `write_enabled` field. Source evidence:
  - The deploy script sets no `*_ENABLED` gate and keeps the server's settings.
  - PR #201 changed no environment.
  - The handler requires both `MM_NETWORK_CONTROLS_ENABLED` and `MM_BROADBAND_WRITES_ENABLED` to equal `true` (`internal/handler/broadband_handler.go:45`).
  - The last recorded state (AGENTS.md §5, 2026-11-06) is off.
- Device, assistive-technology and native-Burmese checks remain open (see above).
