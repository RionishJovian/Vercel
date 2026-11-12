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
| `/v2/<file>` | `/app/<file>` | 301 |
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
- After the merge deploys, the web image serves the console from `/srv/app`, and `/v2/assets/*` no longer exists. A console tab left open from before the release keeps working until it lazy-loads a page chunk. That chunk now returns 404, so the person reloads and lands on `/app/…`. Page HTML is `no-cache`.
- No API, database, environment or write-gate change. `MM_BROADBAND_WRITES_ENABLED` stays off.
- Old links shared in Telegram or saved as bookmarks keep working through the redirects above. The Go API generates no `/v2/` or `.html` links (checked with grep).

## Test evidence (this session, local, phone width 390 px)
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
- **Fast path:** redeploy the previous web image. The API is unchanged, so only the web container needs it.
- **Source path:** revert the squash commit on `main` in a pull request (deploys through CI). Never force-push `main`.
- **After a rollback:** `/app/...` links shared since the release would fall through to the landing page, while `/v2/#/...` links work again.

## Next concrete action
1. The owner reviews the preview and PR #201.
2. On approval, create the rollback branch, merge, and follow the deploy.
3. Verify the following on `megamagic.tech`:
   - `/`, `/signin`, `/pricing` and `/app/home`;
   - the `/v2/#/…` and `.html` redirects;
   - sign-in return-to-page;
   - `/ready` and the security headers;
   - that the image label matches the merge commit.
