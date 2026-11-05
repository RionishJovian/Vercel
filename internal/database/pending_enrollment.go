package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"mega-magic/internal/secretbox"
	"time"
)

// PendingEnrollmentStore preserves a bounded installation attempt across API
// restarts. Its encrypted envelope binds the secret to tenant, device, purpose
// and expiry, so moving ciphertext between rows does not grant access.
// PendingEnrollmentLifetime is how long a generated script stays usable. A script
// pasted late must still connect, so this is long; the material is encrypted.
const PendingEnrollmentLifetime = 30 * 24 * time.Hour

type PendingEnrollmentStore struct {
	DB  *DB
	Box *secretbox.Box
}
type PendingEnrollmentStatus struct {
	RouterKey, Management string
	ManagementSaved       bool
}

// Inspect reads expiry labels only; no installation material is returned.
func (s PendingEnrollmentStore) Inspect(ctx context.Context, biz, device string) (PendingEnrollmentStatus, error) {
	status := PendingEnrollmentStatus{RouterKey: "missing", Management: "missing"}
	if !uuidRe.MatchString(biz) || !uuidRe.MatchString(device) {
		return PendingEnrollmentStatus{}, errors.New("invalid pending enrollment identity")
	}
	e := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM devices WHERE business_id=$1 AND device_id=$2
 AND management_ip IS NOT NULL AND COALESCE(ssh_username,'')<>'' AND COALESCE(ssh_secret_ciphertext,'')<>'')`, biz, device).Scan(&status.ManagementSaved)
	if e != nil {
		return PendingEnrollmentStatus{}, e
	}
	rows, e := s.DB.QueryContext(ctx, `SELECT purpose,expires_at > now() FROM hub_pending_enrollments WHERE business_id=$1 AND device_id=$2`, biz, device)
	if e != nil {
		return PendingEnrollmentStatus{}, e
	}
	defer rows.Close()
	for rows.Next() {
		var purpose string
		var ready bool
		if e = rows.Scan(&purpose, &ready); e != nil {
			return PendingEnrollmentStatus{}, e
		}
		state := "expired"
		if ready {
			state = "ready"
		}
		switch purpose {
		case "router_key":
			status.RouterKey = state
		case "management":
			status.Management = state
		default:
			return PendingEnrollmentStatus{}, errors.New("invalid pending enrollment purpose")
		}
	}
	return status, rows.Err()
}

type pendingEnvelope struct {
	Business, Device, Purpose, Value string
	Expires                          time.Time
}

func (s PendingEnrollmentStore) Check() error {
	var protected bool
	e := s.DB.QueryRow(`SELECT relrowsecurity AND relforcerowsecurity AND NOT has_table_privilege('authenticated',oid,'SELECT,INSERT,UPDATE,DELETE') FROM pg_class WHERE oid=to_regclass('public.hub_pending_enrollments')`).Scan(&protected)
	if e != nil || !protected {
		return errors.New("pending enrollment secret protections unavailable")
	}
	return nil
}
func (s PendingEnrollmentStore) GetOrCreate(ctx context.Context, biz, device, purpose string, generate func() (string, error), lifetime time.Duration) (string, error) {
	if !uuidRe.MatchString(biz) || !uuidRe.MatchString(device) || (purpose != "router_key" && purpose != "management") || s.Box == nil {
		return "", errors.New("invalid pending enrollment identity")
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return "", e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "hub-pending:"+biz+":"+device+":"+purpose); e != nil {
		return "", e
	}
	var cipher string
	var expiry time.Time
	e = tx.QueryRowContext(ctx, `SELECT ciphertext,expires_at FROM hub_pending_enrollments WHERE business_id=$1 AND device_id=$2 AND purpose=$3 FOR UPDATE`, biz, device, purpose).Scan(&cipher, &expiry)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return "", e
	}
	if e == nil && time.Now().Before(expiry) {
		raw, e := s.Box.Open(cipher)
		if e != nil {
			return "", errors.New("pending enrollment could not be decrypted")
		}
		var p pendingEnvelope
		if json.Unmarshal([]byte(raw), &p) != nil || p.Business != biz || p.Device != device || p.Purpose != purpose || !p.Expires.Equal(expiry) || p.Value == "" {
			return "", errors.New("pending enrollment identity mismatch")
		}
		return p.Value, tx.Commit()
	}
	if generate == nil {
		return "", tx.Commit()
	}
	if lifetime <= 0 || lifetime > PendingEnrollmentLifetime {
		return "", errors.New("invalid pending enrollment lifetime")
	}
	value, e := generate()
	if e != nil {
		return "", e
	}
	expiry = time.Now().UTC().Add(lifetime).Truncate(time.Microsecond)
	raw, e := json.Marshal(pendingEnvelope{biz, device, purpose, value, expiry})
	if e != nil {
		return "", e
	}
	cipher, e = s.Box.Seal(string(raw))
	if e != nil {
		return "", e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO hub_pending_enrollments(business_id,device_id,purpose,ciphertext,expires_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT(business_id,device_id,purpose) DO UPDATE SET ciphertext=excluded.ciphertext,expires_at=excluded.expires_at`, biz, device, purpose, cipher, expiry)
	if e != nil {
		return "", e
	}
	return value, tx.Commit()
}

// PendingEnrollmentRef names a router whose script may have been pasted already.
type PendingEnrollmentRef struct{ BusinessID, DeviceID string }

// Waiting lists routers with unexpired setup material, across all businesses, for the
// system task that completes a connection once the router reaches the Hub. It reads no secret.
func (s PendingEnrollmentStore) Waiting(ctx context.Context) ([]PendingEnrollmentRef, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT business_id::text, device_id::text FROM hub_pending_enrollments WHERE purpose='management' AND expires_at > now() ORDER BY expires_at`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []PendingEnrollmentRef
	for rows.Next() {
		var r PendingEnrollmentRef
		if e = rows.Scan(&r.BusinessID, &r.DeviceID); e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// DeletePurpose removes one kind of setup material for a router.
func (s PendingEnrollmentStore) DeletePurpose(ctx context.Context, biz, device, purpose string) error {
	_, e := s.DB.ExecContext(ctx, `DELETE FROM hub_pending_enrollments WHERE business_id=$1 AND device_id=$2 AND purpose=$3`, biz, device, purpose)
	return e
}

func (s PendingEnrollmentStore) Delete(ctx context.Context, biz, device string) error {
	_, e := s.DB.ExecContext(ctx, `DELETE FROM hub_pending_enrollments WHERE business_id=$1 AND device_id=$2`, biz, device)
	return e
}
