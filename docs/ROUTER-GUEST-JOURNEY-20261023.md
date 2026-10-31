# Integrated router-to-guest candidate — 2026-10-23

> **Note (2026-11-01):** the old console this document describes (`web/app.html`, the vanilla `web/v2` and their scripts) was removed. The console is now the React app in `app/`, served at `/v2/`. This is a dated record; file names and UI steps below may no longer exist.

Status: deployed to megamagic.tech on 2026-10-23 after the owner explicitly requested “deploy now”. The server rollout is verified; physical RouterBOARD/phone certification remains incomplete because the spare hAP ax2 is unavailable per the owner's latest reply. The complete original goal remains open.

## Coherent source

The release is current Hub baseline `9b17d9118689492a4a7282132bcd9620262e2ce7` plus the selectively integrated voucher candidate and targeted fixes. Current Hub route registration/callbacks and tenant boundaries are preserved. No whole-tree replacement from the older voucher branch was made. Design-alternatives prototypes are excluded.

The frozen source, per-file SHA256 manifest and evidence are in `/Users/aces/Documents/Codex/2026-10-21/asd/work/journey-integration-20261023/`. `release-2/` contains the tested functional source; `release-2-manifest.json` identifies every changed functional source/test/asset file. Additional progress/QA/asset notes do not change the compiled code.

## Implemented and verified locally

- Existing Magic Hub connect/site workflow and initial LAN/DHCP/HotSpot configuration remain integrated and covered by the combined regression suite. Actual router connectivity is not inferred from saved cards or tests.
- Batch issuance commits original identities before router I/O. Lost responses and partial installations retain the same batch/codes; retry verifies the original allowance. Unknown installations are retained and excluded from printing. Issuance/printing does not activate vouchers.
- Router-local CHAP uses raw ID + code + raw challenge, with the copied local md5.js helper. PAP remains supported. Consent, malformed codes, duplicate submissions and retry are covered by executing the generated page script.
- First-use evidence must be router-persisted epoch + real unicast MAC. A live session/usage without that evidence is reported unverified; sync never invents scan-time activation or a new deadline. Accounting covers the full inventory in bounded pages, validates all pages first and locks the ledger update.
- The login hook stamps once; the scheduler never creates anchors. Epoch and duration units use documented tonsec conversion. Guard-object readback handles multiline source, preserves quoted-literal differences and verifies policy/profile content. Used unanchored legacy vouchers are refused before any user-profile write; successful migration requires the intended server/profile readback.
- Activated voucher value is separate from cash/bank settlement and from unsold issued value. Original expiry/allowance are retained on repeat accounting/reconnect in local coverage.
- The owner preview and router deployment share one renderer. Published-site policy wins over private drafts; foreign businesses are denied. Saved design changes invalidate reviewed deployment confirmations before writes.
- Owner-selected emerald glass guest design is implemented with locally embedded wallpaper/mark. Default complete HTML is 18,358 bytes. Preview cannot submit a real voucher. The approved management Jade Glass theme is retained.

## Integrated baseline evidence (`ddb9031`)

- Isolated functional source: 672 top-level backend tests pass, 0 fail, 12 gated skips (`release-2-internal.jsonl`). These skips are not counted as certification.
- Critical UI: 62 pass, 0 fail (`combined-v3-ui.log`).
- Production-web audit: all six surfaces pass; whitespace check passes.
- Linux AMD64 static Go artifact SHA256: `fc70bf18dc1eb87386771f11166d76fb503d4462824ae4b56d329b34934024e2` (`megamagic-linux-amd64`).
- Actual browser: phone layout, 320 px no-overflow, desktop layout, image decode, preview non-consumption and console checks. Screenshots retained alongside the test evidence.
- RouterOS behavioral/model tests are simulations; they do not execute the scripts on RouterOS. Formal reference-image QA remains pending as recorded in root `design-qa.md`.

An earlier isolated run encountered a stopped disposable PostgreSQL fixture and an unprivileged UI fixture listener. Neither run is treated as passing. After confirming/restarting the local fixture and granting the required loopback access, the final runs above passed. One new test's duplicate policy setup was fixed; production schema was not changed.

## Required physical acceptance (still open)

1. Fresh read-only identity/version, WAN, radio, DHCP/DNS, Hub handshake and management checks on the spare router; compare app account/site/peer identity. Do not reset or delete saved devices blindly.
2. Back up the exact selected HotSpot profile/directory and any scoped settings before intentional writes. Verify the clock is actually correct and synchronized, not merely a plausible epoch; verify on-login variable scope/script permissions and native time/duration readback on the actual RouterOS version.
3. Connect/configure through the app and deploy its reviewed saved portal; verify asset/page/profile readback and rollback. A browser preview is not proof of RouterOS serving it.
4. Issue and print one finite test batch; prove it stays unused. Phone joins the guest SSID with mobile data off and opens the router-served page with the selected design.
5. Enter one code. Prove external guest HTTP/HTTPS traffic using that Wi-Fi connection; a successful router command or active session alone is insufficient.
6. Read back the exact first-use epoch/MAC. Confirm original first-use/expiry and activated voucher value in the app, with no cash-settlement inference or double activation.
7. Disconnect/reconnect: same anchor/deadline and decreasing remaining allowance. Exercise elapsed expiry including an offline interval; observe rejection/session removal after the guard's configured 60-second scheduling interval. Exercise a small data cap independently.
8. Prove expiry/reconnect enforcement during an app/Hub outage, then recovery/accounting without invented anchors or replacement codes. Restore temporary lab endpoints/listeners/settings; no relay is currently authorized or running for this candidate.

## Rollout gate and rollback

Do not deploy this candidate as hardware-certified while the checks above are outstanding. Before an authorized rollout, freshly identify the running API/image/config/source compatibility, export a database backup and retain the running image and current web files. Apply the additive transactional `20261021180000_guest_voucher_issue_batches.sql` migration before enabling the candidate against an enabled guest-voucher configuration; validate schema readiness and tenant restrictions. Do not auto-run migrations at API startup.

Deploy only the tested coherent artifact with the existing Compose/proxy topology, preserving keys, accounts, history and other services. Verify authenticated Hub routes, portal preview, batch recovery and reporting after rollout; guest-internet proof still requires the physical device. Revert the API image/web files on a failed smoke check. Keep additive batch tables/rows on rollback: dropping them would destroy recovery evidence. Router profiles/files need their separately verified rollback. No Caddy/Traefik migration, new service installation or network exposure is part of this candidate.

## Historical release-preservation check — before rollout, 2026-10-23

All 42 functional source/test/asset hashes still match the isolated tested candidate. The four additional owner-context/QA/progress/artwork notes are included in the scoped local commit. The separate design-alternatives directory remains excluded. A fresh SSH public-key attempt to the authorized VPS was rejected (publickey/password); no remote inspection or change succeeded. Restored authorized server access and the unavailable spare router are required before final live acceptance.

## Clock-alignment follow-up — 2026-10-23

Before installing guard objects or voucher users, provisioning now compares the router epoch to the server's actual command-read window, allowing at most 30 seconds outside that window. Plausible but wrong dates (including year offsets) are rejected with a time-synchronization message before writes. The operation does not set the router/server clock or configure NTP. It does not certify clock behavior after issuance or replace hardware acceptance.

The RouterOS package passed 166 top-level tests with race detection and eight explicitly gated hardware skips. New coverage verifies plausible past/future rejection before any further router command, exact skew boundaries, command latency and server clock reversal. The Go model fixtures now use current valid setup timestamps; their sequential retry test is explicitly labelled sequential, not a concurrency proof. Model tests remain distinct from real RouterOS execution.

Evidence: `clock-window-routeros.jsonl`, `clock-guard-release/` and `clock-guard-manifest.json` in the same local evidence directory. The updated isolated Linux AMD64 artifact SHA256 is `73a14cee5a0cdde4c6dd536d2004242dbf5b2d0833cb59f970fa6d368ad45a92` (`megamagic-linux-amd64-clockguard`); this supersedes the earlier binary for a future rollout. The full 672-test backend/62-check UI results above belong to the integrated parent; this follow-up was verified with the affected package plus the server build. No new claim of full hardware or production acceptance is made.


## Authorized production rollout completed — 2026-10-23

This supersedes the historical SSH-access and not-deployed status above. The authorized Hostinger root web console provided access; no password or key was stored in this repository. The running API binary matched the proven 9b17d91 baseline before rollout. The final deployed frontend source is c9ec712542c30324258c49d18472ae5c8f438f0d; compiled API source remains 8dbb1eb because subsequent changes only reconcile the live frontend and its tests.

The live frontend contained additional owner workflow changes. Those were preserved: task navigation, distinct Internet/Jellyfin pages, the existing workflow extension and site-assignment hooks. Existing verified setup, batch print/recovery, revenue and role behavior were retained. The final source passed all 62 critical UI checks, zero failures/skips, recorded in `live-merge-ui-certified-final.log`. Hub coverage explicitly distinguishes available/unavailable capability responses and checks the site-assignment DOM hook.

Deployment evidence (under the same local evidence directory):

- `DEPLOYED-20261023.json` and `DEPLOYED-20261023.md`: exact artifacts, checks and rollback.
- API image `mega-magic-api:e9b8cac`; web image `mega-magic-web:c9ec712`.
- VPS build with Go1.26.5 reproduced the tested static Linux AMD64 binary exactly: SHA256 `73a14cee5a0cdde4c6dd536d2004242dbf5b2d0833cb59f970fa6d368ad45a92`.
- Public `/app.html` SHA256 `d4931aa64cffdb16d1c92389e053d53656aed6ae6b9784acd951f2be311b1bb5`; public Jade stylesheet matches source exactly.
- Database backup `/root/mm-release-8dbb1eb/backup/database.dump`, SHA256 `74577d3366784f1a1c6f9b154618a5fec506e7884095edc826b9fc3f00bf682c`; its pg_restore catalog was successfully read before migration.
- Additive batch migration committed. Both batch tables have forced RLS, their single tenant policy and enabled guard trigger. API schema-readiness preflight passed.
- Public health/readiness successful; database, Redis and NATS all report OK. Final API/web services run with the existing Compose/proxy topology.
- Authenticated browser readback verified saved Hub site assignment, voucher form/inventory and separate activated-value reporting. Existing finite test voucher remained unused: one recorded voucher, 1,000 MMK face value, zero activations, zero activated value. No new code issued, payment approved, or router configured during deployment.

Rollback preserves customer and recovery data: restore `/root/mm-release-8dbb1eb/backup/compose.go-restore.yaml` to the existing Compose path, then use `docker compose -f /opt/mega-magic-go-restored-20261018T100554Z/deploy/compose.go-restore.yaml up -d --no-deps --no-build --pull never api web`. Original images are pinned as `mega-magic-api:rollback-8dbb1eb` and `mega-magic-web:rollback-8dbb1eb`. Leave additive batch tables/rows intact. Do not restore the database merely to roll back application images.

### Resume at physical acceptance

Do not repeat candidate integration or deploy the older voucher branch. Begin with the eight physical acceptance steps above once the owner confirms spare-router availability. First compare the live owner/site/device/Hub identity and actual router clock; inspect existing profiles/files before writes. The deployed server does not update an offline router's stored login page. Actual phone internet, first use, elapsed/data expiry, offline enforcement and reconnect remain unproven. Do not restart a historical relay or reuse its old approval window.
