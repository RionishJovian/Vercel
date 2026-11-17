package database

import (
	"context"
	"errors"
	"fmt"

	"mega-magic/internal/vouchercode"
)

// VoucherCodeBackfillReport counts rows by storage form. It never contains a code.
type VoucherCodeBackfillReport struct {
	Visible       int  // vouchers this connection can see (0 on a large database means the connection is row-restricted)
	PlaintextOnly int  // legacy: plaintext code, no sealed copy
	Both          int  // sealed copy exists and plaintext is still present
	ProtectedOnly int  // plaintext cleared
	Sealed        int  // rows given a sealed copy by this run
	Cleared       int  // rows whose plaintext this run removed
	Mismatched    int  // sealed copy did not match its plaintext; left untouched
	Applied       bool // false for a dry run
}

// VoucherCodeStats reports the current storage forms without changing anything.
func (db *DB) VoucherCodeStats(ctx context.Context) (VoucherCodeBackfillReport, error) {
	var r VoucherCodeBackfillReport
	err := db.QueryRowContext(ctx, `SELECT count(*),
		count(*) FILTER (WHERE code IS NOT NULL AND code_enc IS NULL),
		count(*) FILTER (WHERE code IS NOT NULL AND code_enc IS NOT NULL),
		count(*) FILTER (WHERE code IS NULL)
		FROM vouchers`).Scan(&r.Visible, &r.PlaintextOnly, &r.Both, &r.ProtectedOnly)
	return r, err
}

// ProtectVoucherCodes converts existing rows to the protected form.
//
// Step 1 (apply): every plaintext row gets its sealed copy and blind index, verified by reading it back. Plaintext is
// kept, so the previous app version keeps working and this is fully reversible.
// Step 2 (apply and clearPlaintext): the plaintext of each row is removed, but only after the sealed copy is opened
// again and found identical. Do this only once every running app instance has this version and the key; an older
// instance cannot read a row whose plaintext is gone.
//
// Without apply nothing is written. It runs on the service connection (like RotateCredentialKey), in small
// transactions, so it can run while the app is serving.
func (db *DB) ProtectVoucherCodes(ctx context.Context, p *vouchercode.Protector, apply, clearPlaintext bool, batch int) (VoucherCodeBackfillReport, error) {
	if p == nil {
		return VoucherCodeBackfillReport{}, vouchercode.ErrNoKey
	}
	if clearPlaintext && !apply {
		return VoucherCodeBackfillReport{}, errors.New("clearing plaintext needs apply")
	}
	if batch <= 0 || batch > 5000 {
		batch = 500
	}
	// A key that cannot round-trip must stop the run before it touches a row.
	probe, err := p.Seal("PROBE")
	if err != nil {
		return VoucherCodeBackfillReport{}, err
	}
	if back, err := p.Open(probe); err != nil || back != "PROBE" {
		return VoucherCodeBackfillReport{}, errors.New("voucher code key failed its round-trip check")
	}

	var report VoucherCodeBackfillReport
	if apply {
		sealed, err := db.sealPlaintextVouchers(ctx, p, batch)
		report.Sealed = sealed
		if err != nil {
			return report, err
		}
		if clearPlaintext {
			cleared, mismatched, err := db.clearVerifiedPlaintext(ctx, p, batch)
			report.Cleared, report.Mismatched = cleared, mismatched
			if err != nil {
				return report, err
			}
		}
	}
	stats, err := db.VoucherCodeStats(ctx)
	if err != nil {
		return report, err
	}
	stats.Sealed, stats.Cleared, stats.Mismatched, stats.Applied = report.Sealed, report.Cleared, report.Mismatched, apply
	return stats, nil
}

const zeroUUID = "00000000-0000-0000-0000-000000000000"

func (db *DB) sealPlaintextVouchers(ctx context.Context, p *vouchercode.Protector, batch int) (int, error) {
	total, last := 0, zeroUUID
	for {
		n, next, err := db.sealBatch(ctx, p, batch, last)
		total += n
		if err != nil {
			return total, err
		}
		if next == "" {
			return total, nil
		}
		last = next
	}
}

func (db *DB) sealBatch(ctx context.Context, p *vouchercode.Protector, batch int, after string) (sealed int, last string, err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, "", err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id::text, code FROM vouchers
		WHERE code IS NOT NULL AND code_enc IS NULL AND id > $1::uuid
		ORDER BY id LIMIT $2 FOR UPDATE SKIP LOCKED`, after, batch)
	if err != nil {
		return 0, "", err
	}
	type row struct{ id, code string }
	var found []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.code); err != nil {
			rows.Close()
			return 0, "", err
		}
		found = append(found, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, "", err
	}
	rows.Close()
	if len(found) == 0 {
		return 0, "", nil
	}
	for _, r := range found {
		enc, err := p.Seal(r.code)
		if err != nil {
			return 0, "", fmt.Errorf("seal voucher %s: %w", r.id, err)
		}
		if back, err := p.Open(enc); err != nil || back != r.code {
			return 0, "", fmt.Errorf("voucher %s did not verify after sealing", r.id)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE vouchers SET code_enc=$2, code_hmac=$3 WHERE id=$1::uuid AND code_enc IS NULL`, r.id, enc, p.Blind(r.code)); err != nil {
			return 0, "", fmt.Errorf("store sealed voucher %s: %w", r.id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, "", err
	}
	return len(found), found[len(found)-1].id, nil
}

func (db *DB) clearVerifiedPlaintext(ctx context.Context, p *vouchercode.Protector, batch int) (cleared, mismatched int, err error) {
	last := zeroUUID
	for {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return cleared, mismatched, err
		}
		rows, err := tx.QueryContext(ctx, `SELECT id::text, code, code_enc, code_hmac FROM vouchers
			WHERE code IS NOT NULL AND code_enc IS NOT NULL AND code_hmac IS NOT NULL AND id > $1::uuid
			ORDER BY id LIMIT $2 FOR UPDATE SKIP LOCKED`, last, batch)
		if err != nil {
			_ = tx.Rollback()
			return cleared, mismatched, err
		}
		var ids []string
		var seen int
		var lastSeen string
		for rows.Next() {
			var id, code, enc, blind string
			if err := rows.Scan(&id, &code, &enc, &blind); err != nil {
				rows.Close()
				_ = tx.Rollback()
				return cleared, mismatched, err
			}
			seen++
			lastSeen = id
			if back, e := p.Open(enc); e == nil && back == code && p.Blind(code) == blind {
				ids = append(ids, id)
			} else {
				mismatched++
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			_ = tx.Rollback()
			return cleared, mismatched, err
		}
		rows.Close()
		for _, id := range ids {
			if _, err := tx.ExecContext(ctx, `UPDATE vouchers SET code=NULL WHERE id=$1::uuid AND code IS NOT NULL AND code_enc IS NOT NULL AND code_hmac IS NOT NULL`, id); err != nil {
				_ = tx.Rollback()
				return cleared, mismatched, fmt.Errorf("clear voucher %s: %w", id, err)
			}
		}
		if err := tx.Commit(); err != nil {
			return cleared, mismatched, err
		}
		cleared += len(ids)
		if seen == 0 {
			return cleared, mismatched, nil
		}
		last = lastSeen
	}
}
