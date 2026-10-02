# Subscription-only rollout — NOT enabled

User-confirmed policy: newly registered businesses receive one 14-day trial;
monthly or annual payment is required afterwards. Guest internet/media sales,
tenant isolation, roles, suspension, authentication and operational approvals
are separate and must remain enforced.

Current state: local startup/routes and Store UI wiring implemented, NOT deployed.
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
