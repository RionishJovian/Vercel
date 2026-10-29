# Monthly and annual account-pass checkout

The individual-pass rollout retired legacy Store checkout but left clients with
only a plan-only activation request. The current My pass screen now displays the
published monthly and annual prices and configured bank-payment destinations.
Clients submit their bank transaction reference for platform review. The app
does not charge a bank account or infer settlement from a reference.

`GET /me/app-pass/checkout-options` remains available after pass expiry. The
existing renewal endpoint accepts complete payment evidence with a stable UUID.
The server derives the price, checks the quoted amount and enabled destination,
and saves its destination snapshot. The request ID and exact payment details are
retained in browser session storage before submission, scoped to business and
login; uncertain responses retry the same record. Duplicate keys cannot change
the payment details, and one pending request prevents another transfer request.

The platform review queue shows the amount, destination and reference. Paid
approval requires an explicit payment-verification acknowledgement, with pass,
revision audit and decision committed together. A separate manual activation
cannot bypass a pending paid request. Paid renewals retain unexpired access and
extend from its expiry by one calendar month or year in Asia/Yangon. Expired or
revoked access starts again on approval. Existing plan-only requests retain their
explicit manual review and are labelled as having no recorded payment evidence.

Platform administrators remain exempt and cannot purchase a client pass for
themselves. Historical Store records, including old pending orders, are preserved
and never automatically converted to current pass access. Guest vouchers and
already-running router sessions are unaffected.

Deploy the additive migration
`20261029090000_account_pass_checkout.sql` before the new API. It preserves
existing rows, adds the price/payment snapshots and retry key, and validates
complete evidence plus unique retry identity. It does not approve payments or
grant access. Startup checks require these protections and retain forced tenant
RLS and the existing pass-authority privilege checks.

Verification covers real HTTP/database flows for both plans with new, active and
expired clients; concurrent/replayed requests; forged prices; disabled bank
accounts; foreign and secondary accounts; platform exemption; explicit payment
verification; duplicate/manual approval prevention; calendar expiry and audit
integrity. UI checks cover both plan selections, saved retry recovery, escaped
payment evidence, missing options/history, approval payloads and refreshed access.
Only disposable fixtures are used for payment and approval writes.
