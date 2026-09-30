// Package store owns the server-authoritative catalogue rules for platform
// purchases. Browser-supplied labels and prices are never trusted.
package store

import (
	"errors"
	"fmt"
	"time"

	"mega-magic/internal/model"
)

const (
	ProductMonthly    = "monthly"
	ProductAnnual     = "annual"
	ProductMagicCoins = "magic_coins"
)

var (
	ErrUnknownProduct = errors.New("store: unknown product")
	ErrPriceMissing   = errors.New("store: published price is missing")
	ErrInvalidCoins   = errors.New("store: coin quantity must be positive")
)

type Product struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	PriceMMK  int    `json:"price_mmk"`
	CoinCount int    `json:"coin_count,omitempty"`
	Days      int    `json:"entitlement_days,omitempty"`
}

// ResolveProduct derives the payable product exclusively from the published
// platform price row. One Magic Coin is worth 1,000 MMK.
func ResolveProduct(key string, coinCount int, pricing model.PricingPromo, now time.Time) (Product, error) {
	live, _ := pricing.PromoLiveAt(now)
	price := func(standard, promo *int) (int, error) {
		chosen := standard
		if live {
			chosen = promo
		}
		if chosen == nil || *chosen <= 0 {
			return 0, ErrPriceMissing
		}
		return *chosen, nil
	}
	switch key {
	case ProductMonthly:
		p, err := price(pricing.MonthlyStandardMMK, pricing.MonthlyPromoMMK)
		return Product{Key: key, Label: "Monthly pass", PriceMMK: p, Days: 30}, err
	case ProductAnnual:
		p, err := price(pricing.AnnualStandardMMK, pricing.AnnualPromoMMK)
		return Product{Key: key, Label: "Annual pass", PriceMMK: p, Days: 365}, err
	case ProductMagicCoins:
		if coinCount <= 0 || coinCount > 1_000_000 {
			return Product{}, ErrInvalidCoins
		}
		return Product{Key: key, Label: fmt.Sprintf("%d Magic Coins", coinCount), PriceMMK: coinCount * 1000, CoinCount: coinCount}, nil
	default:
		return Product{}, fmt.Errorf("%w: %q", ErrUnknownProduct, key)
	}
}
