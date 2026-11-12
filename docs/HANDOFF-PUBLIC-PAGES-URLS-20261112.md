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
- Browser transition remains **unverified**: local Chromium is blocked by the sandbox, and the shared browser cannot connect to this local preview. No signed-in production checks or hardware checks were made. Earlier Claude browser evidence above does not cover this new recovery work.

Reproduce the transition check in an environment with Chromium, Playwright and Caddy. Build `app/` at both the previous release `5931ed0` and this candidate, then run the candidate root build:

```sh
CADDY_BIN=/absolute/path/to/caddy PREVIOUS_ROOT=/absolute/path/to/5931ed0-checkout node scripts/check-console-transition.mjs
```

`TRANSITION_HTTP_ONLY=1` runs the server-only checks and must not be reported as browser acceptance. The full test uses a disposable read-only API fixture. It keeps an old tab open while switching releases, opens a previously unloaded map chunk, checks unsaved input and document preservation, simulates a missing future map chunk, cancels Reload, and checks a clean link after rollback. Run it successfully before calling this release ready. Keep PR #201 draft until review and fresh CI are complete; no merge or deploy was performed during this takeover.

## Next concrete action
1. The owner reviews the preview and PR #201.
2. On approval, create the rollback branch, merge, and follow the deploy.
3. Verify the following on `megamagic.tech`:
   - `/`, `/signin`, `/pricing` and `/app/home`;
   - the `/v2/#/…` and `.html` redirects;
   - sign-in return-to-page;
   - `/ready` and the security headers;
   - that the image label matches the merge commit.
