package bridge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
)

// Commerce is independent of guest appearance. The customer workflow is
// bank/MMQR receipt review by the tenant's authorized owner. The optional
// settlement primitive below has no public route. Published terms are authoritative.
var ErrPaymentMismatch = errors.New("payment does not match this purchase")
var ErrPassDenied = errors.New("media pass is not available")
var providerName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,39}$`)

type MediaPass struct {
	Payment         *MediaPayment `json:"payment,omitempty"`
	ID              string        `json:"id"`
	Amount          int           `json:"amount"`
	Currency        string        `json:"currency"`
	DurationSeconds int           `json:"duration_seconds"`
	State           string        `json:"state"`
	CreatedAt       time.Time     `json:"created_at"`
	PaymentDeadline time.Time     `json:"payment_deadline"`
	PaidAt          *time.Time    `json:"paid_at,omitempty"`
	ActivatedAt     *time.Time    `json:"activated_at,omitempty"`
	ExpiresAt       *time.Time    `json:"expires_at,omitempty"`
}

// SettledPayment is adapter-internal, never decoded from a guest HTTP request.
// Provider-specific signatures, merchant identity and settlement verification
// must happen before this is constructed. Refunds are a distinct transition.
type SettledPayment struct {
	PassID, Provider, Reference, Currency string
	Amount                                int
}

func (p SettledPayment) validate() bool {
	return secretPattern.MatchString(p.PassID) && providerName.MatchString(p.Provider) &&
		len(p.Reference) > 0 && len(p.Reference) <= 200 && strings.TrimSpace(p.Reference) == p.Reference &&
		!strings.ContainsAny(p.Reference, "\x00\r\n") && p.Currency == "MMK" && p.Amount > 0 && p.Amount <= 1000000
}

const passColumns = `id,amount,currency,duration_seconds,state,created_at,payment_deadline,paid_at,activated_at,expires_at,payment_details`

func scanPass(row interface{ Scan(...any) error }) (MediaPass, error) {
	var p MediaPass
	var payment []byte
	e := row.Scan(&p.ID, &p.Amount, &p.Currency, &p.DurationSeconds, &p.State, &p.CreatedAt, &p.PaymentDeadline, &p.PaidAt, &p.ActivatedAt, &p.ExpiresAt, &payment)
	if e == nil {
		e = json.Unmarshal(payment, &p.Payment)
	}
	return p, e
}

func (p *Postgres) CreateMediaPass(ctx context.Context, bid, publicID, guestHash, key string) (out MediaPass, e error) {
	if !UUID.MatchString(bid) || !secretPattern.MatchString(publicID) || !secretPattern.MatchString(guestHash) || !secretPattern.MatchString(key) {
		return out, ErrPassDenied
	}
	id, e := Secret()
	if e != nil {
		return out, e
	}
	e = p.tenant(ctx, bid, func(tx *sql.Tx) error {
		// Idempotent retry returns the same snapshotted terms, including after an
		// owner edits the design. Never extend payment or access deadlines on retry.
		var err error
		out, err = scanPass(tx.QueryRowContext(ctx, `SELECT `+passColumns+` FROM magic_media_passes WHERE business_id=$1 AND guest_hash=$2 AND checkout_key=$3 AND publication_id=$4 AND EXISTS(SELECT 1 FROM businesses WHERE business_id=$1 AND is_active)`, bid, guestHash, key, publicID))
		if err == nil {
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var raw []byte
		if err = tx.QueryRowContext(ctx, `SELECT m.design FROM magic_media_publications m JOIN businesses b USING(business_id) WHERE m.business_id=$1 AND m.public_id=$2 AND b.is_active`, bid, publicID).Scan(&raw); err != nil {
			return ErrPassDenied
		}
		d, err := DecodeMediaDesign(strings.NewReader(string(raw)))
		if err != nil {
			return ErrPassDenied
		}
		// The appearance editor historically allowed an unbounded number of hours.
		// Commerce must reject an unsellable configuration, not silently truncate it.
		if d.Duration > 720 {
			return ErrPassDenied
		}
		if d.Payment == nil || d.Payment.Validate() != nil {
			return ErrPassDenied
		}
		payment, err := json.Marshal(d.Payment)
		if err != nil {
			return err
		}
		out, err = scanPass(tx.QueryRowContext(ctx, `INSERT INTO magic_media_passes(id,business_id,guest_hash,checkout_key,publication_id,amount,currency,duration_seconds,payment_details)
   VALUES($1,$2,$3,$4,$5,$6,'MMK',$7,$8::jsonb)
   ON CONFLICT(business_id,guest_hash,checkout_key) DO UPDATE SET checkout_key=EXCLUDED.checkout_key
   WHERE magic_media_passes.publication_id=EXCLUDED.publication_id
   RETURNING `+passColumns, id, bid, guestHash, key, publicID, d.Price, d.Duration*3600, string(payment)))
		return err
	})
	if errors.Is(e, sql.ErrNoRows) {
		e = ErrPassDenied
	}
	return
}

func (p *Postgres) MediaPass(ctx context.Context, bid, id, guestHash string) (out MediaPass, e error) {
	if !secretPattern.MatchString(id) || !secretPattern.MatchString(guestHash) {
		return out, ErrPassDenied
	}
	e = p.tenant(ctx, bid, func(tx *sql.Tx) error {
		var err error
		out, err = scanPass(tx.QueryRowContext(ctx, `SELECT `+passColumns+` FROM magic_media_passes WHERE business_id=$1 AND id=$2 AND guest_hash=$3 AND EXISTS(SELECT 1 FROM businesses WHERE business_id=$1 AND is_active)`, bid, id, guestHash))
		return err
	})
	if errors.Is(e, sql.ErrNoRows) {
		e = ErrPassDenied
	}
	return
}

func (p *Postgres) SettleMediaPass(ctx context.Context, bid string, payment SettledPayment) (out MediaPass, e error) {
	if !payment.validate() {
		return out, ErrPaymentMismatch
	}
	e = p.tenant(ctx, bid, func(tx *sql.Tx) error {
		var provider, reference sql.NullString
		var amount int
		var currency, state string
		var deadline time.Time
		if err := tx.QueryRowContext(ctx, `SELECT amount,currency,state,payment_deadline,provider,payment_reference FROM magic_media_passes WHERE business_id=$1 AND id=$2 AND EXISTS(SELECT 1 FROM businesses WHERE business_id=$1 AND is_active) FOR UPDATE`, bid, payment.PassID).Scan(&amount, &currency, &state, &deadline, &provider, &reference); err != nil {
			return ErrPassDenied
		}
		if amount != payment.Amount || currency != payment.Currency {
			return ErrPaymentMismatch
		}
		if state == "paid" && provider.String == payment.Provider && reference.String == payment.Reference {
			var err error
			out, err = scanPass(tx.QueryRowContext(ctx, `SELECT `+passColumns+` FROM magic_media_passes WHERE business_id=$1 AND id=$2`, bid, payment.PassID))
			return err
		}
		// A late real payment requires explicit reconciliation/refund. It must not
		// automatically create access after an abandoned checkout has expired.
		if state != "pending" {
			return ErrPassDenied
		}
		var err error
		out, err = scanPass(tx.QueryRowContext(ctx, `UPDATE magic_media_passes SET state='paid',provider=$3,payment_reference=$4,paid_at=now() WHERE business_id=$1 AND id=$2 AND payment_deadline>now() RETURNING `+passColumns, bid, payment.PassID, payment.Provider, payment.Reference))
		if errors.Is(err, sql.ErrNoRows) {
			return ErrPassDenied
		}
		return err
	})
	// Never expose a provider reference through a PostgreSQL unique-violation
	// detail. The same authenticated payment cannot fund a second order.
	var state interface{ SQLState() string }
	if errors.As(e, &state) && state.SQLState() == "23505" {
		e = ErrPaymentMismatch
	}
	return
}

// Activate starts time exactly once, only after verified settlement. There is
// no purchased-duration read from current appearance and no retry renewal.
func (p *Postgres) ActivateMediaPass(ctx context.Context, bid, id, guestHash string) (out MediaPass, e error) {
	if !secretPattern.MatchString(id) || !secretPattern.MatchString(guestHash) {
		return out, ErrPassDenied
	}
	e = p.tenant(ctx, bid, func(tx *sql.Tx) error {
		var err error
		out, err = scanPass(tx.QueryRowContext(ctx, `UPDATE magic_media_passes SET activated_at=coalesce(activated_at,now()),expires_at=coalesce(expires_at,now()+duration_seconds*interval '1 second') WHERE business_id=$1 AND id=$2 AND guest_hash=$3 AND state='paid' AND (expires_at IS NULL OR expires_at>now()) AND EXISTS(SELECT 1 FROM businesses WHERE business_id=$1 AND is_active) RETURNING `+passColumns, bid, id, guestHash))
		if errors.Is(err, sql.ErrNoRows) {
			return ErrPassDenied
		}
		return err
	})
	return
}

// Access never relies on a UI countdown or expiry worker. The database checks
// the absolute deadline on every authorization, including after API restarts.
func (p *Postgres) AuthorizeMediaPass(ctx context.Context, bid, id, guestHash string) (out MediaPass, e error) {
	if !secretPattern.MatchString(id) || !secretPattern.MatchString(guestHash) {
		return out, ErrPassDenied
	}
	e = p.tenant(ctx, bid, func(tx *sql.Tx) error {
		var err error
		out, err = scanPass(tx.QueryRowContext(ctx, `SELECT `+passColumns+` FROM magic_media_passes WHERE business_id=$1 AND id=$2 AND guest_hash=$3 AND state='paid' AND activated_at IS NOT NULL AND expires_at>now() AND EXISTS(SELECT 1 FROM businesses WHERE business_id=$1 AND is_active)`, bid, id, guestHash))
		if errors.Is(err, sql.ErrNoRows) {
			return ErrPassDenied
		}
		return err
	})
	return
}

// Revoke is idempotent and cannot resurrect a rejected/refunded purchase.
func (p *Postgres) RevokeMediaPass(ctx context.Context, bid, id string, refund bool) error {
	if !secretPattern.MatchString(id) {
		return ErrPassDenied
	}
	return p.tenant(ctx, bid, func(tx *sql.Tx) error {
		target, from := "rejected", "pending"
		if refund {
			target, from = "refunded", "paid"
		}
		res, e := tx.ExecContext(ctx, `UPDATE magic_media_passes SET state=$3 WHERE business_id=$1 AND id=$2 AND state IN ($3,$4)`, bid, id, target, from)
		if e != nil {
			return e
		}
		n, e := res.RowsAffected()
		if e == nil && n != 1 {
			return ErrPassDenied
		}
		return e
	})
}
