package model

import (
	"regexp"
	"time"
)

// ── Operator calendar ────────────────────────────────────────────────────────
//
// AppTimezone is the business calendar every day-boundary decision is made in
// (promo windows, daily closes, "today" in reports). The old Mikromagic app
// gated its pricing promo on Asia/Yangon, so the same zone is kept here: gating
// on UTC would open or close a promo window up to a day early. A fixed offset is
// used instead of LoadLocation("Asia/Yangon") so the deployed binary needs no
// system tzdata (the VPS host has it, a scratch container may not).
var AppTimezone = time.FixedZone("Asia/Yangon", 6*60*60+30*60)

// AppDay renders t as the operator's calendar day (YYYY-MM-DD). Comparing that
// form as a string is chronological, which is how windows are compared below.
func AppDay(t time.Time) string {
	return t.In(AppTimezone).Format("2006-01-02")
}

// ── Access (capacity) tiers ──────────────────────────────────────────────────
//
// The closed set of tenants' capacity tiers. The database enforces the same set
// with a CHECK constraint on businesses.access_tier, and the per-tier device
// quota lives in database.TierDefaultsFor — deliberately NOT duplicated here, so
// the quota has exactly one source of truth.
const (
	AccessTierSmallWISP     = "SMALL_WISP"
	AccessTierEnterpriseISP = "ENTERPRISE_ISP"
	AccessTierGlobalAdmin   = "GLOBAL_ADMIN"
)

// AccessTiers is the ordered, closed set of capacity tiers, smallest fleet
// first. Order matters: the pricing page renders them in this sequence.
var AccessTiers = []string{
	AccessTierSmallWISP,
	AccessTierEnterpriseISP,
	AccessTierGlobalAdmin,
}

// AccessTierLabel is the human-readable name for a tier. An unknown tier is
// returned unchanged rather than blanked, so an unexpected value is visible in
// the UI instead of looking like a missing tier.
func AccessTierLabel(tier string) string {
	switch tier {
	case AccessTierSmallWISP:
		return "Small WISP"
	case AccessTierEnterpriseISP:
		return "Enterprise ISP"
	case AccessTierGlobalAdmin:
		return "Global admin"
	default:
		return tier
	}
}

// AccessTierIsPublic reports whether a tier may be ADVERTISED publicly.
//
// GLOBAL_ADMIN is deliberately NOT public: it is the platform operator's own
// level, assigned from the platform console rather than sold, so listing it on a
// pricing page invites customers to ask for a tier no published price covers. It
// stays in AccessTiers because tenant creation and the platform console still
// accept it — this filters the ADVERTISEMENT, not the product's vocabulary.
func AccessTierIsPublic(tier string) bool {
	return tier != AccessTierGlobalAdmin
}

// PublicAccessTiers is the advertised subset of AccessTiers, in the same order,
// so the pricing page cannot drift from the tier set the product actually
// accepts (and reacts correctly if a tier is ever added or made public).
func PublicAccessTiers() []string {
	out := make([]string, 0, len(AccessTiers))
	for _, tier := range AccessTiers {
		if AccessTierIsPublic(tier) {
			out = append(out, tier)
		}
	}
	return out
}

// ── Pricing promo ────────────────────────────────────────────────────────────

// dateOnly matches the YYYY-MM-DD form the window columns are stored in. A value
// that does not match is treated as "no window", never guessed at.
var dateOnly = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// Reasons a promo is not shown. Returned verbatim by the API so an operator can
// see why a promo they switched on is not appearing.
const (
	PromoReasonInactive    = "the promo kill switch is off"
	PromoReasonNoWindow    = "the promo window is not set"
	PromoReasonNotStarted  = "the promo window has not started"
	PromoReasonEnded       = "the promo window has ended"
	PromoReasonIncomplete  = "the promo or standard price is missing"
	PromoReasonNotDiscount = "the promo price is not below the standard price"
)

// PricingPromo is the platform-wide pricing singleton (pricing_promo).
//
// ⚠️ Every price and date is a POINTER because the columns are NULLABLE in the
// database, and "unset" must stay distinguishable from zero. A public pricing
// page that read a NULL price as 0 would advertise a free plan.
//
// ⚠️ GLOBAL TABLE: pricing_promo has no business_id and RLS is disabled, so it is
// platform-wide by design and never tenant-scoped.
type PricingPromo struct {
	Active bool `json:"active"`

	StartsOn *string `json:"starts_on"` // YYYY-MM-DD, operator calendar
	EndsOn   *string `json:"ends_on"`
	Label    *string `json:"label"`

	MonthlyPromoMMK    *int `json:"monthly_promo_mmk"`
	MonthlyStandardMMK *int `json:"monthly_standard_mmk"`
	AnnualPromoMMK     *int `json:"annual_promo_mmk"`
	AnnualStandardMMK  *int `json:"annual_standard_mmk"`

	UpdatedAt time.Time `json:"updated_at"`
}

// PromoLiveAt reports whether the promo may be SHOWN AS A DISCOUNT at now, and
// why not when it may not.
//
// Four independent gates, ALL required:
//  1. the `active` kill switch is on;
//  2. the window is set and now (operator calendar day) falls inside it
//     INCLUSIVE of both ends;
//  3. BOTH cadences have a promo price and a standard price;
//  4. every promo price is strictly BELOW its standard price.
//
// Gate 4 is not defensive padding — it is the whole point. The migrated row reads
// label "Grand opening gift — 30% off Monthly & Annual" with
// monthly_promo_mmk 66,500 against monthly_standard_mmk 35,000: a "promo" that is
// 1.9x the list price. A kill switch and a date window cannot catch that, so a
// page gated on those two alone would advertise a discount that OVERCHARGES.
// The old app's own migration carried the rule as a comment ("never shows a
// 'promo' higher than the list price") but applied it by hand at write time, so
// nothing enforced it against data already in the table. Deriving it at read time
// means the guard holds for rows written by any path — including a future manual
// edit — exactly as a stored verdict can never learn a new fact.
func (p PricingPromo) PromoLiveAt(now time.Time) (bool, string) {
	if !p.Active {
		return false, PromoReasonInactive
	}

	// Gate 2 — the window. Validate the stored form before comparing.
	if p.StartsOn == nil || p.EndsOn == nil ||
		!dateOnly.MatchString(*p.StartsOn) || !dateOnly.MatchString(*p.EndsOn) {
		return false, PromoReasonNoWindow
	}
	today := AppDay(now)
	if today < *p.StartsOn {
		return false, PromoReasonNotStarted
	}
	if today > *p.EndsOn {
		return false, PromoReasonEnded
	}

	// Gate 3 — every figure the offer depends on must exist.
	if p.MonthlyStandardMMK == nil || p.MonthlyPromoMMK == nil ||
		p.AnnualStandardMMK == nil || p.AnnualPromoMMK == nil {
		return false, PromoReasonIncomplete
	}

	// Gate 4 — a "promo" at or above the list price is not a discount. Both
	// cadences must qualify, because the offer is advertised for both.
	if *p.MonthlyPromoMMK >= *p.MonthlyStandardMMK ||
		*p.AnnualPromoMMK >= *p.AnnualStandardMMK {
		return false, PromoReasonNotDiscount
	}

	return true, ""
}

// MonthlyIsDiscount / AnnualIsDiscount expose gate 4 per cadence, so a
// half-broken promo is visible in the API response instead of being collapsed
// into a single "not live" that hides which half is wrong.
func (p PricingPromo) MonthlyIsDiscount() bool {
	return p.MonthlyStandardMMK != nil && p.MonthlyPromoMMK != nil &&
		*p.MonthlyPromoMMK < *p.MonthlyStandardMMK
}

// AnnualIsDiscount reports whether the annual promo undercuts the annual standard.
func (p PricingPromo) AnnualIsDiscount() bool {
	return p.AnnualStandardMMK != nil && p.AnnualPromoMMK != nil &&
		*p.AnnualPromoMMK < *p.AnnualStandardMMK
}
