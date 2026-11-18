package database

import (
	"testing"
	"time"
)

func TestWebFigCoveredUntil(t *testing.T) {
	d := func(s string) time.Time { v, _ := time.Parse("2006-01-02 15:04", s); return v }
	m := []string{"monthly", "monthly", "monthly"}
	cases := []struct {
		name    string
		decided []time.Time
		periods []string
		now     time.Time
		want    bool
	}{
		{"nothing approved", nil, nil, d("2026-11-18 10:00"), false},
		{"inside the month", []time.Time{d("2026-11-18 10:00")}, m[:1], d("2026-12-10 00:00"), true},
		{"last paid day, late evening", []time.Time{d("2026-11-18 10:00")}, m[:1], d("2026-12-18 23:59"), true},
		{"stops the next day", []time.Time{d("2026-11-18 10:00")}, m[:1], d("2026-12-19 00:00"), false},
		{"early renewal extends, not overlaps", []time.Time{d("2026-11-18 10:00"), d("2026-12-01 10:00")}, m[:2], d("2027-01-10 00:00"), true},
		{"early renewal still ends after two months", []time.Time{d("2026-11-18 10:00"), d("2026-12-01 10:00")}, m[:2], d("2027-01-19 00:00"), false},
		{"late renewal restarts from approval", []time.Time{d("2026-11-18 10:00"), d("2027-03-01 10:00")}, m[:2], d("2027-03-20 00:00"), true},
		{"annual", []time.Time{d("2026-11-18 10:00")}, []string{"annual"}, d("2027-06-01 00:00"), true},
		{"one time never ends", []time.Time{d("2026-11-18 10:00")}, []string{"one_time"}, d("2035-01-01 00:00"), true},
	}
	for _, c := range cases {
		if got := webFigCoveredUntil(c.decided, c.periods, c.now); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
