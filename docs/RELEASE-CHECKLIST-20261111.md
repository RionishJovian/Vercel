# Release checklist: 2026-11-11/12 stabilization set (PRs 191, 193, 194, 195, 196, 197, 198)

**Status: NOT approved, NOT released.** This is a checklist for a separate release review. Every merge to `main` that passes `validate` builds images and deploys to the production VPS automatically (AGENTS.md section 3), so *merging is the release*. Nothing here authorizes a merge, a deploy, a router change or a production configuration change. Simulations and passing CI are not hardware acceptance.

Evidence classes used below: **builder** (the author's own tests), **independent** (a separate reviewer re-ran or re-derived it), **CI** (GitHub Actions), **production observation** (read-only look at the live host), **hardware** (a real RouterBOARD and phone). Only the last one counts as hardware acceptance, and it has not happened.

## 1. What is in the set
| PR | Change | Schema / migration | Config the live host needs |
|---|---|---|---|
| 191 | Fail-closed `/ready`, request logs use route templates, `/api/v1/version` reports the running commit, bounded DB/Redis probes, server honours `server.host`, deploy-script HTTP timeouts and a commit check | none | `server.host` must be reachable by the web container (verified `0.0.0.0` on 2026-11-11) |
| 193 | Production refuses to start without a valid 32-byte integration key; webhook signing secrets become write-only (`has_secret`) | none | **`provisioner.applier.credential_key` must be a valid key in `config.go-production.yaml`** or the API will not start |
| 194 | Legacy `POST /api/v1/vouchers` rejects `guest-internet` before any DB write | none | none |
| 195 | Documentation of automatic deploy, automatic migrations and rollback limits | none | none |
| 196 | Local workflows, frontend contract, Go 1.26.11 (GO-2026-6859), Vite 6.4.6, CI duplicate app build removed, acceptance records | none | none |
| 197 | Shared-NAT public rate limiting, exact proxy trust, web Caddy trusted-proxy block; owner portal routes excluded from the guest budget; bounded per-account login-lock state | none | optional, see section 5 |
| 198 | Ethernet port diagrams for 13 more exact models (15 total), bundled WebP artwork, reported board identity takes precedence | none | none. **Artwork permission is unresolved, see section 9** |

No PR in the set touches `db/`, `migrations/` or any `.sql` file (checked with `git diff --name-only origin/main...pr/N`).

## 2. Merge order and gates
**There is no approval gate in GitHub.** On 2026-11-11 the `Production` environment had no protection rules (no required reviewer, no wait timer, no branch policy) and `main` requires only the `validate` check with 0 reviews, so the owner's separate approval is the only control. Consider adding a required reviewer to the `Production` environment before the first merge (an owner action in GitHub settings; not done).

Merge one PR at a time, only after separate owner approval, and wait for the automatic deploy and the section 6 checks before the next one. Suggested order (lowest risk first): 195, 194, 193, 191, 196, 198 (only after the artwork question is answered), 197. Reason: 193 can stop the API from starting if the key is absent, so it should be alone in its deploy; 191 changes the deploy script itself, so its first deploy runs the new commit check; 197 changes request limiting for every public route and should go last.

For each PR, immediately before merge confirm: head SHA equals the reviewed SHA, `validate`, both CodeQL jobs and Vercel are green on that exact SHA, no new commits since review, and `main` has not moved (otherwise re-run the combined simulation).

## 3. Before the first merge
- [ ] Fresh database backup exists and was restore-listed: `mm-db-backup` timer ran, and the offsite copy is current (`journalctl -t mm-offsite-copy`). The deploy script also writes its own `pg_dump` and validates it with `pg_restore -l`, but that is per release, not a substitute for a restore drill.
- [ ] Confirm the credential key is present and valid without printing it: run the repo's `config-check` against the live config in a disposable container, as the deploy script does before stopping the old API. A failure there aborts the deploy before downtime.
- [ ] Record current live image digests and `/api/v1/version` output so a rollback target is known.
- [ ] Confirm the write gates table in AGENTS.md section 5 against the server environment; none of these PRs change a gate.
- [ ] Choose a low-traffic window. No customer data is altered by these PRs, but the API restarts (brief interruption; rate-limit counters reset).

### Read-only production preflight (observed 2026-11-11 18:28Z; a dated observation, repeat it on release day)
- Running API and web images both carry revision `e014fb3` (current `main`); compose digests equal the running image IDs (`31baa43e5c27...` API, `5586bfd1ce45...` web). `/ready` is `ready`; `/api/v1/version` still reports `development` and has no `commit` field (PR 191 not deployed).
- The live binary's own `config-check` reports the configuration valid. Production `ENVIRONMENT` is `production`.
- PR 193 requirement: `PROVISIONER_APPLIER_CREDENTIAL_KEY` is present in the API environment and decodes to 32 bytes (shape only; value not read). The YAML `credential_key` is empty, so the key comes from the environment. **Not verified:** PR 193's new validator has not been run against the live config. The deploy script runs the new image's `config-check` before stopping the old API, so a failure aborts before downtime.
- Write gates in the API environment: `MM_NETWORK_CONTROLS_ENABLED`, `MM_NETWORK_ADDRESS_CONTROLS_ENABLED`, `MM_MANAGEMENT_MIGRATION_ENABLED`, `MM_GUEST_ACTIONS_ENABLED`, `MM_DEVICE_CONTROLS_ENABLED`, `MM_NETWORK_WIFI_CONTROLS_ENABLED` are `true`; `MM_BROADBAND_WRITES_ENABLED`, `MM_FIRMWARE_UPGRADE_ENABLED` and the WebFig variables are unset (off). This matches AGENTS.md section 5. None of the seven PRs changes a gate.
- Backups: `mm-db-backup` ran 2026-11-11 03:10Z (919,976 bytes); `mm-offsite-copy` ran 03:43Z and reported `ok`, **but its log shows an S3 status 501 on the first attempt before "Attempt 2/3 succeeded"** (investigate the storage endpoint); the weekly restore drill passed on 2026-11-08 ("the dump restores and matches production"). Newest per-release backup directory: `ci-e014fb3...-20261108T175512Z` (database dump, compose, config, previous binary and previous image IDs).
- Rollback images: the previous API (`6ad60c13d442`) and web (`318cf0893933`) images are present locally on the host. Disk 37% used; memory adequate.
- **Unverified:** that GHCR pulls and the first automatic deploy of the new images succeed; the host Caddy edge configuration; that the offsite copy can be restored from (the drill restores the local dump).

## 4. Migrations and rollback limits
- This set adds **no** migrations. If a later change does, `deploy-vps.sh` applies pending files with `mm-migrate up` **after** the backup and **before** the new API starts, each file in one transaction.
- Rollback restores the previous compose file, binary and edge headers and recreates `api` and `web` from the previous images. **It does not reverse committed migrations and does not restore the database.** Migrations must stay backward compatible with the previous release.
- A database restore is a separate, manual, owner-approved operation from the per-release dump. Never done automatically.
- PR 193 in particular: reverting the image restores old behaviour, but webhook secrets already stored are unchanged; the API simply stops returning them.

## 5. Proxy prerequisites for PR 197 (production behaviour is unchanged until these are done)
Production today is host Caddy 2.11.7 to web Caddy 2.10.2 to the Go API, and the API sees every client as the web container peer. After merging 197 **alone**, behaviour is: per-browser budget of 30/min per scope (using the signed rate cookie) plus aggregate caps of 600/min (auth) and 6,000/min (guest) shared by **all** clients, because the real client IP is still not forwarded. That is not the NAT fix.

To get real client IPs, all of these are needed, and none are done:
- [ ] The Docker peers must be stable and exact: the host-Caddy-to-web peer IP and the web-to-API peer IP, recorded from a read-only inspection after the intended restart pattern. Do not trust a Docker subnet, hostname or `X-Real-IP`.
- [ ] `server.trusted_proxy_ips` set in the live `config.go-production.yaml` to exactly the web container peer(s). The repo default is loopback only.
- [ ] `MM_EDGE_PROXY_IP` for the **web** container set to the exact host-to-web peer. **Important:** `deploy-vps.sh` only rewrites the two image lines of the live compose file and does not re-render the repo template, so the new `environment:` block in `compose.go-restore.template.yaml` does not reach production by itself. This must be a deliberate manual edit of the live compose or env file, with its own backup.
- [ ] A rehearsal of host Caddy, web Caddy and API restart order, proving the peers do not change after a restart (otherwise client IPs silently fall back to the web peer, the safe but unhelpful result).
- [ ] Capacity evidence with a production-shaped proxy and 50 or more distinct sessions behind one NAT (record G in the acceptance sheet), not run against paying customers.
- [ ] Decision on scaling: counters are process-local and reset on restart; more than one API replica needs a shared limiter.

## 6. After each deploy (production observation)
- [ ] `GET /ready` returns 200 with `{"status":"ready"}` and all three checks `ok`; with a dependency down it must return 503 and a JSON body without raw error text.
- [ ] `GET /api/v1/version` `commit` equals the merged SHA (the deploy script asserts this too).
- [ ] API container image label `org.opencontainers.image.revision` equals the merged SHA; served console assets hash-match a local build.
- [ ] Public security headers still match the edge policy; sign-in works from a real browser.
- [ ] Log sample shows route templates (for example `/api/v1/portal/:deviceId/config`), no voucher codes, router ids or query strings.
- [ ] Webhook list response has `has_secret` and never `secret`.
- [ ] Legacy `POST /api/v1/vouchers` with `plan_key: guest-internet` returns 400 and the router-verified `/api/v1/guest-vouchers` path is unchanged.
- [ ] After 197: a burst from one browser returns 429 with `Retry-After` after 30 requests while a second browser is still served.

## 7. Software release checks versus blocked hardware acceptance
**Software release checks** (can be completed without a router): exact-head CI green, combined merge and test run, production preflight, backups and rollback images confirmed, post-deploy checks in section 6. These establish that the code builds, passes its tests and deploys; they do **not** establish that any router, phone or hotspot works.

**Hardware acceptance (BLOCKED, NOT RUN):** everything below needs a powered, owned RouterBOARD and a real phone.
Router and phone acceptance stays **BLOCKED**. Per the acceptance record, RB01 to RB16 are **NOT RUN**. In particular, no real HTTPS captive-browser, physical phone or RouterBOARD capacity test has run, and iOS captive-network-assistant cookie behaviour is unverified (cookie-less clients fall back to the aggregate cap). For PR 198 specifically, no port diagram has been compared with a real router: the real `/system resource` board name captured from the owner's hAP ax2 on 2026-10-29 was `hAP ax^2` (matches), but the board-name strings of the other 13 models are unconfirmed (see section 9), socket-to-interface correspondence and link states are unverified, and renamed or SFP ports are not drawn. Router identity: the physical hAP ax2 is believed to be router `595dc44a` in business `34ec60f9`, last Hub handshake 2026-11-11 08:37:08Z; this rests on a timestamp match and needs the router's own WireGuard key to confirm.

## 8. Known residual risks (not fixed)
- Rate limits are process-local; a hostile device behind the same NAT can still use up the shared aggregate cap for its neighbours.
- The source table can still be filled by 16,384 distinct IPv4 addresses or IPv6 /64 prefixes within a minute, after which new sources are refused until the window ends.
- Media stream/catalog requests share the 30/min per-browser budget with the portal; real playback was not measured.
- `GET /api/v1/portal/<non-UUID>/config` returns 500 instead of a client error (observed locally; pre-existing, not changed by this set).
- Endor package-risk review for the Vite bump returned UNKNOWN; it is not an approval.

## 9. PR 198 model identity and artwork (investigated, not resolved)
**Identity flow.** The app picks a diagram from `snapshot.resources.board_name`, which the Go telemetry reads from RouterOS `/system resource` `board-name`; there is no separate `/system routerboard` model field. The reported board wins even when unsupported, and the saved name is used only when no board is reported. Checked with the PR's matching function: the real capture `hAP ax^2` selects the hAP ax2 diagram (the `^` is stripped), and `hAP ax^3`, `hAP ax lite`, `hAP ac^3`, `RB760iGS`, `L009UiGS` select theirs; an unsupported board such as `CHR` correctly shows no diagram even when the saved name says hAP ax2. **Risk:** the PR's tests use RouterBOARD model codes (for example `RB4011iGS+RM`, `CRS328-24P-4S+RM`, `CRS310-8G+2S+IN`, `CCR2004-16G-2S+PC`), while `board-name` on real devices may be the shorter marketing or model name. With plausible strings (`hEX S`, `RB4011iGS+`, `CRS328-24P-4S+`, `CRS310-8G+2S+`, `CCR2004-16G-2S+`) the function shows no diagram. This fails safe (no wrong picture), but several of the 13 new diagrams may never appear. MikroTik documentation says `board-name` is the RouterBOARD model name and its own sample shows a full model code, while the owner's real hAP ax2 reports `hAP ax^2`, so the format is not uniform. **Needs physical confirmation or captured samples** for each model: the exact `board-name`, socket positions against the real panel, interface naming, and link-state mapping. Recommended before release: either capture real `board-name` values or accept that unmatched models show the text interface list.

**Artwork terms (unresolved permission question).** The images come from the "Visio and SVG files for homepage" share linked from MikroTik's Downloads page under "Drawings for VISIO", described as "for use in schematic drawing applications, such as DRAW.io or VISIO". No usage terms are stated there. MikroTik's brand guidelines say many uses "will require a license first" and do not address product images; the software EULA covers the software and its printed materials, not website images. MikroTik's site footer shows only a privacy link; no website terms of use were found. The share page itself could not be read in this review (it returned only a title), so any notice inside it is unchecked. Open questions for MikroTik (brand contact `marketing@mikrotik.com`): may these drawings and product photos be redistributed inside a commercial hosted application; may they be cropped, recompressed and overlaid with status markers; is attribution or a trademark notice required; and does use alongside the MikroTik name imply affiliation. `main` already ships 27 product photos and two panel images; PR 198 adds 13 more, so the question exists independently of this PR. This is a documentation finding, not legal advice.
