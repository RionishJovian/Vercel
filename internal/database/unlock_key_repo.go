package database

import (
	"database/sql"
	"fmt"

	"mega-magic/internal/model"
)

// MaxUnlockKeyPage caps a page of the key ledger.
const MaxUnlockKeyPage = 200

// DefaultUnlockKeyPage matches the console's page size.
const DefaultUnlockKeyPage = 50

// unlockStatusSQL derives the status. Declared once and used by BOTH the list and
// the summary, so a filter and the counts above it can never disagree.
//
// consumed wins over expired: a key that was used and later lapsed was still used.
const unlockStatusSQL = `CASE WHEN consumed_at IS NOT NULL THEN 'consumed'
                              WHEN expires_at < now() THEN 'expired'
                              ELSE 'active' END`

// UnlockKeyRepository reads the key-activation ledger.
type UnlockKeyRepository struct {
	db *DB
}

// NewUnlockKeyRepository creates an unlock key repository.
func NewUnlockKeyRepository(db *DB) *UnlockKeyRepository {
	return &UnlockKeyRepository{db: db}
}

// ListUnlockKeysTXN returns one page of activations, newest first, plus the total
// matching the filters.
func (r *UnlockKeyRepository) ListUnlockKeysTXN(tx *sql.Tx, businessID, kind, status, q string, limit, offset int) ([]*model.UnlockKeyActivation, int, error) {
	where := "business_id = $1"
	args := []any{businessID}

	if kind != "" {
		args = append(args, kind)
		where += fmt.Sprintf(" AND key_kind = $%d", len(args))
	}
	if status != "" {
		// Validated by the handler against the closed set; the expression is a
		// constant, never caller input.
		args = append(args, status)
		where += fmt.Sprintf(" AND (%s) = $%d", unlockStatusSQL, len(args))
	}
	if q != "" {
		args = append(args, "%"+escapeLike(q)+"%")
		n := len(args)
		where += fmt.Sprintf(" AND (coalesce(key_name,'') ILIKE $%d ESCAPE '\\' OR coalesce(attributes,'') ILIKE $%d ESCAPE '\\')", n, n)
	}

	var total int
	if err := tx.QueryRow(`SELECT count(*) FROM unlock_key_activations WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count unlock keys: %w", err)
	}

	pageArgs := append(append([]any{}, args...), limit, offset)
	rows, err := tx.Query(fmt.Sprintf(`
		SELECT id, business_id, user_id, key_kind, key_id, key_name, price_coins, attributes,
		       account_bound, non_transferable, unique_router_key, grace_days,
		       consumed_at, consumed_ref_id, created_at, expires_at, (%s) AS status
		  FROM unlock_key_activations
		 WHERE %s
		 ORDER BY created_at DESC, id
		 LIMIT $%d OFFSET $%d`, unlockStatusSQL, where, len(pageArgs)-1, len(pageArgs)), pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list unlock keys: %w", err)
	}
	defer rows.Close()

	out := []*model.UnlockKeyActivation{}
	for rows.Next() {
		k := &model.UnlockKeyActivation{}
		var keyID, graceDays sql.NullInt32
		var keyName, consumedRef sql.NullString
		var consumedAt sql.NullTime
		if err := rows.Scan(&k.ID, &k.BusinessID, &k.UserID, &k.KeyKind, &keyID, &keyName,
			&k.PriceCoins, &k.Attributes, &k.AccountBound, &k.NonTransferable,
			&k.UniqueRouterKey, &graceDays, &consumedAt, &consumedRef,
			&k.CreatedAt, &k.ExpiresAt, &k.Status); err != nil {
			return nil, 0, fmt.Errorf("scan unlock key: %w", err)
		}
		if keyID.Valid {
			v := int(keyID.Int32)
			k.KeyID = &v
		}
		if keyName.Valid {
			k.KeyName = keyName.String
		}
		if graceDays.Valid {
			v := int(graceDays.Int32)
			k.GraceDays = &v
		}
		if consumedAt.Valid {
			k.ConsumedAt = &consumedAt.Time
		}
		if consumedRef.Valid {
			k.ConsumedRefID = &consumedRef.String
		}
		out = append(out, k)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate unlock keys: %w", err)
	}
	return out, total, nil
}

// UnlockKeySummaryTXN returns the tenant-wide totals and the per-kind breakdown.
// Unfiltered on purpose: it is the header behind the list.
func (r *UnlockKeyRepository) UnlockKeySummaryTXN(tx *sql.Tx, businessID string) (*model.UnlockKeySummary, error) {
	s := &model.UnlockKeySummary{}
	err := tx.QueryRow(fmt.Sprintf(`
		SELECT count(*), coalesce(sum(price_coins),0),
		       count(*) filter (where (%s) = 'active'),
		       count(*) filter (where (%s) = 'expired'),
		       count(*) filter (where (%s) = 'consumed')
		  FROM unlock_key_activations WHERE business_id = $1`,
		unlockStatusSQL, unlockStatusSQL, unlockStatusSQL), businessID).Scan(
		&s.Activations, &s.CoinsSpent, &s.Active, &s.Expired, &s.Consumed)
	if err != nil {
		return nil, fmt.Errorf("unlock key summary: %w", err)
	}

	rows, err := tx.Query(`
		SELECT key_kind, count(*), coalesce(sum(price_coins),0)
		  FROM unlock_key_activations WHERE business_id = $1
		 GROUP BY 1 ORDER BY 3 DESC, 1`, businessID)
	if err != nil {
		return nil, fmt.Errorf("unlock key by kind: %w", err)
	}
	defer rows.Close()

	s.ByKind = []model.UnlockKeyGroup{}
	for rows.Next() {
		var g model.UnlockKeyGroup
		if err := rows.Scan(&g.Name, &g.Count, &g.CoinsSpent); err != nil {
			return nil, fmt.Errorf("scan unlock key group: %w", err)
		}
		s.ByKind = append(s.ByKind, g)
	}
	return s, rows.Err()
}
