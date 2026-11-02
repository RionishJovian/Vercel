# Guest plan speed tiers

A guest plan (Vouchers → Guest page → Plans) can now carry a **download** and an **upload** speed in Mbps. Both are set or neither is; 0/0 means no limit. The portal shows the download speed on the plan.

## What happens
1. A guest buys a plan on the portal. The plan's speed is **saved with the purchase request**, like the time and data allowance, so editing the plan later never changes what that guest bought.
2. When the owner approves the purchase, the router is told the speed. The app prepares a Hotspot **user profile** `mm-speed-u<up>-d<down>` (rate limit `<up>M/<down>M`, `shared-users=1`, comment `mega-magic:speed`) and switches the guest's user to it together with the paid time and data limits, then reads the user back and refuses to report success unless the guest is on that profile. The guest's active session is removed so the next login picks the speed up.
3. RouterOS rate limits are `rx/tx` from the **router's** side, so the client's **upload comes first**: 20 Mbps down and 5 Mbps up is `5M/20M`. A test fails if the order is swapped.
4. The profile is created only when missing. An existing profile with the same name is adopted only if Mega Magic made it with exactly these limits (same comment, same rate in any unit, `shared-users=1`, no login/logout script). Anything else is refused and left untouched, and the guest's temporary account stays as it was so the approval can be retried.

## Not included
- **Voucher codes** (batches): they share one guarded profile that carries the first-use script, so a speed there needs a guarded copy per speed. That waits until vouchers have been tested on a real router.
- The free grace session while a payment is pending has no speed limit.
- Existing purchases and plans keep working: speed defaults to 0/0.

## Production rollout (the deploy does not migrate)
`deploy/deploy-vps.sh` does no schema migration, so apply `migrations/20261102120000_guest_plan_speed.sql` to the production database **before** deploying this code. It is additive and safe to run twice (`ADD COLUMN IF NOT EXISTS` with default 0, and a check that both speeds are set or neither). Old code ignores the new columns, so applying it first is safe; new code without it would fail every guest purchase read.

## Verified / not verified
Verified: unit tests against a simulated router (order, adoption rules, read-back, invalid speeds), and database tests against a real PostgreSQL (migration twice, default values, the constraint, the purchase saving the speed, approval passing it on). Not yet verified on a real RouterBOARD: how RouterOS prints the profile's `rate-limit` and `shared-users` for the adoption check, and that the Hotspot enforces the profile's limit for a MAC-logged-in guest. Try it on a router whose owner agreed, with one test plan.
