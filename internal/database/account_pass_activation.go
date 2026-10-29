package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"mega-magic/internal/accountpass"
	"time"
)

var ErrAccountPassRevision = errors.New("account pass changed; refresh before retrying")

// Caller holds the target user's row lock. Pass and audit change atomically.
func ActivateAccountPassTXN(tx *sql.Tx, userID, businessID, actorID, plan string, revision int64, now time.Time) (int64, error) {
	expiry, err := accountpass.CalendarExpiry(now, plan)
	if err != nil {
		return 0, err
	}
	return recordAccountPassTXN(tx, userID, businessID, actorID, plan, revision, now, expiry, now, "activate")
}

// A verified paid renewal retains any unexpired, non-revoked access. The caller
// holds the user's row lock, and the request decision shares this transaction.
func RenewAccountPassTXN(tx *sql.Tx, userID, businessID, actorID, plan string, revision int64, now time.Time) (int64, error) {
	start, base := now, now
	var oldStart, oldEnd time.Time
	var revoked sql.NullTime
	var current int64
	err := tx.QueryRow(`SELECT activated_at,expires_at,revoked_at,revision FROM app_account_passes WHERE user_id=$1 AND business_id=$2 FOR UPDATE`, userID, businessID).Scan(&oldStart, &oldEnd, &revoked, &current)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if current != revision {
		return 0, ErrAccountPassRevision
	}
	if err == nil && !revoked.Valid && oldEnd.After(now) {
		start, base = oldStart, oldEnd
	}
	expiry, err := accountpass.CalendarExpiry(base, plan)
	if err != nil {
		return 0, err
	}
	return recordAccountPassTXN(tx, userID, businessID, actorID, plan, revision, start, expiry, now, "renew")
}

func recordAccountPassTXN(tx *sql.Tx, userID, businessID, actorID, plan string, revision int64, start, expiry, approved time.Time, action string) (int64, error) {
	var next int64
	err := tx.QueryRow(`INSERT INTO app_account_passes(user_id,business_id,plan,activated_at,expires_at,recorded_by)
 SELECT $1,$2,$3,$4,$5,$6 WHERE $7=0 OR EXISTS(SELECT 1 FROM app_account_passes WHERE user_id=$1 AND revision=$7)
 ON CONFLICT(user_id) DO UPDATE SET plan=excluded.plan,activated_at=excluded.activated_at,expires_at=excluded.expires_at,revoked_at=NULL,recorded_by=excluded.recorded_by,updated_at=now(),revision=app_account_passes.revision+1
 WHERE app_account_passes.revision=$7 AND app_account_passes.business_id=excluded.business_id RETURNING revision`, userID, businessID, plan, start, expiry, actorID, revision).Scan(&next)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrAccountPassRevision
	}
	if err != nil {
		return 0, err
	}
	detail, _ := json.Marshal(map[string]any{"action": action, "plan": plan, "approved_at": approved, "activated_at": start, "expires_at": expiry, "calendar_timezone": "Asia/Yangon"})
	_, err = tx.Exec(`INSERT INTO app_account_pass_events(user_id,business_id,actor_id,revision,details)VALUES($1,$2,$3,$4,$5)`, userID, businessID, actorID, next, detail)
	return next, err
}
func RevokeAccountPassTXN(tx *sql.Tx, userID, businessID, actorID string, revision int64, now time.Time) (int64, error) {
	var next int64
	err := tx.QueryRow(`UPDATE app_account_passes SET revoked_at=$4,recorded_by=$3,revision=revision+1,updated_at=now() WHERE user_id=$1 AND business_id=$2 AND revision=$5 AND revoked_at IS NULL RETURNING revision`, userID, businessID, actorID, now, revision).Scan(&next)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrAccountPassRevision
	}
	if err != nil {
		return 0, err
	}
	_, err = tx.Exec(`INSERT INTO app_account_pass_events(user_id,business_id,actor_id,revision,details)VALUES($1,$2,$3,$4,'{"action":"revoke"}'::jsonb)`, userID, businessID, actorID, next)
	return next, err
}
