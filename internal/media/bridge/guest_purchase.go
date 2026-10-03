package bridge

import (
	"context"
	"database/sql"
)

func (p *Postgres) ResolveMediaPublication(ctx context.Context, id string) (string, error) {
	if !secretPattern.MatchString(id) {
		return "", ErrPassDenied
	}
	var bid string
	e := p.db.QueryRowContext(ctx, `SELECT m.business_id FROM magic_media_publications m JOIN businesses b USING(business_id) WHERE public_id=$1 AND b.is_active`, id).Scan(&bid)
	if e != nil {
		return "", ErrPassDenied
	}
	return bid, nil
}

// ResolveGuestMediaPass identifies only a matching guest secret/publication/pass
// tuple. A client-supplied business ID is never authoritative.
func (p *Postgres) ResolveGuestMediaPass(ctx context.Context, publicID, id, guestHash string) (string, error) {
	if !secretPattern.MatchString(publicID) || !secretPattern.MatchString(id) || !secretPattern.MatchString(guestHash) {
		return "", ErrPassDenied
	}
	var bid string
	e := p.db.QueryRowContext(ctx, `SELECT m.business_id FROM magic_media_passes m JOIN businesses b USING(business_id) WHERE publication_id=$1 AND id=$2 AND guest_hash=$3 AND b.is_active`, publicID, id, guestHash).Scan(&bid)
	if e != nil {
		return "", ErrPassDenied
	}
	return bid, nil
}

func (p *Postgres) MediaReceiptSubmitted(ctx context.Context, bid, id, guestHash string) (out bool, e error) {
	e = p.tenant(ctx, bid, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `SELECT receipt_submitted_at IS NOT NULL FROM magic_media_passes WHERE business_id=$1 AND id=$2 AND guest_hash=$3`, bid, id, guestHash).Scan(&out)
	})
	return
}
