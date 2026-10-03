package bridge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"mega-magic/internal/database"
	"mega-magic/internal/media/jellyfin"
)

type Postgres struct{ db *database.DB }

func NewPostgres(db *database.DB) *Postgres { return &Postgres{db: db} }

// ResolveConnector authenticates relay traffic with the existing expiring PC
// token. A supplied tenant ID is never accepted as authentication.
func (p *Postgres) ResolveConnector(ctx context.Context, hash string) (string, error) {
	return p.resolve(ctx, "token", hash)
}
func (p *Postgres) tenant(ctx context.Context, bid string, fn func(*sql.Tx) error) error {
	tx, e := p.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if e = database.WithTenant(tx, bid, func() error { return fn(tx) }); e != nil {
		return e
	}
	return tx.Commit()
}
func (p *Postgres) Pair(ctx context.Context, bid, hash string) error {
	return p.tenant(ctx, bid, func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, `INSERT INTO magic_media_connections(business_id,pairing_hash,pairing_expires_at) VALUES($1,$2,now()+interval '10 minutes')
 ON CONFLICT(business_id) DO UPDATE SET pairing_hash=$2,pairing_expires_at=now()+interval '10 minutes',updated_at=now()`, bid, hash)
		return e
	})
}

// Only these two lookups use the owner pool: an anonymous connector does not yet
// have tenant context. Resolve through a secret hash, never a supplied business ID.
func (p *Postgres) resolve(ctx context.Context, column, hash string) (string, error) {
	var bid string
	query := `SELECT m.business_id FROM magic_media_connections m JOIN businesses b USING(business_id) WHERE b.is_active AND `
	if column == "pair" {
		query += `pairing_hash=$1 AND pairing_expires_at>now()`
	} else {
		query += `token_hash=$1 AND token_expires_at>now() AND revoked_at IS NULL`
	}
	e := p.db.QueryRowContext(ctx, query, hash).Scan(&bid)
	if e != nil {
		return "", ErrUnavailable
	}
	return bid, nil
}
func (p *Postgres) Redeem(ctx context.Context, pair, token, name string) error {
	bid, e := p.resolve(ctx, "pair", pair)
	if e != nil {
		return e
	}
	return p.tenant(ctx, bid, func(tx *sql.Tx) error {
		res, e := tx.ExecContext(ctx, `UPDATE magic_media_connections SET pairing_hash=NULL,pairing_expires_at=NULL,token_hash=$3,token_expires_at=now()+interval '24 hours',server_name=$4,last_seen_at=now(),revoked_at=NULL,updated_at=now()
	WHERE business_id=$1 AND pairing_hash=$2 AND pairing_expires_at>now() AND EXISTS(SELECT 1 FROM businesses b WHERE b.business_id=$1 AND b.is_active)`, bid, pair, token, name)
		if e != nil {
			return e
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return ErrUnavailable
		}
		_, e = tx.ExecContext(ctx, `UPDATE magic_media_jobs SET state='cancelled',completed_at=now() WHERE business_id=$1 AND state IN ('queued','running')`, bid)
		return e
	})
}
func (p *Postgres) Connection(ctx context.Context, bid string) (Connection, error) {
	var c Connection
	e := p.tenant(ctx, bid, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `SELECT server_name,last_seen_at,token_expires_at,revoked_at FROM magic_media_connections WHERE business_id=$1`, bid).Scan(&c.ServerName, &c.LastSeen, &c.TokenExpires, &c.Revoked)
	})
	if errors.Is(e, sql.ErrNoRows) {
		return c, nil
	}
	return c, e
}
func (p *Postgres) Revoke(ctx context.Context, bid string) error {
	return p.tenant(ctx, bid, func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, `UPDATE magic_media_connections SET revoked_at=now(),token_hash=NULL,pairing_hash=NULL,updated_at=now() WHERE business_id=$1`, bid)
		if e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, `UPDATE magic_media_jobs SET state='cancelled',completed_at=now() WHERE business_id=$1 AND state IN ('queued','running')`, bid)
		return e
	})
}
func (p *Postgres) Enqueue(ctx context.Context, bid, actor string, j Job) error {
	return p.tenant(ctx, bid, func(tx *sql.Tx) error {
		var locked string
		if e := tx.QueryRowContext(ctx, `SELECT business_id FROM magic_media_connections WHERE business_id=$1 AND token_hash IS NOT NULL AND revoked_at IS NULL AND token_expires_at>now() AND last_seen_at>now()-interval '30 seconds' FOR UPDATE`, bid).Scan(&locked); e != nil {
			return ErrUnavailable
		}
		var count int
		if e := tx.QueryRowContext(ctx, `SELECT count(*) FROM magic_media_jobs WHERE business_id=$1 AND state IN ('queued','running') AND expires_at>now()`, bid).Scan(&count); e != nil {
			return e
		}
		if count >= 10 {
			return ErrUnavailable
		}
		if e := tx.QueryRowContext(ctx, `SELECT count(*) FROM magic_media_jobs WHERE business_id=$1 AND created_at>now()-interval '24 hours'`, bid).Scan(&count); e != nil {
			return e
		}
		if count >= 1000 {
			return ErrUnavailable
		}
		_, e := tx.ExecContext(ctx, `INSERT INTO magic_media_jobs(id,business_id,actor_id,operation,payload) VALUES($1,$2,$3,$4,$5)`, j.ID, bid, actor, j.Operation, string(j.Payload))
		return e
	})
}
func scanJob(row *sql.Row) (Job, error) {
	var j Job
	// A queued job has SQL NULL for result. database/sql cannot scan that into
	// *json.RawMessage; []byte supports NULL and preserves it as a nil slice.
	var payload, result []byte
	e := row.Scan(&j.ID, &j.Operation, &payload, &j.State, &result, &j.ErrorCode, &j.Expires)
	j.Payload, j.Result = json.RawMessage(payload), json.RawMessage(result)
	return j, e
}
func (p *Postgres) GetJob(ctx context.Context, bid, id string) (Job, error) {
	var j Job
	e := p.tenant(ctx, bid, func(tx *sql.Tx) error {
		var e error
		j, e = scanJob(tx.QueryRowContext(ctx, `SELECT id,operation,payload,CASE WHEN expires_at<now() AND state IN ('queued','running') THEN 'expired' ELSE state END,result,error_code,expires_at FROM magic_media_jobs WHERE business_id=$1 AND id=$2`, bid, id))
		return e
	})
	if errors.Is(e, sql.ErrNoRows) {
		e = ErrUnavailable
	}
	return j, e
}
func (p *Postgres) connector(ctx context.Context, hash string, fn func(*sql.Tx, string) error) error {
	bid, e := p.resolve(ctx, "token", hash)
	if e != nil {
		return e
	}
	return p.tenant(ctx, bid, func(tx *sql.Tx) error {
		var id string
		if e := tx.QueryRowContext(ctx, `SELECT m.business_id FROM magic_media_connections m JOIN businesses b USING(business_id) WHERE m.business_id=$1 AND m.token_hash=$2 AND m.token_expires_at>now() AND m.revoked_at IS NULL AND b.is_active FOR UPDATE OF m`, bid, hash).Scan(&id); e != nil {
			return ErrUnavailable
		}
		if _, e := tx.ExecContext(ctx, `UPDATE magic_media_connections SET last_seen_at=now() WHERE business_id=$1`, bid); e != nil {
			return e
		}
		return fn(tx, bid)
	})
}
func (p *Postgres) Poll(ctx context.Context, hash string) (*Job, error) {
	var job *Job
	e := p.connector(ctx, hash, func(tx *sql.Tx, bid string) error {
		j, e := scanJob(tx.QueryRowContext(ctx, `UPDATE magic_media_jobs SET state='running',started_at=now(),claimed_token_hash=$2
	 WHERE id=(SELECT j.id FROM magic_media_jobs j JOIN users u ON u.user_id=j.actor_id AND u.business_id=j.business_id WHERE j.business_id=$1 AND j.state='queued' AND j.expires_at>now() AND u.is_active AND u.user_role IN ('ADMIN','OWNER','DEVELOPER') ORDER BY j.created_at LIMIT 1 FOR UPDATE OF j SKIP LOCKED)
 RETURNING id,operation,payload,state,result,error_code,expires_at`, bid, hash))
		if errors.Is(e, sql.ErrNoRows) {
			return nil
		}
		if e == nil {
			job = &j
		}
		return e
	})
	return job, e
}
func (p *Postgres) Complete(ctx context.Context, hash, id string, c Completion) error {
	return p.connector(ctx, hash, func(tx *sql.Tx, bid string) error {
		var op string
		if e := tx.QueryRowContext(ctx, `SELECT operation FROM magic_media_jobs WHERE business_id=$1 AND id=$2 AND claimed_token_hash=$3 AND state='running' AND expires_at>now() FOR UPDATE`, bid, id, hash).Scan(&op); e != nil {
			return ErrUnavailable
		}
		// Persist only the result for the requested operation, never unrelated data.
		clean := Completion{ErrorCode: c.ErrorCode}
		if c.ErrorCode == "" {
			switch op {
			case "dashboard":
				if c.Dashboard == nil || jellyfin.ValidateDashboard(*c.Dashboard) != nil {
					return ErrUnavailable
				}
				clean.Dashboard = c.Dashboard
			case "status":
				if c.Status == nil {
					return ErrUnavailable
				}
				clean.Status = c.Status
			case "libraries":
				clean.Libraries = c.Libraries
			case "scan", "add_library":
				if !c.Accepted {
					return ErrUnavailable
				}
				clean.Accepted = true
			}
		}
		state := "succeeded"
		if clean.ErrorCode != "" {
			state = "failed"
		}
		data, e := json.Marshal(clean)
		if e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, `UPDATE magic_media_jobs SET state=$3,result=$4,error_code=$5,completed_at=now() WHERE business_id=$1 AND id=$2`, bid, id, state, string(data), clean.ErrorCode)
		return e
	})
}
