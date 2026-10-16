package handler

import (
	"strings"
	"testing"

	"mega-magic/internal/model"
)

func validPlatformPricing() model.PricingPromo {
	start, end, label := "2026-11-01", "2026-11-30", "Launch offer"
	monthlyPromo, monthlyStandard := 25000, 35000
	annualPromo, annualStandard := 250000, 350000
	return model.PricingPromo{Active: true, StartsOn: &start, EndsOn: &end, Label: &label,
		MonthlyPromoMMK: &monthlyPromo, MonthlyStandardMMK: &monthlyStandard,
		AnnualPromoMMK: &annualPromo, AnnualStandardMMK: &annualStandard}
}

func TestValidatePricingPromoAcceptsCompleteOffer(t *testing.T) {
	if err := validatePricingPromo(ptrPricing(validPlatformPricing())); err != nil {
		t.Fatalf("valid pricing rejected: %v", err)
	}
}

func TestValidatePricingPromoRejectsPromoAboveStandard(t *testing.T) {
	p := validPlatformPricing()
	*p.MonthlyPromoMMK = *p.MonthlyStandardMMK + 1
	if err := validatePricingPromo(&p); err == nil || !strings.Contains(err.Error(), "monthly_promo_mmk") {
		t.Fatalf("expected monthly discount validation error, got %v", err)
	}
}

func TestValidatePricingPromoRejectsActiveOfferWithoutDates(t *testing.T) {
	p := validPlatformPricing()
	p.StartsOn, p.EndsOn = nil, nil
	if err := validatePricingPromo(&p); err == nil || !strings.Contains(err.Error(), "active promotion") {
		t.Fatalf("expected active-window validation error, got %v", err)
	}
}

func ptrPricing(p model.PricingPromo) *model.PricingPromo { return &p }
