# Handoff: paid additional gateways and one-paste safety (2026-11-13)

Branch `claude/magichub-additional-gateways`, based on `main` at `7f63fbf`. It is separate from PR #205 and from the PPPoE work. **Not merged, not deployed.** A merge to `main` deploys and applies the migration (AGENTS.md §3).

The research reports named in the brief (`/Users/aces/Documents/Codex/2026-11-13/my/outputs/magichub-one-paste-*.md`) were not reachable from the cloud session. Every finding below was revalidated against the source.

## Owner rule implemented
One gateway is included with each client account. Additional gateways come from Store router-slot purchases (approved by the platform operator) or from explicit platform grants. They are added to the same account and the existing site. Each gateway has its own:
- device record;
- Hub allocation (tunnel address and keys);
- `mmh-` interface;
- `mm-` management account;
- status and script.

There is no shared LAN, voucher pool, failover or load balancing. The rule is recorded in AGENTS.md §4.

## Before → after (source-backed)

| Area | Before (at `7f63fbf`) | After |
|---|---|---|
| Capacity rule | `enforce_business_app_resource_cap` (`db/baseline.sql:110-113`). With enforcement on, a business could have only one MAIN_GATEWAY or DOWNSTREAM_SWITCH in total, whatever was paid. `accountpass.Evaluate` returned `RouterLimit: 1`. Production runs with enforcement on: `CheckIndividualPassSchema` refuses to start without an enforced `individual-v1` rollout, and the release script enables individual passes. (Inferred from code, not read from the server.) | `business_gateway_capacity(b)` = 1 included + legacy router grants (`device_allowances.routers − 1`) + active `gateway_capacity_grants`. The devices trigger enforces it for MAIN_GATEWAY in **every** mode, under a per-business advisory lock. Pass status, the platform accounts list and `/gateway-capacity` report this value. |
| Switches | Counted in the same one-router total. | They no longer use gateway slots. Under enforcement, a switch is still refused once the business has any gateway or switch (previous rule unchanged). In legacy mode the application-level check is unchanged. |
| Purchase → capacity | `PlatformDecide` recorded the decision only; an operator then had to raise capacity by hand. | Approving a `router_slot` purchase inserts one grant **in the same transaction** (unique per purchase, with a product/term snapshot copied by a trigger). A duplicate approval returns 409. `POST /platform/store/requests/:id/fulfill` is idempotent and fulfils purchases approved before this release. A grant can only come from an approved router-slot purchase of the same business; even direct SQL is refused. Grants are append-only (one revocation allowed). Requests report `fulfillment`: `not_applicable`, `awaiting` or `fulfilled`. |
| Creating gateway + site | Two requests (`POST /routers`, then `PUT …/site`), so a failure in between left a gateway without a site. | `POST /routers` accepts `site_id` and assigns it in the same transaction. A refused request leaves nothing behind. |
| New enrollment vs capacity | Not checked. | `RequireGatewayCapacity` guards only bootstrap, enroll and quick-enroll. The gateway must be within capacity in registration order. Finish/authenticate are not gated, so a later capacity change never strands or disconnects a gateway that is already set up. |
| Defect 1: expired setup | The server removed the Hub peer and issued a new key, but a router that had already installed the old key refused the new script ("key differs") and was stranded. | The allocation is kept and a **resume** script uses the key already on the router (no private key in it). **Start over** happens only on an explicit owner request (`start_over: true`). Its script removes only items carrying this gateway's exact marker, installs the new key on the same interface and continues. Foreign or ambiguous items still stop it. |
| Defect 2: broad cleanup | Setup deleted every other `mega-magic:mm-*` user, its SSH keys and its firewall/NAT rules. | Removed. Setup touches only this enrollment's account. Retiring an account is explicit (disconnect script). |
| Defect 3: group collision | A group with the same name had its policy overwritten. | The group is adopted only if it carries this enrollment's marker, or (setups before 2026-11-13) if its only member is this enrollment's own account. Otherwise STOP before any write. New groups are marked. |
| Order of checks | Management ownership was checked after the tunnel had been written. | The combined script runs every tunnel and management check, then all writes. The optional Wi-Fi/Ethernet WAN step is placed after the checks and before the first Hub write. |
| Defect 4: success message | "Setup complete. You can close this window" once cumulative `rx > 0`. No Safe Mode advice in the terminal or after Connected. | `TUNNEL ACTIVE` only if data arrived **during** the 30 s check, and it says management is confirmed separately. The script always prints how to commit Safe Mode (Ctrl+X). The app shows the same instruction in the script step and after Connected. |
| Header | The combined paste kept a tunnel-only header ("does not change … users") while it changed users and the SSH service. | One accurate header lists everything the paste adds or may change. |
| Secret retention | Comments said the private key is "never persisted". | Corrected: setup keys are kept encrypted until the router connects (at most 30 days), then deleted, and are never sent to the Hub. The app says so in the script step. |
| Defect 5: Wi-Fi WAN | A partial earlier attempt (disabled bridge port, list member, DHCP client) made a re-paste stop for good. The step ran before the Hub checks. | Each item it touches carries the `Magic Hub Wi-Fi WAN` marker. A re-paste accepts only factory state or this setup's own marked state and resumes each step only if not done. Anything else stops. It is labelled hAP ax² with RouterOS 7.13 or later (`/interface wifi`). It is not offered as hAP ax3 or RouterOS 7.12 (`wifiwave2`) support, which use "Keep existing network settings". |
| Credentials | Docs implied separate monitoring and change credentials. | `docs/HUB-AUTOMATIC-MANAGEMENT.md` now says there is one management account per gateway, used for both. The group policy was not widened. |
| UI | Several gateway cards, but no capacity, purchase state or site atomicity. | The Hub page shows the slots in use, the breakdown and each router-slot purchase (payment under review / approved, slot being added / slot added / not approved). With no free slot it offers the Store instead of the form. Each card shows site and management state. While one gateway's script is being made, the other gateways' actions are locked. The wizard names its target gateway. Start over needs a checkbox. English and Burmese. |

## Evidence
| Check | Result |
|---|---|
| `go vet ./internal/...`, `go test -race ./internal/...` | Pass. |
| `TestGatewayCapacitySchema`: real schema built with `mm-migrate init` + `up` on disposable PostgreSQL 16 | 9/9. Base allowance refuses B. Pending, rejected and feature purchases add nothing; forged grants are refused. Exactly-once fulfilment and retry. **8 concurrent inserts for the last slot: exactly 1 succeeds.** Legacy grants count and are not rewritten. Revocation keeps devices and blocks only new enrollments beyond capacity. RLS and tenant isolation, including a tenant self-grant (permission denied). Switch and site rules under enforcement. Operator exemption. |
| Handler tests | Expired key → resume, no removal. Start over only when asked. Start over without a remover → 409, nothing changed. |
| Hub script tests | Start over removes only marker-scoped items, with all guards before APPLY. Resume carries no private key and stops before writes. Two gateways in one tenant share no names, markers or accounts. The default paste is still non-destructive. |
| Management script tests | No `mm-*` cleanup. Group and user checks come before the tunnel's first write. Fresh-traffic check. Safe Mode text. Accurate header. |
| `node scripts/test-hub-enrollment.mjs` | Pass, including two routers in one tenant: distinct addresses, B retry/conflict leaves A byte-identical. |
| App: lint, `tsc -b`, vitest 413/413, build | Pass. New tests: `gateways.test.tsx` (slots, add B in one request, capacity refusal, context lock, Safe Mode, resume, confirmed Start over, Wi-Fi resume order) and i18n coverage. |
| Root: `test:deployment` 18/18, `test:vercel` 8/8, `npm run build`, `test:critical-ui` 25/25 | Pass. |
| Rollback SQL `db/rollback/20261113120000_gateway_capacity_down.sql` | Applied after `up` on a throwaway DB: the original function is restored. |
| **Local purchase-to-second-gateway journey** (Caddy with production Caddyfile, API from this branch, disposable DB, enforcement on as in production, Chromium 390 px) | **29/29**, covering the steps below. |

The journey's 29 checks covered:
- the site and gateway A are created in one request;
- the capacity reads 1 of 1, B is refused (409) and leaves no record;
- the Hub page shows the Store link;
- the platform operator creates the router-slot add-on;
- the owner's purchase shows "Payment under review" and adds no capacity;
- the owner cannot approve their own purchase (403);
- approval fulfils the slot, a duplicate approval returns 409, and both fulfil retries return the same grant (exactly one exists);
- the capacity becomes 1 of 2 with the monthly snapshot and `not_enforced` term;
- the Hub page shows "Slot added";
- B is added in the UI to the same site, and A's row is byte-identical;
- B's enrollment says honestly that the Hub is not available (no Hub in the lab), and C is refused;
- deleting B leaves A unchanged and frees the slot;
- another tenant sees nothing;
- the Burmese card has no overflow.

Screenshots of the journey were taken locally at 390 px: slots full, payment under review, two gateways, Burmese.

## Skipped (honest)
- **RouterOS CHR lab** (`TestCHRLabOnePaste`) was skipped: `MM_CHR_SSH`/`MM_CHR_KEY` are not configured in this environment. The new resume, start-over, group-adoption and Wi-Fi resume scripts have **not** run on RouterOS. In particular, RouterOS behaviour for `/user/group … comment=` and `get … comment`, and for `[:tonum rx]` deltas, is unproven here.
- **No Magic Hub** in the lab, so no tunnel, handshake, management login or real script output. No physical router, WinBox paste, reboot persistence, shared-NAT two-router test or guest-service acceptance.
- **Native Burmese review** of the new strings (`app/src/i18n/gateways.ts`) is pending.

## Commercial decisions still open (behind explicit boundaries)
1. **Renewal and expiry** of monthly/annual router slots. Grants record the period and price but have `term_policy='not_enforced'` and never lapse. Nothing disconnects.
2. **Refunds and revocation.** The ledger supports one audited revocation per grant, but no API or UI exposes it. Revocation never removes devices; it only blocks new enrollments beyond capacity.
3. **Transfer** of slots between accounts: not supported.
4. **Switch allowance:** switches keep the previous rule. Whether they get their own allowance is undecided.
5. **Router-slot purchases approved before this release:** the operator should check that capacity was not already raised by hand, then use `POST /api/v1/platform/store/requests/:id/fulfill`. Fulfilment is not automatic for old approvals, to avoid double grants.

## Rollout
1. Review and merge the PR (deploys). `mm-migrate up` applies `20261113120000_gateway_capacity.sql`, which adds a table and function and replaces the trigger function, then re-runs the runtime-role grants.
2. Verify `/ready`, then sign in as a client and check that `GET /api/v1/gateway-capacity` reports 1 of 1 for a single-gateway account.
3. Fulfil any already-approved router-slot purchases explicitly (see item 5 above).

## Rollback
Revert the PR on `main` (deploys the old code). The new table and function are unused by the old code and can stay. To restore the old one-router database rule too, run `psql -v ON_ERROR_STOP=1 -f db/rollback/20261113120000_gateway_capacity_down.sql`. It keeps the grant ledger as purchase evidence and never removes devices.

## Remaining hardware acceptance (needs owner approval of a test window)
1. Back up the test router. Record RouterOS version and model (e.g. hAP ax3 on 7.12.2, internet over Wi-Fi). Use **Keep existing network settings**.
2. **Gateway A:** one paste in WinBox with Safe Mode on. Check that the terminal prints `TUNNEL ACTIVE` or `NOT CONNECTED YET` and the Safe Mode line, press Ctrl+X, and confirm the app shows Connected (saved management).
3. **Gateway B** (second router, same site, after a fulfilled slot): one paste and connected. Check that A stays Connected throughout and that A's `mmh-`/`mm-` items are untouched on both routers.
4. **Expired setup:** let B's setup lapse (or remove the stored key in a lab), then get the resume script. On a router with the earlier key it continues; on one without, it STOPs. Then Start over, and check that only B's marked items are replaced.
5. **Collision:** pre-create a group or user named `mm-…` that is not ours. The paste must STOP with nothing changed.
6. **Wi-Fi WAN on a factory hAP ax² (7.13+):** interrupt after the bridge-port step, re-paste, and check that it resumes.
7. Reboot both routers and check persistence. Run a two-router shared-NAT check. Check guest Hotspot on each gateway separately.
