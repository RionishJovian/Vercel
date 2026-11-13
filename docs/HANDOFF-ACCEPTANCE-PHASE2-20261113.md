# Handoff: mobile, accessibility and language acceptance (Phase 2, 2026-11-13)

Base: `main` at `7f63fbf` (live; see `docs/HANDOFF-PUBLIC-PAGES-URLS-20261112.md`, "Production release verification"). Branch `claude/gallant-mendel-urv40t`. Nothing here is merged or deployed; merging deploys (AGENTS.md §3).

## How it was tested
**Environment:**
- A local stack serving the exact release build: Caddy v2.10.2 with the production `deploy/Caddyfile.go-staging`, and `/srv` assembled exactly as `Dockerfile.web.prod` does.
- The Go API built from source, with a disposable PostgreSQL 16 database (`mm-migrate init` + `up`), Redis and NATS JetStream.
- Chromium through Playwright, and axe-core 4.10.3 (WCAG 2.0/2.1/2.2 A and AA rules).
- A disposable account registered through `/signin?register=1`.

**Pages:** `/`, `/pricing`, `/signin` and `/signin?register=1`, plus the console's Home, Quick Setup, Network, Hotspot, Vouchers (Codes, Create, Guest page), Customers (all and PPPoE), PPPoE ISP, Activity, Changes and approvals, and More.

**Matrix:** 320, 390, 430, 768 (tablet) and 1440 (desktop) px × Day/Night × English/Burmese, which is 340 page loads. axe ran on every 390 px combination and on 1440 Day English.

The production checks themselves are in the Phase 1 record. Signed-in production checks were not possible (no account).

## Results before fixes (exact release `7f63fbf`)
| Check | Result |
|---|---|
| Horizontal overflow | **Pass**: 0 of 340 page loads |
| Clipped text (ellipsis or hidden overflow), including long Burmese labels | **Pass**: none found |
| Content hidden behind the capsule bottom navigation (scrolled to the end; 320 Burmese, 390 English and Burmese) | **Pass** on all 10 console screens |
| Keyboard: 25 Tab stops per page on 7 pages, Day and Night | **Pass**: every stop shows a focus ring and none is off-screen |
| Reduced motion | **Pass**: `base.css` collapses animations and transitions to 0.001 ms; the bottom-nav bubble and brand stars have their own rules |
| Skeleton loading | **Pass**: page and section skeletons render while data loads (a first screenshot of Vouchers captured the light skeleton before data arrived; re-checked) |
| Partial errors (all data APIs returning 500, `/me` working) | **Mostly pass**: Home, Hotspot, Activity, Vouchers and ISP payments each say what failed and offer Try again. **Defect A** below. |
| Offline (network cut after load) | **Defect B**: nothing says the device is offline |
| axe | **Defect C**: `color-contrast` on Day `.chip.idle` (4.44:1) on Home, Quick Setup, Network, Hotspot and Activity. No other violations. |
| Touch targets ≥ 44 × 44 (phone widths) | **Defect D**: language switch 38 px tall on every page; help "?" 32 × 32; "Open" header links 37 × 21; map zoom/compass 29 × 29; brand link 32 px wide at 320 px. Inline links inside sentences are exempt and not changed. |
| Request errors | Local-only: `503 /api/v1/pricing` (production returns 200). `404 /api/v1/ap-optimization/proposals` on Changes and approvals is also 404 in production (feature disabled; unchanged). |

## Fixes in this branch
- **A: PPPoE lists stuck on "Loading".** When `/routers` fails, Customers (PPPoE) and the ISP overview showed a skeleton forever. They now show the existing Recovery panel ("Subscriber records could not be read" / Try again, which retries the router read). The Hotspot gateway line shows "Not available" instead of "…". Change: `app/src/pages/workspaces/Services.tsx`. A new test fails on the old code and passes now.
- **B: offline notice.** New `app/src/components/OfflineNotice.tsx`, placed at the top of the console's main region:
  - It is an always-present `role="status"` polite live region, so screen readers hear the change, and it is visually hidden and takes no space while online.
  - It shows a warning note only when the browser reports no network.
  - It never claims MegaMagic is reachable.
  - New text uses key `c.deviceOffline` in English and Burmese.
  - Test: `OfflineNotice.test.tsx`.
- **C: contrast.** Day `--idle` changed from `#5a6e65` to `#54675e`, the same hue, giving 4.92:1 on the chip background. Night is unchanged. axe now reports **0 violations** at 320, 390 and 430 px in Day/Night and English/Burmese.
- **D: touch targets.**
  - The language switch buttons are 44 px tall in the console (`shell.css`) and on the public pages (`landing.css`, cache key bumped to `?v=20261113`). The pill grows by 2 px.
  - The brand link has a minimum width of 44 px.
  - The help "?" and the "Open" header links keep their visual size; a transparent layer extends the tap area to at least 44 × 44, measured with `elementFromPoint` on a Pixel 7 profile.
  - Map controls are 44 × 44 on touch screens (`pointer: coarse`).

Screenshots: `docs/design/acceptance-20261113/` (`fix-offline-my.png`, `fix-customers-en.png`, `fix-signin-header.png`).

## Suites run on this tree
- **App:** lint, `tsc -b`, vitest 408/408, production build.
- **Root:** `test:deployment` 18/18, `test:vercel` 8/8, `npm run build`, `test:critical-ui` 25/25.
- **Go:** no Go changes; CI runs the race tests and the vulnerability scan.
- **Browser re-run on the fixed build:** 204 phone-width page loads, 0 overflow, 0 axe violations; offline notice and recovery verified in English and Burmese.

## Not verified (needs people or devices)
- **Real devices.** iPhone Safari and Android Chrome, including password-manager autofill across the `signin.html` → `/signin` change. Emulation is not evidence.
- **Screen readers.** VoiceOver and TalkBack announcements for navigation, forms, loading, status changes, errors and the new offline notice. The tests above check the markup only.
- **Native Burmese review.** The new string `c.deviceOffline` (and the earlier pending strings) still needs a native speaker. **Pending until a real person completes it.**
- **Signed-in production pass.** The owner should walk the five tabs on a phone.

## Rollback
This is a frontend-only change with no API, migration or gate changes. Revert the PR's squash commit on `main` through a PR, which deploys. `MM_BROADBAND_WRITES_ENABLED` is untouched.
