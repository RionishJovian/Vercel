package database

import (
	"database/sql"
	"errors"
	"fmt"

	"mega-magic/internal/model"
)

// ErrInsufficientCoins is returned when a debit would drive a wallet negative.
var ErrInsufficientCoins = errors.New("insufficient coin balance")

// ErrCoinUserNotInBusiness is returned when the target user is not a member of
// the business the caller is acting in — a tenant-isolation guard that does not
// rely on RLS alone.
var ErrCoinUserNotInBusiness = errors.New("user does not belong to this business")

// MagicCoinRepository reads and writes the coin ledger. All methods take a *sql.Tx
// because every one of them must run inside a tenant-bound transaction (RLS
// context set), exactly like the other money paths.
type MagicCoinRepository struct {
	db *DB
}

// NewMagicCoinRepository creates a coin repository.
func NewMagicCoinRepository(db *DB) *MagicCoinRepository {
	return &MagicCoinRepository{db: db}
}

// GetWalletTXN returns a user's wallet, or (nil, nil) when there is nobody to
// report — the caller renders that as a zero balance rather than an error.
func (r *MagicCoinRepository) GetWalletTXN(tx *sql.Tx, businessID, userID string) (*model.MagicCoinWallet, error) {
	query := `
		SELECT user_id, business_id, balance, created_at, updated_at
		  FROM magic_coin_wallets
		 WHERE user_id = $1 AND business_id = $2`

	w := &model.MagicCoinWallet{}
	err := tx.QueryRow(query, userID, businessID).Scan(
		&w.UserID, &w.BusinessID, &w.Balance, &w.CreatedAt, &w.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get coin wallet: %w", err)
	}
	return w, nil
}

// ListTransactionsTXN returns the most recent ledger entries for the whole
// business (the owner sees every member's activity). The ledger is append-only,
// so this is a complete audit trail.
func (r *MagicCoinRepository) ListTransactionsTXN(tx *sql.Tx, businessID string, limit int) ([]*model.MagicCoinTransaction, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `
		SELECT id, business_id, user_id, delta, balance_after, kind, note,
		       service_purchase_id, actor_user_id, created_at
		  FROM magic_coin_transactions
		 WHERE business_id = $1
		 ORDER BY created_at DESC, id DESC
		 LIMIT $2`

	rows, err := tx.Query(query, businessID, limit)
	if err != nil {
		return nil, fmt.Errorf("list coin transactions: %w", err)
	}
	defer rows.Close()

	out := []*model.MagicCoinTransaction{}
	for rows.Next() {
		t := &model.MagicCoinTransaction{}
		var svcID, actorID sql.NullString
		if err := rows.Scan(
			&t.ID, &t.BusinessID, &t.UserID, &t.Delta, &t.BalanceAfter, &t.Kind,
			&t.Note, &svcID, &actorID, &t.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan coin transaction: %w", err)
		}
		if svcID.Valid {
			t.ServicePurchaseID = &svcID.String
		}
		if actorID.Valid {
			t.ActorUserID = &actorID.String
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate coin transactions: %w", err)
	}
	return out, nil
}

// AdjustTXN applies a manual credit or debit atomically.
//
// Order matters and is deliberate: lock the wallet row, compute the new balance,
// insert the immutable ledger entry carrying balance_after, then update the
// balance. The ledger row is written BEFORE the balance moves so that a crash
// between the two (impossible inside one tx, but harmless to rely on) could never
// leave a balance change with no explanation. The reverse order could.
func (r *MagicCoinRepository) AdjustTXN(
	tx *sql.Tx, businessID, userID string, delta float64,
	kind model.MagicCoinKind, note, actorUserID string,
) (*model.MagicCoinTransaction, error) {
	if !kind.Valid() {
		return nil, fmt.Errorf("invalid coin kind %q", kind)
	}
	if delta == 0 {
		return nil, errors.New("amount must not be zero")
	}
	// A credit is positive, a debit negative. Reject a sign that contradicts the
	// kind so the ledger can never disagree with its own label.
	if kind == model.CoinManualCredit && delta < 0 {
		return nil, errors.New("manual_credit requires a positive amount")
	}
	if kind == model.CoinManualDebit && delta > 0 {
		return nil, errors.New("manual_debit requires a negative amount")
	}

	// The target must belong to the caller's business.
	var exists bool
	if err := tx.QueryRow(
		`SELECT EXISTS (SELECT 1 FROM users WHERE user_id = $1 AND business_id = $2)`,
		userID, businessID,
	).Scan(&exists); err != nil {
		return nil, fmt.Errorf("verify coin target: %w", err)
	}
	if !exists {
		return nil, ErrCoinUserNotInBusiness
	}

	// Wallets are auto-created by a trigger on users, but an account predating
	// that trigger may not have one — so create it on demand.
	if _, err := tx.Exec(
		`INSERT INTO magic_coin_wallets (user_id, business_id)
		 VALUES ($1, $2) ON CONFLICT (user_id) DO NOTHING`,
		userID, businessID,
	); err != nil {
		return nil, fmt.Errorf("ensure coin wallet: %w", err)
	}

	// Lock the wallet so two concurrent adjustments cannot both read the same
	// balance and write an inconsistent balance_after.
	var balance float64
	if err := tx.QueryRow(
		`SELECT balance FROM magic_coin_wallets WHERE user_id = $1 FOR UPDATE`,
		userID,
	).Scan(&balance); err != nil {
		return nil, fmt.Errorf("lock coin wallet: %w", err)
	}

	newBalance := balance + delta
	if newBalance < 0 {
		return nil, ErrInsufficientCoins
	}

	t := &model.MagicCoinTransaction{}
	var svcID, actorID sql.NullString
	// actor_user_id may be empty for a system-initiated adjustment.
	var actor any
	if actorUserID != "" {
		actor = actorUserID
	}
	err := tx.QueryRow(`
		INSERT INTO magic_coin_transactions
		    (business_id, user_id, delta, balance_after, kind, note, actor_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, business_id, user_id, delta, balance_after, kind, note,
		          service_purchase_id, actor_user_id, created_at`,
		businessID, userID, delta, newBalance, string(kind), note, actor,
	).Scan(
		&t.ID, &t.BusinessID, &t.UserID, &t.Delta, &t.BalanceAfter, &t.Kind,
		&t.Note, &svcID, &actorID, &t.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert coin transaction: %w", err)
	}
	if svcID.Valid {
		t.ServicePurchaseID = &svcID.String
	}
	if actorID.Valid {
		t.ActorUserID = &actorID.String
	}

	if _, err := tx.Exec(
		`UPDATE magic_coin_wallets SET balance = $1, updated_at = now() WHERE user_id = $2`,
		newBalance, userID,
	); err != nil {
		return nil, fmt.Errorf("update coin balance: %w", err)
	}

	return t, nil
}
