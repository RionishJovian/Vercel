# Takeover release — 2026-10-24

Base: b7635b0c88be13d923867f81e5fc8422fb213174. The live binary was independently read back as cd16a4fea4e456a27cbfd23c539d0d720b0cf91b29d75750889deec09740c915 before rollout.

## Changes

- Discovery labels use the frontend's `mac` contract and one tenant transaction for device ownership, write and readback. Additive migration forces RLS, grants the authenticated role and enforces a composite device/business foreign key.
- Read-only observations have a fixed-menu RouterOS reader behind `provisioner.inventory_enabled` (default false), independently of router writes. Responses never advertise write access. Guest disconnect returns 503 until an accepted implementation exists.
- VIEWER cannot create/update/delete alert policies. Tenant admins/owners cannot self-approve legacy Store requests; approval/rejection is restricted to the platform DEVELOPER role. This does not establish Store as the new subscription authority.
- Public guest purchase uses the same published site policy as the guest page. New requests persist purchased minutes, amount and data. Decisions use stored terms. Historical requests without an immutable router allowance require manual review and cannot extend business Hub/WebFig access.
- Recovered frontend files are tracked at their exact deployed contents; this release requires no web asset change.

## Verification

Disposable PostgreSQL at loopback port 55440, with separate automatically created/dropped fixture databases. Affected auth, handler, server, database, routeros and config packages pass `go test -race`; authenticated role/tenant tests include denied-write database readback. Handler regressions prove published-site purchase admission, saved terms after plan removal, duplicate decision rejection and historical-request refusal. Router command tests use fakes, not physical RouterOS.

Critical UI: 74 pass, zero failures/skips. Linux AMD64 built with Go 1.26.5, CGO_ENABLED=0, -trimpath -buildvcs=false. Candidate binary SHA256: e04a297bb39b4eada0b093fc60d4b6a8a16d7af77b7c69926c88f5c72e7f2e20.

Evidence: /Users/aces/Documents/Codex/2026-10-21/asd/work/takeover-20261024/.

Recovered live asset hashes:
- workflow-revamp.js: af13b131a57891c17540da13092049b4999db526f017b4a79f420a0aa9b66f77
- media-dashboard.js: 8b89a62eaeb8013f9bdbae2597ab8c66e6d12798337c1cc0d933ad296d270eae
- network-control.js: 648bc2e5b2a0b8fdbeb94ca694a2becf25cd2513e3b37e4131e18a4e497ffb1e

## Rollout

Before API replacement: capture current image IDs, tag rollback image, copy Compose file/config and create a validated database dump. Apply only 20261024010000_discovery_label_isolation.sql and 20261024020000_guest_purchase_terms.sql with ON_ERROR_STOP. Preserve every historical row. Build an immutable API image from the current runtime base and the verified binary; replace only the API service. Restore previous Compose/image on failed readiness. Additive schema changes remain compatible with the previous binary; do not restore the entire database for an application rollback.

## Still pending

Physical RouterBOARD/phone internet, first use, elapsed expiry, data cap and reconnect acceptance; inventory activation; actual guest disconnect; paid scheduler verification and router/database reconciliation after uncertain commits; remaining legacy billing/entitlement and site-editor restructuring. Commercial policy (account vs business, pricing, seats/limits) remains an owner decision. No live payment execution, customer role writes, voucher consumption, router reset or hardware certification is included.
