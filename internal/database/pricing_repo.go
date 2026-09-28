package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"mega-magic/internal/model"
)

// ErrPricingPromoNotFound is returned when the pricing singleton row is absent.
// The row is created once by migration; a missing row means the platform has no
// published prices, which the caller must surface rather than paper over with a
// default (an invented price on a public page is worse than a blank one).
var ErrPricingPromoNotFound = errors.New("pricing promo row not found")

// PricingRepository reads the platform-wide pricing singleton.
//
// ⚠️ GLOBAL TABLE — pricing_promo has no business_id column and RLS is disabled
// (relrowsecurity = false, zero policies). There is therefore no tenant predicate
// to apply and none is written: these prices are platform-wide and public. This
// is a READ of non-tenant data, so it is served without authentication, unlike
// the platform tables whose WRITES are gated to the DEVELOPER role.
type PricingRepository struct {
	db *DB
}

// NewPricingRepository creates a pricing repository.
func NewPricingRepository(db *DB) *PricingRepository {
	return &PricingRepository{db: db}
}

// GetPromo returns the singleton row.
//
// Every nullable column is scanned into a nullable Go type. Today's row happens
// to populate all of them, so scanning into plain int/string would pass in
// testing and then misreport a NULL as 0 (a free plan) the first time someone
// clears a price.
func (r *PricingRepository) GetPromo(ctx context.Context) (*model.PricingPromo, error) {
	const q = `
		SELECT active, starts_on, ends_on, label,
		       monthly_promo_mmk, monthly_standard_mmk,
		       annual_promo_mmk, annual_standard_mmk, updated_at
		FROM pricing_promo
		WHERE id = true`

	p := &model.PricingPromo{}
	var (
		// ⚠️ The window columns are scanned as DATES, not strings. lib/pq returns a
		// `date` column as a time.Time at midnight UTC, and database/sql will
		// stringify that as RFC3339 if the destination is a string — turning
		// "2026-08-23" into "2026-08-23T00:00:00Z". That form is not a date: the
		// model's window gate validates YYYY-MM-DD, so the RFC3339 form would be
		// rejected as a malformed window and a genuine promo would silently never
		// run. Scan the type the column actually has, then render it explicitly.
		startsOn, endsOn              sql.NullTime
		label                         sql.NullString
		monthlyPromo, monthlyStandard sql.NullInt64
		annualPromo, annualStandard   sql.NullInt64
	)

	err := r.db.QueryRowContext(ctx, q).Scan(
		&p.Active, &startsOn, &endsOn, &label,
		&monthlyPromo, &monthlyStandard,
		&annualPromo, &annualStandard, &p.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPricingPromoNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load pricing promo: %w", err)
	}

	p.StartsOn = dateOnly(scanNullableTime(startsOn))
	p.EndsOn = dateOnly(scanNullableTime(endsOn))
	p.Label = scanNullableString(label)
	p.MonthlyPromoMMK = scanNullableInt(monthlyPromo)
	p.MonthlyStandardMMK = scanNullableInt(monthlyStandard)
	p.AnnualPromoMMK = scanNullableInt(annualPromo)
	p.AnnualStandardMMK = scanNullableInt(annualStandard)
	return p, nil
}

// dateOnly renders a DATE column as its calendar day (YYYY-MM-DD) in UTC. The
// driver has already parsed the stored date to midnight UTC, so the UTC calendar
// day IS the stored day — no timezone shift is introduced here. The operator
// calendar is applied later, in model.PromoLiveAt, when "today" is compared.
func dateOnly(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format("2006-01-02")
	return &s
}
