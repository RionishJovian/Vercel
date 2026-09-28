package database

import (
	"context"
	"regexp"
	"testing"
	"time"
)

// dateForm matches the bare YYYY-MM-DD shape the promo window must reach the API
// in — the only shape model.PromoLiveAt accepts.
var dateForm = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// TestPricingPromoShapeFromLiveDB pins the DRIVER BOUNDARY.
//
// A `date` column arrives from lib/pq as a time.Time at midnight UTC, and
// database/sql silently stringifies that as RFC3339 ("2026-08-23T00:00:00Z") when
// the scan destination is a string. The model validates YYYY-MM-DD, so that form
// is rejected as a malformed window and a genuine promo would never run — a
// failure invisible to a unit test built from hand-written date strings, because
// the unit test never crosses the driver. This one does.
//
//	MM_TEST_DSN='host=127.0.0.1 port=5433 user=... password=... dbname=mega_magic sslmode=disable' \
//	    go test ./internal/database/ -run PricingPromo -v
func TestPricingPromoShapeFromLiveDB(t *testing.T) {
	db := openTestDB(t)
	repo := NewPricingRepository(db)

	promo, err := repo.GetPromo(context.Background())
	if err != nil {
		t.Fatalf("GetPromo: %v", err)
	}

	if promo.StartsOn == nil || promo.EndsOn == nil {
		t.Fatalf("expected a promo window, got starts_on=%v ends_on=%v", promo.StartsOn, promo.EndsOn)
	}
	for _, c := range []struct {
		name string
		v    *string
	}{{"starts_on", promo.StartsOn}, {"ends_on", promo.EndsOn}} {
		if !dateForm.MatchString(*c.v) {
			t.Fatalf("%s reached the API as %q — it must be a bare YYYY-MM-DD or "+
				"model.PromoLiveAt rejects it as a malformed window", c.name, *c.v)
		}
	}

	// A NULL price must stay NULL rather than becoming 0: the page renders 0 as a
	// real figure, so a NULL read as zero would advertise a free plan.
	if promo.MonthlyStandardMMK == nil || promo.AnnualStandardMMK == nil {
		t.Fatal("a standard price is NULL — the page would show a dash where a price belongs")
	}
	if *promo.MonthlyStandardMMK == 0 || *promo.AnnualStandardMMK == 0 {
		t.Fatalf("standard prices read as zero (%d / %d) — NULL must never become 0",
			*promo.MonthlyStandardMMK, *promo.AnnualStandardMMK)
	}
	if promo.UpdatedAt.IsZero() {
		t.Fatal("updated_at did not scan")
	}

	t.Logf("window %s..%s · monthly %d · annual %d · active=%v",
		*promo.StartsOn, *promo.EndsOn, *promo.MonthlyStandardMMK, *promo.AnnualStandardMMK, promo.Active)
}

// TestMigratedPromoRefusedFromLiveDB runs the discount guard against the REAL row.
// It is written so it keeps passing if the promo is legitimately fixed later, while
// still failing the moment a promo priced at or above the standard goes live.
func TestMigratedPromoRefusedFromLiveDB(t *testing.T) {
	db := openTestDB(t)
	repo := NewPricingRepository(db)

	promo, err := repo.GetPromo(context.Background())
	if err != nil {
		t.Fatalf("GetPromo: %v", err)
	}

	live, reason := promo.PromoLiveAt(time.Now())

	aboveStandard := promo.MonthlyPromoMMK != nil && promo.MonthlyStandardMMK != nil &&
		*promo.MonthlyPromoMMK >= *promo.MonthlyStandardMMK
	if aboveStandard && live {
		t.Fatalf("a promo priced at or above the standard went LIVE (monthly %d vs standard %d)",
			*promo.MonthlyPromoMMK, *promo.MonthlyStandardMMK)
	}

	t.Logf("live=%v reason=%q · monthly_is_discount=%v annual_is_discount=%v",
		live, reason, promo.MonthlyIsDiscount(), promo.AnnualIsDiscount())
}
