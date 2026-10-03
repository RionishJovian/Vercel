package bridge

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// MediaReceipt is internal-only: paths must come from tenant-scoped SaveReceipt,
// not guest JSON. ReviewID fits Telegram's callback limit; the 64-byte pass ID
// would not fit when prefixed with "approve ". No storage path is public.
type MediaReceipt struct {
	ReviewID string
	Path     string
	Hash     string
}

// ResolveMediaReview is used only after the owner-bot authorization guard.
// It is an owner-pool identity lookup; all decisions still enter tenant RLS.
func (p *Postgres) ResolveMediaReview(ctx context.Context, reviewID string) (string, error) {
	if !UUID.MatchString(reviewID) {
		return "", ErrPassDenied
	}
	var bid string
	err := p.db.QueryRowContext(ctx, `SELECT m.business_id FROM magic_media_passes m JOIN businesses b USING(business_id) WHERE review_id=$1 AND b.is_active`, reviewID).Scan(&bid)
	if err != nil {
		return "", ErrPassDenied
	}
	return bid, nil
}

// SubmitMediaReceipt binds an immutable receipt to the guest's pending order.
// Submission is not approval and cannot activate playback. Same-image retries
// return the original stored receipt; changing an already submitted proof fails.
func (p *Postgres) SubmitMediaReceipt(ctx context.Context, bid, id, guestHash, path, hash string) (out MediaReceipt, e error) {
	if !UUID.MatchString(bid) || !secretPattern.MatchString(id) || !secretPattern.MatchString(guestHash) || !secretPattern.MatchString(hash) || path == "" || len(path) > 4096 || strings.ContainsAny(path, "\x00\r\n") {
		return out, ErrPassDenied
	}
	e = p.tenant(ctx, bid, func(tx *sql.Tx) error {
		var oldPath, oldHash sql.NullString
		var state string
		var timely bool
		err := tx.QueryRowContext(ctx, `SELECT review_id,receipt_path,receipt_hash,state,payment_deadline>now() FROM magic_media_passes WHERE business_id=$1 AND id=$2 AND guest_hash=$3 AND EXISTS(SELECT 1 FROM businesses WHERE business_id=$1 AND is_active) FOR UPDATE`, bid, id, guestHash).Scan(&out.ReviewID, &oldPath, &oldHash, &state, &timely)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrPassDenied
		}
		if err != nil {
			return err
		}
		if state != "pending" {
			return ErrPassDenied
		}
		if oldHash.Valid {
			if oldHash.String != hash {
				return ErrPassDenied
			}
			out.Path, out.Hash = oldPath.String, oldHash.String
			return nil
		}
		if !timely {
			return ErrPassDenied
		}
		_, err = tx.ExecContext(ctx, `UPDATE magic_media_passes SET receipt_path=$4,receipt_hash=$5,receipt_submitted_at=now() WHERE business_id=$1 AND id=$2 AND guest_hash=$3`, bid, id, guestHash, path, hash)
		out.Path, out.Hash = path, hash
		return err
	})
	return
}

// DecideMediaReceipt is a trusted owner/admin seam, never a public guest route.
// HTTP callers must check staff role; owner-bot callers must use the existing
// per-business chat/user authorization guard before invoking it. Receipt arrival
// must be timely, but the owner's review can happen after the upload deadline.
// Opposite decisions and reapproval of refunded orders cannot resurrect access.
func (p *Postgres) DecideMediaReceipt(ctx context.Context, bid, reviewID string, approve bool) (out MediaPass, e error) {
	if !UUID.MatchString(bid) || !UUID.MatchString(reviewID) {
		return out, ErrPassDenied
	}
	e = p.tenant(ctx, bid, func(tx *sql.Tx) error {
		var id, state string
		var submitted bool
		err := tx.QueryRowContext(ctx, `SELECT id,state,receipt_submitted_at IS NOT NULL FROM magic_media_passes WHERE business_id=$1 AND review_id=$2 AND EXISTS(SELECT 1 FROM businesses WHERE business_id=$1 AND is_active) FOR UPDATE`, bid, reviewID).Scan(&id, &state, &submitted)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrPassDenied
		}
		if err != nil {
			return err
		}
		target := "rejected"
		if approve {
			target = "paid"
		}
		if !submitted || (state != "pending" && state != target) {
			return ErrPassDenied
		}
		if state == "pending" {
			if approve {
				// A proof image cannot approve two orders for the same business.
				// The unique provider/reference constraint serializes concurrent replay.
				_, err = tx.ExecContext(ctx, `UPDATE magic_media_passes SET state='paid',paid_at=now(),decided_at=now(),provider='owner_bank_receipt',payment_reference=$1::text || ':' || receipt_hash WHERE business_id=$1 AND id=$2`, bid, id)
			} else {
				_, err = tx.ExecContext(ctx, `UPDATE magic_media_passes SET state='rejected',decided_at=now() WHERE business_id=$1 AND id=$2`, bid, id)
			}
			if err != nil {
				return err
			}
		}
		out, err = scanPass(tx.QueryRowContext(ctx, `SELECT `+passColumns+` FROM magic_media_passes WHERE business_id=$1 AND id=$2`, bid, id))
		return err
	})
	var state interface{ SQLState() string }
	if errors.As(e, &state) && state.SQLState() == "23505" {
		e = ErrPaymentMismatch
	}
	return
}
