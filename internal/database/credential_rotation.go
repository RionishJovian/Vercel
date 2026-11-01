package database

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"mega-magic/internal/secretbox"
)

// CredentialRotationReport counts what a rotation touched. It never contains
// secret material, so it is safe to print.
type CredentialRotationReport struct {
	Applied             bool
	DeviceSecrets       int // device SSH logins re-sealed
	PendingEnrollments  int // unexpired pending enrollments re-sealed
	ExpiredEnrollments  int // expired pending enrollments deleted (already unusable)
	VerifiedUnderNewKey int // values opened again with the new key before commit
}

// RotateCredentialKey re-seals every value protected by the applier credential
// key from oldBox to newBox in ONE transaction. With apply=false the same work
// is done and verified, then rolled back, so it is a faithful dry run.
//
// The service must not be running against these tables with the old key while
// this commits: after commit only the new key can open the data. The runbook
// stops the API for the window.
//
// Fail-closed rules: any value the old key cannot open aborts the whole
// rotation (nothing is changed), a new key equal to the old key is refused, and
// every re-sealed value is opened again with the new key (and shown to be
// unreadable with the old one) before commit.
func (d *DB) RotateCredentialKey(ctx context.Context, oldBox, newBox *secretbox.Box, apply bool, now time.Time) (CredentialRotationReport, error) {
	var report CredentialRotationReport
	if oldBox == nil || newBox == nil {
		return report, errors.New("both the current and the new credential key are required")
	}
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return report, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `LOCK TABLE devices, hub_pending_enrollments IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return report, fmt.Errorf("lock tables: %w", err)
	}

	type deviceSecret struct{ id, sealed string }
	var devices []deviceSecret
	rows, err := tx.QueryContext(ctx, `SELECT device_id::text, ssh_secret_ciphertext FROM devices WHERE ssh_secret_ciphertext IS NOT NULL ORDER BY device_id`)
	if err != nil {
		return report, err
	}
	for rows.Next() {
		var s deviceSecret
		if err := rows.Scan(&s.id, &s.sealed); err != nil {
			rows.Close()
			return report, err
		}
		devices = append(devices, s)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return report, err
	}
	rows.Close()

	reseal := func(label, sealed string) (string, error) {
		plain, err := oldBox.Open(sealed)
		if err != nil {
			return "", fmt.Errorf("%s cannot be opened with the current key; nothing was changed", label)
		}
		next, err := newBox.Seal(plain)
		if err != nil {
			return "", fmt.Errorf("%s: seal with the new key: %w", label, err)
		}
		back, err := newBox.Open(next)
		if err != nil || subtle.ConstantTimeCompare([]byte(back), []byte(plain)) != 1 {
			return "", fmt.Errorf("%s: value does not round-trip under the new key", label)
		}
		if _, err := oldBox.Open(next); err == nil {
			return "", errors.New("the new key opens values sealed for the current key: the keys must differ")
		}
		report.VerifiedUnderNewKey++
		return next, nil
	}

	for _, dev := range devices {
		next, err := reseal("device "+dev.id, dev.sealed)
		if err != nil {
			return report, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE devices SET ssh_secret_ciphertext=$1 WHERE device_id=$2`, next, dev.id); err != nil {
			return report, fmt.Errorf("device %s: %w", dev.id, err)
		}
		report.DeviceSecrets++
	}

	type pending struct {
		business, device, purpose, sealed string
		expires                           time.Time
	}
	var enrollments []pending
	rows, err = tx.QueryContext(ctx, `SELECT business_id::text, device_id::text, purpose, ciphertext, expires_at FROM hub_pending_enrollments ORDER BY business_id, device_id, purpose`)
	if err != nil {
		return report, err
	}
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.business, &p.device, &p.purpose, &p.sealed, &p.expires); err != nil {
			rows.Close()
			return report, err
		}
		enrollments = append(enrollments, p)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return report, err
	}
	rows.Close()
	for _, p := range enrollments {
		key := "pending enrollment " + p.device + "/" + p.purpose
		if !p.expires.After(now) {
			// Already unusable (the store refuses expired rows); deleting is
			// safer than keeping data sealed under a retired key.
			if _, err := tx.ExecContext(ctx, `DELETE FROM hub_pending_enrollments WHERE business_id=$1 AND device_id=$2 AND purpose=$3`, p.business, p.device, p.purpose); err != nil {
				return report, fmt.Errorf("%s: %w", key, err)
			}
			report.ExpiredEnrollments++
			continue
		}
		next, err := reseal(key, p.sealed)
		if err != nil {
			return report, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE hub_pending_enrollments SET ciphertext=$1 WHERE business_id=$2 AND device_id=$3 AND purpose=$4`, next, p.business, p.device, p.purpose); err != nil {
			return report, fmt.Errorf("%s: %w", key, err)
		}
		report.PendingEnrollments++
	}

	// Final pass over what is now in the database, not what we meant to write.
	if err := verifyAllUnderKey(ctx, tx, newBox, report); err != nil {
		return report, err
	}
	if !apply {
		return report, nil // deferred Rollback: a dry run changes nothing
	}
	if err := tx.Commit(); err != nil {
		return report, err
	}
	report.Applied = true
	return report, nil
}

func verifyAllUnderKey(ctx context.Context, tx *sql.Tx, box *secretbox.Box, want CredentialRotationReport) error {
	count := 0
	for _, q := range []string{
		`SELECT ssh_secret_ciphertext FROM devices WHERE ssh_secret_ciphertext IS NOT NULL`,
		`SELECT ciphertext FROM hub_pending_enrollments`,
	} {
		rows, err := tx.QueryContext(ctx, q)
		if err != nil {
			return err
		}
		for rows.Next() {
			var sealed string
			if err := rows.Scan(&sealed); err != nil {
				rows.Close()
				return err
			}
			if _, err := box.Open(sealed); err != nil {
				rows.Close()
				return errors.New("a stored value does not open under the new key; nothing was changed")
			}
			count++
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
	}
	if count != want.DeviceSecrets+want.PendingEnrollments {
		return fmt.Errorf("verification saw %d sealed values, expected %d; nothing was changed", count, want.DeviceSecrets+want.PendingEnrollments)
	}
	return nil
}
