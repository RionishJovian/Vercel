# Voucher codes at rest

Added 2026-11-14 (audit blocker B1). Code: `internal/vouchercode`, `internal/database/voucher_code_protection.go`,
`internal/database/voucher_code_backfill.go`, `cmd/voucher-code-protect`. Migration:
`db/migrations/20261114090000_voucher_code_protection.sql`.

## What it protects, and what it does not

Before: `vouchers.code` held every code in clear. Anyone who could read the database, a backup or a read-only SQL grant
had working internet codes.

Why not a one-way hash: the router's HotSpot user name **is** the code. Usage sync, Kick, Ban, batch retry and
printing all need the code back, so the app must be able to recover it. A plain hash would also be useless for an 8
letter code (22^8, about 35 bits, is cracked offline in minutes).

Now, per voucher:

| Column | Holds |
|---|---|
| `code_enc` | the code sealed with AES-256-GCM |
| `code_hmac` | HMAC-SHA256 blind index of the code, so a typed code is found without storing it |
| `code` | `NULL` once protected; the plaintext only for legacy rows |

The keys are derived (HKDF-SHA256, separate contexts for sealing and indexing) from `security.voucher_code_key`, which
is never in the database.

- Protects against: a leaked database, backup, or read-only SQL access. They get neither codes nor a way to test guesses.
- Does not protect against: an attacker who holds the database **and** this key (the running app has both).
- Trade-off: owner search matches a complete code only (any case). Fragments still match plan labels and legacy rows.

## Settings (both default off)

```yaml
security:
  voucher_code_key: <32 random bytes, base64 or hex>   # lets the app READ protected codes
  protect_voucher_codes: true                          # makes NEW codes protected
```

`protect_voucher_codes` without a key, or a malformed key, stops the API from starting. A protected row read without
the key is an error, never an empty code. Generate a key with `openssl rand -base64 32`. **Back it up apart from the
database**: without it protected codes cannot be recovered and cannot be re-installed on a router.

Do not reuse the credential key. Rotating the credential key (`docs/SECRET-ROTATION.md`) does not touch this one.
There is no rotation tool for this key yet; do not change it while protected rows exist.

## Rollout

Each step is safe to stop after. Nothing here is automatic, and the migration does not change behaviour by itself.

1. **Migrate.** `mm-migrate up`. Adds the columns and index. The app behaves as before.
2. **Deploy this version with the key set and `protect_voucher_codes` off.** The app now reads either form.
3. **Turn `protect_voucher_codes` on.** New vouchers are stored protected. Existing ones are untouched. Issue a batch,
   redeem one code, print one; confirm each works.
4. **Seal existing rows.** `voucher-code-protect` (dry run) then `voucher-code-protect -apply`. Every legacy row gets a
   verified sealed copy; plaintext stays. Re-run safely; it only touches rows still missing one.
5. **Clear plaintext.** Only when every API instance runs this version with the key:
   `voucher-code-protect -apply -clear-plaintext`. Each row is opened again and compared before its plaintext is
   removed; a mismatch is reported and left alone.
6. **Take a fresh backup.** Older backups still contain plaintext codes. Expire or secure them accordingly.

## Rollback

- Before step 5: set `protect_voucher_codes` off. Protected rows stay readable while the key is loaded.
- After step 5: the plaintext is gone. Rolling back the app to a version without this feature makes those vouchers
  unreadable. Roll forward, or restore the pre-step-5 backup.
- The migration is additive; do not edit or revert it. A later migration may add `NOT NULL`-style tightening once all
  rows are protected.

## Not covered

VIP Wi-Fi codes (`vip_wifi_codes`) are a different feature and still store their code in clear. The Python prototype
(`backend/`) already stores only hashes of its 128-bit codes and is unaffected.
