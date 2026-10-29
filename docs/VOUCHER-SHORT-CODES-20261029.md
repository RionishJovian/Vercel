# Guest voucher installation and eight-letter codes

29 October 2026. New Internet guest vouchers use eight uniformly sampled
uppercase letters, excluding ambiguous I, L, O and U. Generation uses
`crypto/rand` and rejection sampling. Business-pass generation retains its
twelve-character format. Existing vouchers and saved batches are not rewritten.
Guest code acceptance, router login validation, CHAP/PAP and printed-code
rendering continue to accept the older codes.

A code collision now uses `ON CONFLICT (business_id, code) DO NOTHING` in the
transaction-scoped insert and returns a typed retry signal. Previously a unique
violation aborted the PostgreSQL transaction, preventing the handler's intended
five-attempt retry from succeeding. Existing codes are never overwritten, and
the tenant-scoped unique constraint remains authoritative.

The pending-batch action is labelled **Install on router**. It retries the
existing saved batch and verifies the same voucher IDs, codes and allowance
snapshots on its original gateway. The former **Sync router accounting** action
is labelled **Refresh usage**; it reads existing router usage back into the app
and does not create missing router users. Printing still requires verified
installation. Guest validity begins at first use, with original time/data and
recovery evidence preserved.

The Internet Portal editor is under **Vouchers → Guest page**, with the existing
`https://megamagic.tech/app.html#portal` deep link. Jellyfin's guest page remains
separate.

Read-only live prerequisites were checked through the app container's actual
saved credentials, pinned SSH transport and outbound CIDR policy. The hAP ax2
had exactly one enabled HotSpot service and zero `mega-magic:vlogin:` users.
The saved ten-code batch remained unconfirmed. No batch retry or router user
installation was performed by this task; the user can invoke Install on router.

Validation:

- 118 critical UI checks passed, including the original-batch retry route,
  retained parameters, verified printing, duplicate guards and clear labels.
- Affected voucher, handler, database and RouterOS packages passed with race
  detection against disposable PostgreSQL: 879 passing tests/subtests, with 11
  separately gated fixture/hardware checks skipped.
- Tests cover eight letters, unbiased rejection and entropy failures, unchanged
  business-pass defaults, a real database collision followed by a successful
  insert in the same transaction, tenant-scoped uniqueness, and installation
  retries retaining twelve-character legacy codes and IDs.
- The production frontend build, Linux package checks and read-only runtime
  schema/privilege preflight passed.

Private release evidence is under `.local/voucher-short-codes-20261029` and the
corresponding protected VPS folder. A successful pinned SSH read is not guest
traffic, expiry or quota-enforcement acceptance.

Deployed API and web images: `mega-magic-api:voucher-short-codes-20261029`
and `mega-magic-web:voucher-short-codes-20261029`. All 55 frontend files
matched local release hashes and HTTP responses; the app document matched
through public HTTPS and the API reported ready. All 19 pre-existing voucher
identities/codes/allowance snapshots, management credentials, peer allocations,
data containers and configuration were preserved. No schema migration ran.

A temporary signed-in live-app tab verified the Guest page link, Install on
router action and Refresh usage label. It showed the existing ten-code batch
still at 0/10 verified; no install button was pressed. The temporary tab was
closed. Previous images, Compose configuration and binary remain available for
rollback in `/root/mm-voucher-short-codes-20261029`.
