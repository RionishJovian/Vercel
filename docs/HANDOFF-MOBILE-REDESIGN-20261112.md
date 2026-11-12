# MegaMagic mobile redesign — release candidate handoff (2026-11-12)

Spec: Notion "11 — Approved MegaMagic mobile design, quick setup and PPPoE ISP console".
Owner authorization (Notion, 2026-11-12): design and development are approved. **Merge and production deployment are separate gates.** The owner asked for the release candidate to be presented for a final decision, **not merged**.

## Release candidate
- Branch: `claude/dazzling-feynman-chu5tf` (the cloud session's assigned branch; the brief named `redesign/mobile-ui`, which this session may not push). This cloud container has no Mac checkout, so no worktree was created next to `/Users/aces/GitHub/Mega-Magic`.
- PR (draft): https://github.com/RionishJovian/Vercel/pull/199 — the final commit is the PR head. CI (validate, CodeQL, Go and JS/TS analysis) passed on `0d7b54a`, the last code commit; later commits change documentation only.
- Base: `main` at `e014fb3`. **Merging the PR deploys to production** (AGENTS.md §3).
- Mobile preview (Vercel, branch alias): https://mega-magic-git-claude-dazzling-feynman-chu5tf-magic-2c1b.vercel.app/v2/ — its `/api/v1` is proxied to production (`vercel.json`), so signing in there uses a real production account.

### What this release ships
1. **Frontend (`app/`, `/v2/`)** — the redesign below.
2. **Go 1.26.11 API rebuild.** `go.mod` toolchain and the `Dockerfile.prod` build image (`golang:1.26.11-alpine`, pinned by multi-platform index digest `sha256:ae52b5bc…79b0bb`) move from 1.26.10. CI's govulncheck reports GO-2026-6859 in `net/http@go1.26.10`, fixed in 1.26.11. The API code is unchanged apart from the Magic Dude navigation guide (below), but **the production API binary and image are rebuilt**, so a merge replaces the running API container as well as the web container. The earlier 1.26.10 digest no longer matched either current 1.26.10 digest, which suggests the tag was republished after the original pin.
3. **Network form fix** (`app/src/pages/network/NetworkEditor.tsx`): a re-read of the same router inventory no longer wipes values the owner has typed into a network task form, or the result just shown. CI caught it once as `network/addresses.test.tsx` losing a VLAN name; it did not reproduce locally (4 full-suite and 24 parallel single-file runs), so the fix removes the identity-based reset that matches the symptom. It passed CI on the next run.
4. **Magic Dude navigation guide** (`internal/handler/magic_dude_prompt.go`) names the new tabs and screens, so the assistant does not direct owners to removed tabs.
5. No database migrations. No write-gate changes: **PPPoE (broadband) writes stay disabled** (`MM_BROADBAND_WRITES_ENABLED` off) until their separate operational acceptance; the ISP screens only read, and say when writes are locked.

## What changed in the frontend
| Area | Files |
|---|---|
| Tokens: emerald, mint, ivory; Day = ivory, Night = deep emerald | `src/styles/tokens.css`, `base.css`, `shell.css`, `pages.css` |
| Logo A, the wand with sparkling stars (**approved by the owner 2026-11-12**) replaces the header wand and the sign-in logo stage | `src/components/Brand.tsx`, `SignIn.tsx` |
| Navigation: 5 bottom tabs on phones, grouped left rail ≥900 px; skip link | `src/lib/nav.ts`, `src/components/Shell.tsx` |
| Home: dual Hotspot/PPPoE command, alerts, setup progress, per-card failures | `src/pages/Home.tsx` |
| New workspaces | `src/pages/workspaces/{Setup,Services,Operations,data}.tsx` |
| Shared status, evidence-time, metric, recovery and segmented components | `src/components/ui.tsx` |
| Layout-matched loading skeletons; static boot frame | `src/components/Skeleton.tsx`, `index.html`, `App.tsx` |
| Strings (EN + MY first draft) | `src/i18n/workspaces.ts` |

New routes: `#/setup`, `#/hotspot`, `#/isp`, `#/customers`, `#/net`, `#/activity`, `#/approvals`, plus `#/portal` → `#/vouchers/guest`. Every earlier route still resolves. `#/home?router=X` redirects to `#/net?router=X`, where the port panel lives. Jellyfin moved from the bottom bar to More → Business and the desktop rail.

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

## Loading skeletons (spec 11: "loading states preserve layout and announce progress")
- Skeleton shapes reuse the CSS classes of what they stand in for (`.metric`, `.linkrow`, `.step`, `.svc-card`, `.sub-row`, `.site-map`), so the loaded content takes the same space. Text-line placeholders are sized from font size × line height, with Burmese's taller line height applied under `:lang(my)`.
- Each loading region is one `role="status" aria-busy="true"` with a translated "Loading" (`ဖွင့်နေသည်`); the shapes are `aria-hidden`. The older pages' generic block skeleton now announces too. Shimmer stops under reduced motion.
- App boot and the route-level fallback render the real shell with a per-screen layout (home, steps, hub, page). `index.html` paints a static, theme-aware frame before the JavaScript arrives; it uses the system font on purpose so it does not start a web-font download that competes with the app bundle.
- Older pages (vouchers, router detail, topology, etc.) keep their own block heights; they announce, but are not individually shape-matched.

### Slow-connection evidence
Production build (`vite build`, served by `vite preview`) against the local API, 390 × 844, Chromium with Slow 3G emulation (400 ms latency, 400 kbit/s), 4× CPU slowdown and an extra 1.5 s on every API reply. Each route was loaded fresh. CLS = cumulative layout shift (web.dev "good" is ≤ 0.1). "Before" is the previous release candidate `606b9f3`.

| Route | CLS before | CLS after | Loading announced |
|---|---|---|---|
| `#/home` | 0.091 | 0.005 | yes |
| `#/setup` | 0.000 | 0.000 | yes |
| `#/isp` | 0.093 | 0.038 | yes |
| `#/net` | 0.075 | 0.043 | yes |
| `#/activity` | 0.011 | 0.017 | yes |
| `#/customers` | 0.014 | 0.015 | yes |
| `#/hotspot` | 0.013 | 0.005 | yes |
| `#/approvals` | 0.000 | 0.000 | yes |

- Burmese, same conditions: Home 0.029, Setup 0.000, ISP 0.028, Network 0.035; announced as `ဖွင့်နေသည်`.
- Before this change no route announced loading (the old skeleton was `aria-hidden`).
- First paint: the static frame is on screen at about 1.0 s; the previous build showed a blank page until the bundle ran (about 5 s). Image: `docs/design/skeleton-slow3g-home.png` (1.5 s, 7.5 s, 10 s).
- Time until Home figures show: about 9.7–10.1 s versus 9.4 s before. The difference is one additional Home request and dependent requests, not the skeletons. A first version of the boot frame cost about 1 s because its hidden text pulled the web fonts early; that was fixed.
- No horizontal overflow on any route.
- These are emulated conditions on one machine, not a measurement on a real midrange phone or network.

## Test evidence (this session)
- `app`: `tsc --noEmit`, `eslint src`, `vitest run` (59 files / 392 tests, including Home, Quick setup, Customers, subscriber-state and skeleton-accessibility tests) and `vite build` pass.
- Go 1.26.11: `go build ./...`, `go vet ./internal/handler` and `go test ./internal/...` pass locally; CI runs the race tests and govulncheck (govulncheck cannot reach `vuln.go.dev` from this container).
- Root: `test:deployment` 18/18 and `test:vercel` 8/8 pass. In this container `test:critical-ui` was 21/22 (`scripts/test-media-publishing-ui.mjs` could not launch Chrome); it passes in CI.
- Browser checks used a local Go API (`cmd/server`, built with Go 1.26.11) with a disposable Postgres (all migrations applied), NATS and Redis, signed in as a seeded owner:
  - 320, 390, 820 and 1440 px; English and Burmese; Day and Night. `scrollWidth - innerWidth = 0` on every screen checked.
  - Sign-in → Quick setup → Open Sites → create a site (one POST, 201; a second click sent no duplicate) → browser Back → step 1 shows Done with the site name as evidence.
  - Skip link is the first Tab stop and moves focus into the content.
  - With a gateway record that has no live router, Network shows "Not confirmed" and the ports panel offers "Try again". The ISP overview shows real (zero) records and the server's subscriber-write lock.
- Not tested in a browser: voucher generation and installation, guest captive login, PPPoE subscriber creation and sessions. Each needs a reachable router through Magic Hub.

## Not verified — open acceptance gaps
1. **Real router, Hotspot:** gateway connection through Magic Hub, Internet setup, Hotspot setup, voucher generation, a customer logging in on a phone, and the session and usage appearing in Guests and Customers. No router was contacted in this work.
2. **Real router, PPPoE:** create a subscriber, assign a plan, see authentication and session, renew, suspend and record payment. Blocked by design until the separate operational acceptance; production writes stay off.
3. **Screen reader:** no VoiceOver or TalkBack pass has been made. Automated checks covered roles, labels, the skip link and loading announcements only; they do not establish screen-reader accessibility.
4. **Burmese review:** `src/i18n/workspaces.ts` Burmese strings are a first draft written without a native speaker; layout was checked, wording was not.
5. **Real devices:** layout and loading were checked in emulation only; no physical midrange phone test.

## Known gaps and API notes
- No cross-router PPPoE summary endpoint. `#/isp` and `#/customers` read `/routers/:id/broadband/accounts` once per gateway; that is fine for the one-router-per-client rule.
- Hotspot live-session counts are not on Home. They need a live router read (`/routers/:id/observations`) and are shown in Guests.
- No list endpoint for pending network changes across routers. Approvals shows AP-optimization proposals and the first router's `/devices/:id/changes` record.
- The topology Map/Tree views are the existing implementation, restyled only.
- The favicon and the `web/` marketing pages still use the earlier mark; logo A is in the console only.
- The local dev proxy in `app/vite.config.ts` forwards `Host: localhost:8080`, which the API's same-origin check rejects. This session used an uncommitted local override that forwards the browser's Host.

## Rollback
### Previous release (verified 2026-11-12)
- `e014fb3` is the last release on `main`: run 617 (2026-11-08) validated, built, pushed and deployed it ("Verified release e014fb3…").
- Its images are in GHCR and pullable (checked by digest and by tag, linux/amd64):
  - `ghcr.io/rionishjovian/vercel/mega-magic-api@sha256:31baa43e5c27442c3290f5fac2c64e3a99c1f90841d9bd80b7eb04903be3d81e`
  - `ghcr.io/rionishjovian/vercel/mega-magic-web@sha256:5586bfd1ce4552ee2975280e4c798bb4a916a2f0c2d76f7a91d0e878b2732d94`
- That deploy reported its own rollback copy at `/root/mm-rollout-backups/ci-e014fb3dc3c09d429dd356caae67143001ed4ec6-20261108T175512Z` (not inspected: this session has no SSH access to the VPS).

### Rollback tag
- **Not created on GitHub.** The session's git proxy refuses every push except the session branch; the tag exists only in this container. Owner command (from any clone):
  `git tag -a pre-mobile-redesign-20261112 e014fb3dc3c09d429dd356caae67143001ed4ec6 -m "Last release before the MegaMagic mobile redesign"` then `git push origin pre-mobile-redesign-20261112`, and check with `git ls-remote --tags origin pre-mobile-redesign-20261112`.

### If the merge deploys and must be undone
1. Fast, on the server: the release-candidate deploy writes `/root/mm-rollout-backups/ci-<new sha>-<time>/` containing the compose file that points at the `e014fb3` images, the config, the previous API binary, `previous-image-ids` and a database dump. Copy that `compose.yaml` back to `/opt/mega-magic-go-restored-20261018T100554Z/deploy/compose.go-restore.yaml` (and `restore-bin` to `restore-bin/mega-magic`), then run `docker compose -f deploy/compose.go-restore.yaml up -d --no-deps --no-build --pull never api web` in that directory. The `e014fb3` images are still on the host and in GHCR.
2. Source: revert the merge commit on `main` through a PR. Every push to `main` redeploys, so this also rebuilds and releases the previous code.
3. Automatic: if the new release fails `/ready` or the header checks, the deploy script restores the previous API and web itself.
- No database migrations are included; the database needs no rollback.

## Next concrete action
The owner reviews the mobile preview and PR #199, pushes the rollback tag (command above), and decides whether to merge. After merging, confirm the deployed API image label `org.opencontainers.image.revision` equals the merge commit. Then run gaps 1 and 5 on the hAP ax2 and a phone, and record the results in `docs/HARDWARE-TRIAL-RESULTS-*`. Arrange the screen-reader pass and the Burmese review.
