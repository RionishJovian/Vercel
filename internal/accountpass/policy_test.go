package accountpass

import (
	"testing"
	"time"
)

func TestCalendarTerms(t *testing.T) {
	for _, tc := range []struct{ start, plan, want string }{
		{"2026-01-31T07:30:00Z", "monthly", "2026-02-28T07:30:00Z"},
		{"2028-01-31T07:30:00Z", "monthly", "2028-02-29T07:30:00Z"},
		{"2028-02-29T01:00:00Z", "annual", "2029-02-28T01:00:00Z"},
		{"2026-12-31T18:00:00Z", "monthly", "2027-01-31T18:00:00Z"},
	} {
		a, _ := time.Parse(time.RFC3339, tc.start)
		got, e := CalendarExpiry(a, tc.plan)
		if e != nil || got.Format(time.RFC3339) != tc.want {
			t.Fatalf("%+v got %s %v", tc, got, e)
		}
	}
	if _, e := CalendarExpiry(time.Now(), "sovereign"); e == nil {
		t.Fatal("legacy plan accepted")
	}
}
func TestExactExpiryTransitionAndExemption(t *testing.T) {
	now := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	past, future := now.Add(-time.Hour), now.Add(time.Hour)
	for _, tc := range []struct {
		name                            string
		operator                        bool
		start, end, revoked, transition *time.Time
		want                            bool
		state                           string
	}{
		{"paid", false, &past, &future, nil, nil, true, "active"}, {"exact expiry", false, &past, &now, nil, nil, false, "expired"}, {"transition", false, nil, nil, nil, &future, true, "transition"}, {"transition boundary", false, nil, nil, nil, &now, false, "pass_required"}, {"operator", true, nil, nil, nil, nil, true, "platform_exempt"}, {"revoked", false, &past, &future, &now, nil, false, "revoked"},
		{"revoked during transition", false, &past, &future, &now, &future, false, "revoked"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := Evaluate(now, true, tc.operator, tc.start, tc.end, tc.revoked, tc.transition)
			if s.Access != tc.want || s.State != tc.state || s.RouterLimit != 1 || !s.UnlimitedAI {
				t.Fatal(s)
			}
		})
	}
}
func TestRecoveryCannotWriteManagement(t *testing.T) {
	if !RecoveryRoute("POST", "/api/v1/me/app-pass/renewals") || !RecoveryRoute("GET", "/api/v1/me") {
		t.Fatal("renewal/signin recovery blocked")
	}
	for _, p := range []string{"/api/v1/routers", "/api/v1/sites", "/api/v1/me/app-pass/renewals/other", "/api/v1/store/purchases"} {
		if RecoveryRoute("POST", p) {
			t.Fatal(p)
		}
	}
}
