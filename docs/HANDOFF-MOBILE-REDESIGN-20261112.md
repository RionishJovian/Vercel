# MegaMagic mobile redesign — handoff (2026-11-12)

Spec: Notion "11 — Approved MegaMagic mobile design, quick setup and PPPoE ISP console".
Owner authorization (Notion, 2026-11-12): design and development are approved. **Merge and production deployment are separate gates.**

## Where the work is
- Branch: `claude/dazzling-feynman-chu5tf` (the cloud session's assigned branch; the brief named `redesign/mobile-ui`, which this session may not push).
  This cloud container has no Mac checkout, so no worktree was created next to `/Users/aces/GitHub/Mega-Magic`.
- Draft PR: https://github.com/RionishJovian/Vercel/pull/199
- Base: `main` at `e014fb3`. **Merging the PR deploys to production** (AGENTS.md §3).

## What changed (all in `app/`, the `/v2/` console)
| Area | Files |
|---|---|
| Tokens: emerald, mint, ivory; Day = ivory, Night = deep emerald | `src/styles/tokens.css`, `base.css`, `shell.css`, `pages.css` |
| Brand mark (wand with sparkling stars) replaces the header wand and the sign-in logo stage | `src/components/Brand.tsx`, `SignIn.tsx` |
| Navigation: 5 bottom tabs on phones, grouped left rail ≥900 px; skip link | `src/lib/nav.ts`, `src/components/Shell.tsx` |
| Home: dual Hotspot/PPPoE command, alerts, setup progress, per-card failures | `src/pages/Home.tsx` |
| New workspaces | `src/pages/workspaces/{Setup,Services,Operations,data}.tsx` |
| Shared status, evidence-time, metric, recovery and segmented components | `src/components/ui.tsx` |
| Strings (EN + MY first draft) | `src/i18n/workspaces.ts` |
| Logo review (option A in use, option B proposed) | `docs/design/megamagic-logo-review.{html,png}` |

New routes: `#/setup`, `#/hotspot`, `#/isp`, `#/customers`, `#/net`, `#/activity`, `#/approvals`, plus `#/portal` → `#/vouchers/guest`.
Every earlier route still resolves. `#/home?router=X` now redirects to `#/net?router=X`, where the port panel lives. Jellyfin moved from the bottom bar to More → Business and the desktop rail.

Removed after checking references: `src/components/LogoStage.tsx`, `public/mark.png`, `public/mark.webp`.

## Mapping to the 13 approved screens
| # | Screen | Implementation |
|---|---|---|
| 1 | Home / Dual Service | `#/home` (new) |
| 2 | Quick Setup | `#/setup` (new; evidence-based steps that open the existing screens) |
| 3 | Network Topology | `#/topology` (existing, restyled) |
| 4 | Managed Devices | `#/managed-devices` (existing, restyled) |
| 5 | Hotspot Service | `#/hotspot` (new hub) + `#/guests` (existing) |
| 6 | Vouchers | `#/vouchers/*` (existing, restyled) |
| 7 | PPPoE ISP | `#/isp` (new overview) + `#/pppoe` (existing subscriber management) |
| 8 | Customers | `#/customers` (new; Hotspot and PPPoE labelled separately) |
| 9 | Activity & Traffic | `#/activity` (new) + `#/events`, `#/fleet`, `#/guests` |
| 10 | Alerts & Recovery | `#/incidents` (existing) + shared `Recovery` panel |
| 11 | Configuration & Approvals | `#/approvals` (new hub over existing review/apply paths) |
| 12 | Revenue Records | `#/revenue` (existing) |
| 13 | More / Settings / Security | `#/more` (display and language added), `#/profile`, `#/security`, `#/backups` |

## Truthfulness rules built in
- Status uses Online / Needs attention (old data) / Offline / Not confirmed. No routers, or no evidence, is never shown as offline.
- A PPPoE subscriber is "connected" only when a session is recorded. A subscriber with no router observation is "Not confirmed".
- Voucher value is labelled "value of codes used, not cash received". PPPoE payments are "recorded by you", with no payment gateway implied.
- Quick setup never ticks Internet setup or the guest page on the router: the cloud cannot observe them. They show as "Check on router".
- Planned ISP modules (billing automation, RADIUS operations, FTTx/OLT/ONU) are shown disabled with a "Planned" label.
- The monitoring copy states the ~30 s screen refresh and the server monitor schedule; it does not claim real-time detection.

## Test evidence (this session)
- `app`: `tsc --noEmit`, `eslint src`, `vitest run` (58 files / 388 tests, including new Home, Quick setup, Customers and subscriber-state tests) and `vite build` all pass.
- Root: `test:deployment` 18/18, `test:vercel` 8/8 and `npm run build` pass. `test:critical-ui` is 21/22: `scripts/test-media-publishing-ui.mjs` timed out launching Chrome in this container. It covers `web/` media publishing, which this branch does not touch; PR CI is the authority.
- Browser checks used a local Go API (`cmd/server`) with a disposable Postgres (all migrations applied), NATS and Redis, signed in as a seeded owner:
  - 320, 390, 820 and 1440 px; English and Burmese; Day and Night. `scrollWidth - innerWidth = 0` on every screen checked.
  - Sign-in → Quick setup → Open Sites → create a site (one POST, 201; a second click sent no duplicate) → browser Back → step 1 shows Done with the site name as evidence.
  - Skip link is the first Tab stop and moves focus into the content.
  - With a gateway record that has no live router, Network shows "Not confirmed" and the ports panel offers "Try again". The ISP overview shows real (zero) records and the server's subscriber-write lock.
- Not tested in a browser: voucher generation and installation, guest captive login, PPPoE subscriber creation and sessions. Each needs a reachable router through Magic Hub (`guest_wifi.voucher_access_enabled` also needs the live applier).

## Hardware checks still required (unverified)
1. Gateway connection through Magic Hub, then Internet setup, then Hotspot setup, voucher generation, a customer logging in on a phone, and the session and usage appearing in Guests and Customers.
2. PPPoE: create a subscriber, assign a plan, see authentication and session in Customers, then renew, suspend and record payment. Production writes are gated by `MM_BROADBAND_WRITES_ENABLED` (off).
3. Screen-reader pass (VoiceOver and TalkBack) on navigation, forms and status changes.
4. A native Burmese review of `src/i18n/workspaces.ts`.

## Known gaps and API notes
- No cross-router PPPoE summary endpoint. `#/isp` and `#/customers` read `/routers/:id/broadband/accounts` once per gateway; that is fine for the one-router-per-client rule.
- Hotspot live-session counts are not on Home. They need a live router read (`/routers/:id/observations`) and are shown in Guests.
- No list endpoint for pending network changes across routers. Approvals shows AP-optimization proposals and the first router's `/devices/:id/changes` record.
- The topology Map/Tree views are the existing implementation, restyled only.
- Logo: option A is in use. Option B needs an owner decision, and the favicon and `web/` marketing pages still use the earlier mark.
- The local dev proxy in `app/vite.config.ts` forwards `Host: localhost:8080`, which the API's same-origin check rejects. This session used an uncommitted local override.

## Rollback
- Source rollback point: `main` at `e014fb3` (local tag `pre-mobile-redesign-20261112`; the session proxy refused the tag push, so the owner should push it or tag that commit).
- If the merge has deployed: revert the merge commit on `main` through a PR. Pushes to `main` redeploy. The deploy script also keeps the previous release and rolls back automatically when `/ready` or the header checks fail.
- No database migrations or backend changes are included, so rollback is frontend only.

## Next concrete action
The owner reviews PR #199 on a phone and decides on logo A or B. After CI is green and the owner accepts, merge (this deploys). Then run hardware checks 1–2 on the hAP ax2 and record the results in `docs/HARDWARE-TRIAL-RESULTS-*`.
