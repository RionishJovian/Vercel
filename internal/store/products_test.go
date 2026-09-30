package store

import (
	"errors"
	"testing"
	"time"

	"mega-magic/internal/model"
)

func pint(v int) *int       { return &v }
func pstr(v string) *string { return &v }

func TestResolveProductUsesOnlyPublishedPrice(t *testing.T) {
	now := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	pricing := model.PricingPromo{MonthlyStandardMMK: pint(35000), AnnualStandardMMK: pint(350000)}
	got, err := ResolveProduct(ProductMonthly, 0, pricing, now)
	if err != nil || got.PriceMMK != 35000 || got.Days != 30 {
		t.Fatalf("monthly = %#v, %v", got, err)
	}
}

func TestResolveProductRejectsMissingAndUnknown(t *testing.T) {
	if _, err := ResolveProduct(ProductAnnual, 0, model.PricingPromo{}, time.Now()); !errors.Is(err, ErrPriceMissing) {
		t.Fatalf("missing price error = %v", err)
	}
	if _, err := ResolveProduct("browser-invented", 0, model.PricingPromo{}, time.Now()); !errors.Is(err, ErrUnknownProduct) {
		t.Fatalf("unknown product error = %v", err)
	}
}

func TestResolveProductPromoAndCoins(t *testing.T) {
	now := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	pricing := model.PricingPromo{Active: true, StartsOn: pstr("2026-09-01"), EndsOn: pstr("2026-10-01"), MonthlyStandardMMK: pint(35000), MonthlyPromoMMK: pint(30000), AnnualStandardMMK: pint(350000), AnnualPromoMMK: pint(300000)}
	annual, err := ResolveProduct(ProductAnnual, 0, pricing, now)
	if err != nil || annual.PriceMMK != 300000 {
		t.Fatalf("annual = %#v, %v", annual, err)
	}
	coins, err := ResolveProduct(ProductMagicCoins, 25, pricing, now)
	if err != nil || coins.PriceMMK != 25000 || coins.CoinCount != 25 {
		t.Fatalf("coins = %#v, %v", coins, err)
	}
	if _, err := ResolveProduct(ProductMagicCoins, 0, pricing, now); !errors.Is(err, ErrInvalidCoins) {
		t.Fatalf("zero coins error = %v", err)
	}
}
