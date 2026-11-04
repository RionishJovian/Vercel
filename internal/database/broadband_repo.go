package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"mega-magic/internal/broadband"
	"time"
)

var ErrBroadbandConflict = errors.New("a different broadband operation is pending; recover it before another change")

type BroadbandAccount struct {
	ID                string                  `json:"id"`
	DeviceID          string                  `json:"device_id"`
	Username          string                  `json:"username"`
	Profile           string                  `json:"profile"`
	Observation       *broadband.State        `json:"observation"`
	RouterOperation   string                  `json:"router_operation"`
	ObservedAt        *time.Time              `json:"observed_at"`
	PendingOperation  string                  `json:"pending_operation,omitempty"`
	Session           *broadband.SessionUsage `json:"session"`
	SessionObservedAt *time.Time              `json:"session_observed_at"`
}
type BroadbandOperation struct {
	ID          string `json:"id"`
	AccountID   string `json:"account_id"`
	Kind        string `json:"kind"`
	AmountMMK   int64  `json:"amount_mmk"`
	PeriodDays  int    `json:"period_days"`
	State       string `json:"state"`
	RequestHash string `json:"-"`
	Payload     string `json:"-"`
}
type BroadbandRepository struct{}

// Retire only a create proven not installed; preserve its history and release
// the name for a corrected request without touching any router account.
func (BroadbandRepository) RejectCreateTXN(tx *sql.Tx, biz, device, op string) error {
	var account string
	if e := tx.QueryRow(`SELECT a.id::text FROM broadband_accounts a JOIN broadband_operations o ON o.business_id=a.business_id AND o.account_id=a.id
 WHERE a.business_id=$1 AND a.device_id=$2 AND o.id=$3 AND o.kind='create' AND o.state='pending' AND a.observation IS NULL FOR UPDATE OF a`, biz, device, op).Scan(&account); e != nil {
		return e
	}
	if _, e := tx.Exec(`UPDATE broadband_operations SET state='rejected' WHERE business_id=$1 AND id=$2 AND state='pending'`, biz, op); e != nil {
		return e
	}
	_, e := tx.Exec(`UPDATE broadband_accounts SET retired=true WHERE business_id=$1 AND id=$2`, biz, account)
	return e
}

func (BroadbandRepository) ListTXN(tx *sql.Tx, biz, device string) ([]BroadbandAccount, error) {
	rows, e := tx.Query(`SELECT a.id,a.device_id,a.username,a.profile,a.observation,COALESCE(a.router_operation::text,''),a.observed_at,
 COALESCE((SELECT o.id::text FROM broadband_operations o WHERE o.business_id=a.business_id AND o.account_id=a.id AND o.state='pending'),''),a.session,a.session_observed_at
 FROM broadband_accounts a WHERE a.business_id=$1 AND a.device_id=$2 AND NOT a.retired ORDER BY a.created_at,a.id`, biz, device)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []BroadbandAccount{}
	for rows.Next() {
		var a BroadbandAccount
		var data []byte
		var session []byte
		if e = rows.Scan(&a.ID, &a.DeviceID, &a.Username, &a.Profile, &data, &a.RouterOperation, &a.ObservedAt, &a.PendingOperation, &session, &a.SessionObservedAt); e != nil {
			return nil, e
		}
		if len(session) > 0 {
			if e = json.Unmarshal(session, &a.Session); e != nil {
				return nil, e
			}
		}
		if len(data) > 0 {
			if e = json.Unmarshal(data, &a.Observation); e != nil {
				return nil, e
			}
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Repeated samples are maxima, never summed. A counter regression (including
// reused session IDs after reboot) is retained as uncertainty, not hidden.
func (BroadbandRepository) ObserveSessionTXN(tx *sql.Tx, biz, device, account string, s *broadband.SessionUsage, at time.Time) error {
	var data any
	if s != nil {
		if len(s.ID) < 1 || len(s.ID) > 64 || s.DownloadBytes < 0 || s.UploadBytes < 0 || s.UptimeSeconds < 0 {
			return broadband.ErrState
		}
		raw, e := json.Marshal(s)
		if e != nil {
			return e
		}
		data = string(raw)
		if _, e = tx.Exec(`INSERT INTO broadband_session_usage(business_id,device_id,account_id,session_id,download_bytes,upload_bytes,uptime_seconds,observed_at)
   VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(business_id,account_id,session_id) DO UPDATE SET
   counter_reset=broadband_session_usage.counter_reset OR EXCLUDED.download_bytes<broadband_session_usage.download_bytes OR EXCLUDED.upload_bytes<broadband_session_usage.upload_bytes OR EXCLUDED.uptime_seconds<broadband_session_usage.uptime_seconds,
   download_bytes=GREATEST(broadband_session_usage.download_bytes,EXCLUDED.download_bytes),upload_bytes=GREATEST(broadband_session_usage.upload_bytes,EXCLUDED.upload_bytes),
   uptime_seconds=GREATEST(broadband_session_usage.uptime_seconds,EXCLUDED.uptime_seconds),observed_at=EXCLUDED.observed_at
   WHERE EXCLUDED.observed_at>=broadband_session_usage.observed_at`, biz, device, account, s.ID, s.DownloadBytes, s.UploadBytes, s.UptimeSeconds, at); e != nil {
			return e
		}
	}
	_, e := tx.Exec(`UPDATE broadband_accounts SET session=$4,session_observed_at=$5 WHERE business_id=$1 AND device_id=$2 AND id=$3 AND (session_observed_at IS NULL OR session_observed_at<=$5)`, biz, device, account, data, at)
	return e
}
func (BroadbandRepository) OperationTXN(tx *sql.Tx, biz, device, id string) (BroadbandOperation, error) {
	var o BroadbandOperation
	e := tx.QueryRow(`SELECT id,account_id,kind,amount_mmk,period_days,state,request_hash,payload_ciphertext FROM broadband_operations WHERE business_id=$1 AND device_id=$2 AND id=$3`, biz, device, id).Scan(&o.ID, &o.AccountID, &o.Kind, &o.AmountMMK, &o.PeriodDays, &o.State, &o.RequestHash, &o.Payload)
	return o, e
}
func (r BroadbandRepository) ReserveTXN(tx *sql.Tx, biz, device string, o BroadbandOperation) error {
	var lock string
	if e := tx.QueryRow(`SELECT id FROM broadband_accounts WHERE business_id=$1 AND device_id=$2 AND id=$3 FOR UPDATE`, biz, device, o.AccountID).Scan(&lock); e != nil {
		return e
	}
	var hash, account, kind string
	e := tx.QueryRow(`SELECT request_hash,account_id::text,kind FROM broadband_operations WHERE business_id=$1 AND id=$2`, biz, o.ID).Scan(&hash, &account, &kind)
	if e == nil {
		if hash != o.RequestHash || account != o.AccountID || kind != o.Kind {
			return ErrBroadbandConflict
		}
		return nil
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	var pending bool
	if e = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM broadband_operations WHERE business_id=$1 AND account_id=$2 AND state='pending')`, biz, o.AccountID).Scan(&pending); e != nil {
		return e
	}
	if pending {
		return ErrBroadbandConflict
	}
	_, e = tx.Exec(`INSERT INTO broadband_operations(id,business_id,device_id,account_id,kind,amount_mmk,period_days,request_hash,payload_ciphertext) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, o.ID, biz, device, o.AccountID, o.Kind, o.AmountMMK, o.PeriodDays, o.RequestHash, o.Payload)
	return e
}
func (BroadbandRepository) ObserveTXN(tx *sql.Tx, biz, device, account, op string, s broadband.State, at time.Time) error {
	if !s.Valid() || !broadband.UUID.MatchString(op) {
		return broadband.ErrState
	}
	// Lock the account for foreground/background reconciliation of the same observation.
	var current []byte
	var oldAt sql.NullTime
	if e := tx.QueryRow(`SELECT observation,observed_at FROM broadband_accounts WHERE business_id=$1 AND device_id=$2 AND id=$3 FOR UPDATE`, biz, device, account).Scan(&current, &oldAt); e != nil {
		return e
	}
	if oldAt.Valid && at.Before(oldAt.Time) {
		return nil
	}
	var known bool
	if e := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM broadband_operations WHERE business_id=$1 AND device_id=$2 AND account_id=$3 AND id=$4 AND state<>'rejected')`, biz, device, account, op).Scan(&known); e != nil {
		return e
	}
	if !known {
		return broadband.ErrState
	}
	var superseded bool
	if e := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM broadband_accounts a JOIN broadband_operations current_op ON current_op.business_id=a.business_id AND current_op.id=a.router_operation
 JOIN broadband_operations incoming ON incoming.business_id=a.business_id AND incoming.id=$4 WHERE a.business_id=$1 AND a.device_id=$2 AND a.id=$3 AND current_op.operation_order>incoming.operation_order)`, biz, device, account, op).Scan(&superseded); e != nil {
		return e
	}
	if superseded {
		return nil
	}
	if len(current) > 0 {
		var old broadband.State
		if e := json.Unmarshal(current, &old); e != nil {
			return e
		}
		if old.FirstUse != 0 && s.FirstUse != old.FirstUse {
			return broadband.ErrState
		}
		if s.LastClock < old.LastClock || (s.FirstUse != 0 && s.ExpiresAt < old.ExpiresAt) {
			return broadband.ErrState
		}
	}
	data, e := json.Marshal(s)
	if e != nil {
		return e
	}
	if _, e = tx.Exec(`UPDATE broadband_accounts SET observation=$4,router_operation=$5,observed_at=$6 WHERE business_id=$1 AND device_id=$2 AND id=$3`, biz, device, account, string(data), op, at); e != nil {
		return e
	}
	_, e = tx.Exec(`UPDATE broadband_operations SET state='applied',applied_at=$5 WHERE business_id=$1 AND device_id=$2 AND account_id=$3 AND id=$4 AND state='pending'`, biz, device, account, op, at)
	return e
}
