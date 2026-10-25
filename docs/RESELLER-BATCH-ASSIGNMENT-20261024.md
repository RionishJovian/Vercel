# Reseller batch assignment — 2026-10-24

Existing guest voucher batches can be assigned, reassigned or returned to direct sales from Internet Vouchers. Batch creation optionally records a selected active shop in the same transaction as the codes. The original issuance idempotency key remains valid after later shop reassignment.

Assignment is stock metadata, not a sale, payment, delivery confirmation or guest activation. It never renames a code or changes its price, first-use anchor, limits, installation evidence or expiry. Shop names/prefixes are snapshots for labels; the prefix is not added to existing code strings. Batch selection in the printer includes only installation-verified codes. Latest 100 saved batches are shown; existing batch recovery stays separate.

A tenant-scoped mapping and append-only-for-app-role event table preserve assignment history. Composite tenant foreign keys prevent cross-workspace assignment. An expected-current-shop value rejects stale edits; an identical retry is idempotent. An assigned shop cannot be deleted until its stock is reassigned or unassigned. Inactive shops cannot receive new stock.

Release validation: real disposable PostgreSQL tests cover initial assignment, foreign batch/shop rejection, inactive shop rejection, concurrent retry, stale edits, unchanged original codes and first-use state, original issuance replay, event deduplication, stock readback, protected shop deletion and unassignment. Shared production HTTP route tests exercise authentication and role rejection. Four DOM tests cover assignment/readback, stale failure, exact-batch verified printing and selected-shop issuance. All 98 critical frontend checks and affected handler/database/server race tests pass.

Apply migration 20261024170000_guest_voucher_resellers.sql before the API. Deploy matching API + web together with validated database backup and previous images retained. Rollback restores prior API/web; additive mapping/event tables can remain to preserve history. No automatic database restore or router reset is needed.

Hardware installation, guest connectivity, expiry and reconnect certification remain pending while the physical router is unavailable.
