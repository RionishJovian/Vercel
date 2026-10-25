package handler

import (
	"errors"
	"math"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/database"
	"mega-magic/internal/model"
)

// PricingHandler serves the PUBLIC price list.
//
// pricing_promo is a platform-global table (no business_id, RLS disabled) and
// these figures ARE the published price list — the entire point is that an
// anonymous visitor can read them, so the route is registered WITHOUT auth, like
// /version. No tenant row and no secret is exposed: four prices, a promo window,
// and the per-tier device quotas.
type PricingHandler struct {
	repo                    *database.PricingRepository
	IndividualPassesEnabled bool
}

// NewPricingHandler wires the pricing repository.
func NewPricingHandler(db *database.DB) *PricingHandler {
	return &PricingHandler{repo: database.NewPricingRepository(db)}
}

func (h *PricingHandler) WithIndividualPasses(enabled bool) *PricingHandler {
	h.IndividualPassesEnabled = enabled
	return h
}

// Get returns the published prices, the DERIVED promo state, and the tier table.
func (h *PricingHandler) Get(c *gin.Context) {
	now := time.Now().UTC()

	promo, err := h.repo.GetPromo(c.Request.Context())
	if err != nil {
		if errors.Is(err, database.ErrPricingPromoNotFound) {
			// No published price list. Say so. Substituting a default here would
			// put an invented price in front of customers.
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "the price list is not published yet",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	live, reason := promo.PromoLiveAt(now)

	tiers := tierList()
	policy := gin.H{"mode": "legacy"}
	if h.IndividualPassesEnabled {
		tiers = []gin.H{}
		policy = gin.H{"mode": "individual_pass", "authorized_logins": 1, "routers": 1, "sites": 1, "capacity_scope": "business", "activation": "operator_approval", "validity": "calendar_month_or_year", "same_features": true, "unlimited_vouchers": true, "unlimited_ai": true, "unlimited_jellyfin": true}
	}
	c.JSON(http.StatusOK, gin.H{
		"account_policy": policy,
		"currency":       "MMK",
		"timezone":       model.AppTimezone.String(),
		"generated_at":   now.Format(time.RFC3339),
		"plans":          plansView(promo),
		"promo":          promoView(promo, live, reason),
		"tiers":          tiers,
	})
}

// plansView exposes the standard prices plus the annual-vs-monthly arithmetic.
//
// The saving is DERIVED on every read, never stored: a year paid monthly costs
// 12 x the monthly rate, so the difference is what the annual pass saves. It is
// emitted only when both figures exist — computing against a missing price would
// publish a fabricated number — and a negative result is passed through as-is,
// because an annual pass that costs MORE than paying monthly is a fact the page
// must not silently hide.
func plansView(p *model.PricingPromo) gin.H {
	monthly := gin.H{"standard_mmk": p.MonthlyStandardMMK}
	annual := gin.H{"standard_mmk": p.AnnualStandardMMK}

	if p.MonthlyStandardMMK != nil && p.AnnualStandardMMK != nil {
		twelveMonthly := 12 * *p.MonthlyStandardMMK
		annual["twelve_monthly_mmk"] = twelveMonthly
		annual["saving_vs_monthly_mmk"] = twelveMonthly - *p.AnnualStandardMMK
		if *p.MonthlyStandardMMK > 0 {
			months := float64(*p.AnnualStandardMMK) / float64(*p.MonthlyStandardMMK)
			annual["equivalent_months_of_monthly"] = math.Round(months*100) / 100
		}
	}
	return gin.H{"monthly": monthly, "annual": annual}
}

// promoView reports whether a promo may be shown as a discount, and why not.
//
// The stored promo prices are included so the state is auditable rather than
// hidden, but they are NEVER a price the page may display: `live` gates that, and
// the per-cadence flags show which half fails when it does. Today's migrated row
// fails on `monthly_is_discount`/`annual_is_discount` (the "promo" is priced
// ABOVE the standard), so the page shows standard rates only.
func promoView(p *model.PricingPromo, live bool, reason string) gin.H {
	out := gin.H{
		"live":                live,
		"active_flag":         p.Active,
		"label":               p.Label,
		"starts_on":           p.StartsOn,
		"ends_on":             p.EndsOn,
		"monthly_promo_mmk":   p.MonthlyPromoMMK,
		"annual_promo_mmk":    p.AnnualPromoMMK,
		"monthly_is_discount": p.MonthlyIsDiscount(),
		"annual_is_discount":  p.AnnualIsDiscount(),
	}
	if !live && reason != "" {
		out["reason"] = reason
	}
	return out
}

// tierList renders the ADVERTISED capacity tiers with their device quotas.
//
// PublicAccessTiers excludes the platform-operator tier (GLOBAL_ADMIN), so the
// public price list never offers a level no published price covers. The quota
// comes from database.TierDefaultsFor — the same function the tenant-provisioning
// path uses — so the page can never advertise a limit the product does not
// enforce.
func tierList() []gin.H {
	tiers := model.PublicAccessTiers()
	out := make([]gin.H, 0, len(tiers))
	for _, tier := range tiers {
		out = append(out, gin.H{
			"key":          tier,
			"label":        model.AccessTierLabel(tier),
			"device_quota": database.TierDefaultsFor(tier).DeviceQuotaLimit,
		})
	}
	return out
}
