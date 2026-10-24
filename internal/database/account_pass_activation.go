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
	var next int64
	err = tx.QueryRow(`INSERT INTO app_account_passes(user_id,business_id,plan,activated_at,expires_at,recorded_by)
 SELECT $1,$2,$3,$4,$5,$6 WHERE $7=0 OR EXISTS(SELECT 1 FROM app_account_passes WHERE user_id=$1 AND revision=$7)
 ON CONFLICT(user_id) DO UPDATE SET plan=excluded.plan,activated_at=excluded.activated_at,expires_at=excluded.expires_at,revoked_at=NULL,recorded_by=excluded.recorded_by,updated_at=now(),revision=app_account_passes.revision+1
 WHERE app_account_passes.revision=$7 AND app_account_passes.business_id=excluded.business_id RETURNING revision`, userID, businessID, plan, now, expiry, actorID, revision).Scan(&next)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrAccountPassRevision
	}
	if err != nil {
		return 0, err
	}
	detail, _ := json.Marshal(map[string]any{"action": "activate", "plan": plan, "activated_at": now, "expires_at": expiry, "calendar_timezone": "Asia/Yangon"})
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
