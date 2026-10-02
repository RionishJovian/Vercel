# Subscription-only rollout — enabled on 2026-10-02

## Live rollout

The app now runs with `BILLING_SUBSCRIPTION_ONLY=true` and
`BILLING_EXISTING_ACCOUNT_POLICY=require_payment`. New businesses get one
14-day trial; existing unsubscribed businesses received one transition trial.
The additive migration was applied after a protected, verified database backup.
Existing trials and approved paid periods are preserved. No Sovereign pass is
required for Hub operations during an active trial or monthly/annual subscription.
Owner/admin, tenant, CSRF, suspension and router authentication checks remain.

Production was patched selectively; unrelated guest recovery and LAN features
were not enabled. Staged full Go and race suites passed. All three disposable
subscription database/HTTP tests passed without skips, including renewal,
concurrency, expiry recovery, role/tenant denial and cookie write protection.
Environment-only policy loading now has explicit defaults and a regression test.

Rollback source, binary and database backup:
`/home/ubuntu/deploy-backups/subscription-20261002/` on the app VPS.
The policy drop-in is `98-subscription-policy.conf`. An application rollback
must also revert that drop-in; never delete or restart issued trial periods.

## Previous implementation checklist (historical)

User-confirmed policy: newly registered businesses receive one 14-day trial;
monthly or annual payment is required afterwards. Guest internet/media sales,
tenant isolation, roles, suspension, authentication and operational approvals
are separate and must remain enforced.

Pre-rollout state: local startup/routes and Store UI wiring implemented, NOT deployed.
Shared access middleware covers protected app routes and separate media/guest
voucher owner groups. Registration and legacy-gate switches are configured together;
legacy Sovereign expiry sweep is off only in subscription-only mode. Startup checks
trial/purchase schema and tenant protection. Do not enable switches independently.
No migration or deployment has been performed for this change.

Pending before rollout:

- User confirmed existing unsubscribed businesses receive one 14-day transition
  trial. Migration inserts missing trials once at rollout and never extends an
  existing trial on rerun. Production migration is still pending.
- Done locally: shared middleware, status endpoint and coordinated handler switches.
  Middleware tests cover expiry/recovery/roles/error fail-closed. Disposable
  authenticated HTTP + database tests now prove registration/login, expiry,
  bank renewal/approval/restored routers, CSRF, roles, tenant denial and suspension
  using the production route declarations. Live rollout verification still required.
- Done locally: business-level serialized approvals preserving paid periods/trial;
  concurrent renewal and database tenant isolation tests pass on disposable VPS DB.
- Review automated AI token gates, legacy pass-expiry consumers and capacity
  displays; retain provider abuse limits and guest entitlement enforcement.
- Done locally: Store status/trial notices, monthly/annual checkout, hidden old
  capacity offers, purchase history and unavailable-status checkout guard.
  Sidebar/guide hide coin/key views in this mode. Key endpoint is read-only;
  Store coin purchases are disabled. Existing ADMIN-only manual ledger bookkeeping
  remains unchanged; balances/history remain preserved and must not be deleted.
- Test PostgreSQL transactions/RLS, trial boundaries, renewals, concurrency,
  cross-tenant denial, expiry recovery routes and all role gates.
- Apply migration explicitly with a database backup; deploy only the complete,
  tested policy; verify new registration and expired-account renewal live.

The alignment cleanup is a separate frontend change and can ship without this
unfinished backend work. Never deploy all local files indiscriminately.
