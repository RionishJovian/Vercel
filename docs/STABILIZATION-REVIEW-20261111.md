# Stabilization review and evidence — 2026-11-11

**Source work completed for review; no main merge, production deployment, production configuration change, customer-data change or RouterOS write.** Hardware acceptance is **NOT RUN**. A green PR is not release approval: `main` automatically deploys.

## Source and requirements reconciliation

- Authoritative repository: `RionishJovian/Vercel`; reviewed main `e014fb3dc3c09d429dd356caae67143001ed4ec6`. Original `/Users/aces/GitHub/Mega-Magic` was clean at `620f2d2`; it was preserved while branches were fetched and work ran in isolated temporary checkouts. No old repository/history was substituted.
- Read `.repo-identity.json`, consolidation instructions, current AGENTS/RTK, actual source/workflows/deploy script, the five PRs and their discussions, issue #192, and the actual VPS proxy/release evidence before changes.
- Read Notion tracker, procedure/evidence pages and domains 01–06. Page 04's N19 decision is dated 2026-11-08; most other pages retain October material and unverified/historical repository, role and UI references. Current explicit owner instructions and AGENTS govern source/release/UI conflicts. Retained requirements include R07/R09/R13 safeguards, Internet voucher first-use/accounting/device/expiry acceptance, tenant isolation, Internet/Jellyfin separation and truthful N19 capability evidence. [Requirements](https://app.notion.com/p/3f2489e3dbb681cc9d2de66befe42255), [QA procedure](https://app.notion.com/p/3f8489e3dbb681b9ba1dc45967d3cab2), [N19](https://app.notion.com/p/3f8489e3dbb6810d9261e2bd3acbab64).

## PR reconciliation

| PR / tested source head | Review result and limits |
|---|---|
| [#191](https://github.com/RionishJovian/Vercel/pull/191) `a4b5f39276716dc747af44d5083d33a969af3b1f` | Preserve fail-closed readiness, URL redaction and runtime revision. Reviewer found ineffective Redis context deadline; fixed with real stalled-socket regression. PostgreSQL readiness now has a bounded shared in-flight probe even if the driver ignores cancellation; no per-poll accumulation. All release HTTP checks have timeouts. Configured IPv4/IPv6 listen host is honored; default production behavior retained. |
| [#193](https://github.com/RionishJovian/Vercel/pull/193) `64f21897a0d745042aefb00d6a25dd7efed1600e` | Valid production integration encryption key required independently of router writes; webhook API secret remains write-only. Public serialization does not mutate notifier signing data. No browser client reading webhook secrets found. Production key presence confirmed without exposing value; no rotation or migration performed. |
| [#194](https://github.com/RionishJovian/Vercel/pull/194) `5f87c85a7826b46d9a521e2d16f42e4839cd7384` | Reserved `guest-internet` rejected before legacy DB I/O. Router-provisioned batch path, shared internal issuer, retry/printing and first-use safeguards retained. Real RouterBOARD issuance/readback still needs acceptance. |
| [#195](https://github.com/RionishJovian/Vercel/pull/195) `e76e047531a625cc530ca68a80747f3728fdc610` | Documentation now matches automatic main deployment and migrations. Application image rollback does not reverse committed schema migrations. No release performed. |
| [#196](https://github.com/RionishJovian/Vercel/pull/196) implementation `ac339992dd301968595729dec06d19c3a3bcf56d` | Original frontend/media-asset and acceptance improvements retained. Scan fixed in `af5e22a` (Go 1.26.11). Local browser exposed Vite Host rewriting; fixed in `f2587c4`. Hardware run sheet in `7e149a6`. Workflow cleanup and patch-level root Vite update in `ac33999`; this evidence document follows those implementation commits. |
| [#197](https://github.com/RionishJovian/Vercel/pull/197) `b855eb43b0b213b1f04a205b56ae4a6553e880be` | Only new targeted PR, for previously unimplemented issue #192. Actual host/web/API route and collapsed client IP confirmed read-only; signed rate-only browser budgets plus aggregate caps and exact proxy trust prepared. No production config applied. [Evidence and release blockers](PORTAL-RATE-LIMIT-192-20261111.md). |

All six heads combined without conflicts on **test-only** branch `codex/stabilization-compatibility-20261111`, tested commit `2864e79f313b3179cfe74fdb218cc3fdfb8a4cd6`. This is not a merge to main or a second release PR. Shared Go 1.26.11 edits are identical. Changes to server/config/workflow files combine cleanly and their security/tenant/voucher regressions remain green. Refresh main and rerun affected checks after any future drift; clean merging alone is not compatibility evidence.

## Verification results

| Check | Result / scope |
|---|---|
| Combined `go test -race -json ./internal/...` | **2,121 passing test/subtest events, 46 passing packages; 17 hardware/lab tests skipped.** Explicit private disposable PostgreSQL 17 cluster selected; `MM_TEST_DB_URL`, `MM_TEST_DSN`, `MM_MEDIA_TEST_DSN`, trust-auth fixture selection set. No production DSN/settings reused. |
| `go build ./...`; Go 1.26.11 `govulncheck ./...` v1.9.0 | PASS; **0 reachable vulnerabilities**. Scanner still lists 2 imported-package and 3 required-module vulnerabilities with no detected calls; not a claim of zero advisories in the dependency graph. |
| Production frontend build; app lint; app tests | PASS; **57 files / 378 React tests**. |
| Deployment transforms / hub reconciliation; Vercel proxy tests; critical UI | PASS: **18 / 8 / 24** respectively, including Chromium media publishing and router-local CHAP/PAP/consent/retry checks. |
| Backup/offsite/migration scripts | Syntax PASS; **22 backup, 41 offsite, 63 migration-runner checks** passed against disposable fixtures/PG17. No backup/restore drill or migration performed on production. |
| Shared-NAT regressions | PASS with race detector: 60 simultaneous browsers, 20 requests each, abusive customer isolation, signed cookie restart/expiry/forgery/duplicates, spoofed/absent/malformed forwarding headers, exact/new proxy peers, Retry-After and state capacity/reclamation. Cookie-less media behavior preserved; aggregate fallback is not verified physical identity. |
| Caddy candidate | Installed VPS web Caddy 2.10.2 accepted `adapt` via stdin. No file write/reload. Full controlled two-hop forwarding and real HotSpot source tests remain release acceptance. |
| Local browser | Actual React dev console signed into disposable Go API; Home/Vouchers opened; Create correctly blocked without a gateway. Built preview served `/media.html` and signed-in `/v2/` through corrected same-origin proxy. API listen verified `127.0.0.1:8080`; local `/ready` healthy and version included injected test revision. No voucher/router/payment created by the walkthrough. Screenshot retained locally, not product/hardware evidence. |
| Build equivalence | Repeated app build produced **104 byte-identical files**. App/dist and production-preview/v2 manifest hash both `13ec779d4ab8b270907c1638096fc77060097585b994d9178af914cc9bf7368a`. Only duplicate app build removed from CI; root production typecheck/build, lint/tests, Go/security/backup/migration/CodeQL gates remain. |
| npm audit | Root Vite 6.4.4 advisory [GHSA-8g5g-mxqh-grgj](https://github.com/advisories/GHSA-8g5g-mxqh-grgj) resolved with exact 6.4.6 patch; root audit now **0 known vulnerabilities**. App Vite 6.4.5 and the separate Vite 8 upgrade PR are unchanged. Endor `dependency-reviewer` package-risk result is **UNKNOWN**, not approval: Endor tooling/intelligence unavailable; supplied npm evidence showed no actionable risk. Exact registry URL, SHA-512 integrity and MIT license retained in lockfile. |

### Failed or skipped checks and why

- Original #196 CI failed on `GO-2026-6859` in `net/http@go1.26.10`; updated compiler, kept the mandatory scanner and all frontend changes. CI validation and CodeQL passed on `af5e22a` (run [42976672762](https://github.com/RionishJovian/Vercel/actions/runs/42976672762)).
- Regression tests intentionally failed before fixes: shared source bucket rejected request 31; unsigned sessions absent; broad proxy CIDR / X-Real-IP accepted; stalled Redis exceeded budget; configured listen host ignored; unresponsive DB probe exceeded budget. New versions passed after implementation.
- Initial sandbox-bound Chromium/server tests could not bind loopback (`EPERM`); repeated with scoped network permission and passed. No production service workaround used.
- First optional pricing driver run lacked a pricing row because the new disposable schema is data-free; supplied synthetic dated prices and reran both tests successfully. Final full run includes them. Media and connection-parsing harnesses also ran, rather than treating missing environment skips as coverage.
- The **17 remaining skips** need explicit CHR/lab credentials, isolated CHR peers, hardware certification target, or live-device binding (`MM_CHR_*`, `MM_LAB_*`, `MM_CERT_*`, `MM_LIVE_*`). None was supplied/selected for this task; no hardware write authorization is inferred.
- Independent code reviewer could run the DB/listen race tests but could not bind the Redis fixture in its own sandbox. Builder's scoped-network socket tests passed; do not relabel them independent hardware QA.

## CI and release state

At this checkpoint, #193/#194/#195/#197 validation and CodeQL are green at the tabled heads; build/deploy jobs are skipped. #191 and #196 receive final follow-up runs after their new commits. Use each PR's **current head** checks, not an older green result; final CI snapshot is linked from Notion page 08 after completion. No CI rerun uses production dispatch.

## Remaining acceptance / release blockers

1. **No main merge or production deploy without separate release review and owner approval.** Review image/schema compatibility and actual live gates before that decision. Default-off router features remain default off; no gate changed here.
2. #192/#197 needs stable explicitly controlled proxy peers and full production-shaped Caddy/header/restart tests, representative traffic budgets and an owned HotSpot client observation. Do not promote today's dynamic Docker addresses or a whole subnet into enduring trust. No live 50+ customer load test was run.
3. [Physical run sheet](ROUTERBOARD-ACCEPTANCE-RECORD-20261111.md): all RB01–RB16 **NOT RUN**. No current router model/version/backup/phone/enforcement/rollback PASS is claimed. Historical hAP/CHR reports remain dated evidence only.
4. Complete authenticated production smoke/readiness/runtime SHA/readback only after an approved release. Do not stop production DB/Redis/NATS to test faults. Committed schema migrations require explicit recovery planning; image rollback alone is not DB rollback.
5. Remaining cleanup intentionally deferred: separate Sites/historical Python material, distinct image packaging and release steps, broader major dependency upgrades and obsolete hardware-certification wording. No mass deletion, workflow-gate weakening or forced dependency resolution.
