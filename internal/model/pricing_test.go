package model

import (
	"testing"
	"time"
)

func iptr(n int) *int { return &n }
func sptr(s string) *string {
	return &s
}

// migrated returns the pricing_promo row as it exists in production after the
// Mikromagic migration: label claims 30% off, the window ran 23 Aug - 24 Sep
// 2026, the kill switch is off, and — the defect this guards — the "promo" prices
// (66,500 / 731,500) are ABOVE the standard prices (35,000 / 350,000).
func migrated() PricingPromo {
	return PricingPromo{
		Active:             false,
		StartsOn:           sptr("2026-08-23"),
		EndsOn:             sptr("2026-09-24"),
		Label:              sptr("Grand opening gift — 30% off Monthly & Annual"),
		MonthlyPromoMMK:    iptr(66500),
		MonthlyStandardMMK: iptr(35000),
		AnnualPromoMMK:     iptr(731500),
		AnnualStandardMMK:  iptr(350000),
	}
}

// healthy returns a coherent 30%-off promo built on the standard prices the
// owner confirmed (35,000 / year 350,000).
func healthy() PricingPromo {
	return PricingPromo{
		Active:             true,
		StartsOn:           sptr("2026-08-23"),
		EndsOn:             sptr("2026-09-24"),
		Label:              sptr("Grand opening gift — 30% off Monthly & Annual"),
		MonthlyPromoMMK:    iptr(24500),
		MonthlyStandardMMK: iptr(35000),
		AnnualPromoMMK:     iptr(245000),
		AnnualStandardMMK:  iptr(350000),
	}
}

func at(t *testing.T, iso string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		t.Fatalf("bad test timestamp %q: %v", iso, err)
	}
	return ts
}

func TestPromoLiveAt(t *testing.T) {
	tests := []struct {
		name       string
		promo      PricingPromo
		now        string
		wantLive   bool
		wantReason string
	}{
		{
			// THE REGRESSION THAT MATTERS: even with the kill switch ON and the
			// date inside the window, a promo priced above the standard must not
			// go live. Flip this assert and the corrupted migrated row would be
			// advertised to customers as a discount.
			name:       "switch on, inside window, but promo is priced ABOVE standard",
			promo:      func() PricingPromo { p := migrated(); p.Active = true; return p }(),
			now:        "2026-09-01T06:00:00Z",
			wantLive:   false,
			wantReason: PromoReasonNotDiscount,
		},
		{
			name:       "kill switch off",
			promo:      migrated(),
			now:        "2026-09-01T06:00:00Z",
			wantLive:   false,
			wantReason: PromoReasonInactive,
		},
		{
			name:       "window in the past",
			promo:      healthy(),
			now:        "2026-09-25T06:00:00Z",
			wantLive:   false,
			wantReason: PromoReasonEnded,
		},
		{
			name:       "window in the future",
			promo:      healthy(),
			now:        "2026-08-01T06:00:00Z",
			wantLive:   false,
			wantReason: PromoReasonNotStarted,
		},
		{
			name:     "inside the window, genuine discount",
			promo:    healthy(),
			now:      "2026-09-01T06:00:00Z",
			wantLive: true,
		},
		{
			// Inclusive window: the first day counts.
			name:     "first day of the window is live",
			promo:    healthy(),
			now:      "2026-08-23T06:00:00Z",
			wantLive: true,
		},
		{
			// Inclusive window: the last day counts...
			name:     "last day of the window is live",
			promo:    healthy(),
			now:      "2026-09-24T13:00:00Z",
			wantLive: true,
		},
		{
			// ...but not a minute past it ON THE OPERATOR CALENDAR. 20:00Z on
			// 24 Sep is already 02:30 on 25 Sep in Asia/Yangon (UTC+06:30), so a
			// UTC-gated window would wrongly keep selling for six and a half
			// hours. This case fails if AppTimezone is ever swapped for UTC.
			name:       "operator calendar rolls over, UTC has not",
			promo:      healthy(),
			now:        "2026-09-24T20:00:00Z",
			wantLive:   false,
			wantReason: PromoReasonEnded,
		},
		{
			name:       "window not set",
			promo:      func() PricingPromo { p := healthy(); p.StartsOn = nil; return p }(),
			now:        "2026-09-01T06:00:00Z",
			wantLive:   false,
			wantReason: PromoReasonNoWindow,
		},
		{
			name:       "window malformed is not guessed at",
			promo:      func() PricingPromo { p := healthy(); p.EndsOn = sptr("24 Sep 2026"); return p }(),
			now:        "2026-09-01T06:00:00Z",
			wantLive:   false,
			wantReason: PromoReasonNoWindow,
		},
		{
			name:       "a missing promo price is incomplete, never free",
			promo:      func() PricingPromo { p := healthy(); p.MonthlyPromoMMK = nil; return p }(),
			now:        "2026-09-01T06:00:00Z",
			wantLive:   false,
			wantReason: PromoReasonIncomplete,
		},
		{
			name:       "a missing standard price is incomplete",
			promo:      func() PricingPromo { p := healthy(); p.AnnualStandardMMK = nil; return p }(),
			now:        "2026-09-01T06:00:00Z",
			wantLive:   false,
			wantReason: PromoReasonIncomplete,
		},
		{
			name:       "promo EQUAL to standard is not a discount",
			promo:      func() PricingPromo { p := healthy(); p.MonthlyPromoMMK = iptr(35000); return p }(),
			now:        "2026-09-01T06:00:00Z",
			wantLive:   false,
			wantReason: PromoReasonNotDiscount,
		},
		{
			// Half a promo: the monthly side undercuts but the annual side does
			// not. The offer is advertised for both, so it must not go live.
			name: "only one cadence is a real discount",
			promo: func() PricingPromo {
				p := healthy()
				p.AnnualPromoMMK = iptr(999999)
				return p
			}(),
			now:        "2026-09-01T06:00:00Z",
			wantLive:   false,
			wantReason: PromoReasonNotDiscount,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			live, reason := tc.promo.PromoLiveAt(at(t, tc.now))
			if live != tc.wantLive {
				t.Fatalf("PromoLiveAt(%s) live = %v, want %v (reason %q)",
					tc.now, live, tc.wantLive, reason)
			}
			if !tc.wantLive && reason != tc.wantReason {
				t.Fatalf("PromoLiveAt(%s) reason = %q, want %q", tc.now, reason, tc.wantReason)
			}
			if tc.wantLive && reason != "" {
				t.Fatalf("live promo carried a refusal reason %q", reason)
			}
		})
	}
}

// TestMigratedPromoIsNeverLive pins the production row: whatever the kill switch
// and the clock say, the migrated row can never be advertised. If a future edit
// "fixes" the standard prices down to meet it, or flips the switch, this still
// holds while the promo columns stay above the standard.
func TestMigratedPromoIsNeverLive(t *testing.T) {
	p := migrated()
	for _, live := range []bool{false, true} {
		p.Active = live
		for day := 20; day <= 30; day++ {
			now := time.Date(2026, time.August, day, 6, 0, 0, 0, time.UTC)
			if got, reason := p.PromoLiveAt(now); got {
				t.Fatalf("migrated promo went live on 2026-08-%02d (active=%v, reason=%q)", day, live, reason)
			}
		}
	}
}

// TestPerCadenceDiscountFlags proves the flags report the two halves separately,
// so a half-broken promo is visible instead of collapsing to "not live".
func TestPerCadenceDiscountFlags(t *testing.T) {
	half := healthy()
	half.AnnualPromoMMK = iptr(999999)

	if !half.MonthlyIsDiscount() {
		t.Fatal("monthly promo 24,500 < standard 35,000 should read as a discount")
	}
	if half.AnnualIsDiscount() {
		t.Fatal("annual promo 999,999 > standard 350,000 must not read as a discount")
	}

	live, reason := half.PromoLiveAt(at(t, "2026-09-01T06:00:00Z"))
	if live {
		t.Fatalf("half-broken promo went live (reason %q)", reason)
	}

	m := migrated()
	if m.MonthlyIsDiscount() || m.AnnualIsDiscount() {
		t.Fatal("the migrated row's promo prices exceed its standards and must not read as discounts")
	}
}

// TestGlobalAdminIsNotAdvertised pins the public/private tier split. GLOBAL_ADMIN
// is the platform operator's own level (assigned from the platform console, not
// sold), so it must never appear on a price list — while REMAINING a real tier
// the product accepts, so nothing downstream loses the vocabulary.
func TestGlobalAdminIsNotAdvertised(t *testing.T) {
	pub := PublicAccessTiers()

	want := []string{AccessTierSmallWISP, AccessTierEnterpriseISP}
	if len(pub) != len(want) {
		t.Fatalf("published tiers = %v, want %v", pub, want)
	}
	for i := range want {
		if pub[i] != want[i] {
			t.Fatalf("published tiers = %v, want %v (order follows AccessTiers)", pub, want)
		}
		if !AccessTierIsPublic(pub[i]) {
			t.Fatalf("%s is advertised but reports itself non-public", pub[i])
		}
	}

	if AccessTierIsPublic(AccessTierGlobalAdmin) {
		t.Fatal("GLOBAL_ADMIN reports itself public — it is the operator's own tier")
	}

	// The tier must survive in the full set: tenant creation and the platform
	// console still accept it, and platform_handler.validTiers derives from here.
	var present bool
	for _, tier := range AccessTiers {
		if tier == AccessTierGlobalAdmin {
			present = true
		}
	}
	if !present {
		t.Fatal("GLOBAL_ADMIN was dropped from AccessTiers — that filters the vocabulary, not the advertisement")
	}
}

// TestAppDayUsesOperatorTimezone pins the day boundary the windows are gated on.
func TestAppDayUsesOperatorTimezone(t *testing.T) {
	tests := []struct {
		utc  string
		want string
	}{
		{"2026-09-24T18:29:00Z", "2026-09-25"}, // 00:59 next day in Yangon
		{"2026-09-24T17:29:00Z", "2026-09-24"}, // 23:59 same day in Yangon
		{"2026-08-22T20:00:00Z", "2026-08-23"},
	}
	for _, tc := range tests {
		if got := AppDay(at(t, tc.utc)); got != tc.want {
			t.Fatalf("AppDay(%s) = %s, want %s", tc.utc, got, tc.want)
		}
	}
}
