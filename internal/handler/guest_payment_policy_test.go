package handler

import (
	"testing"
	"time"

	"mega-magic/internal/model"
)

func TestBankTransferCooldownBoundaries(t *testing.T) {
	end := time.Date(2026, 10, 1, 23, 59, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, status    string
		now             time.Time
		resume, blocked bool
	}{
		{"resume live minute", "pending", end.Add(-time.Second), true, false},
		{"expiry begins cooldown", "pending", end, false, true},
		{"six hours minus a nanosecond", "pending", end.Add(6*time.Hour - time.Nanosecond), false, true},
		{"six hours exact across midnight", "pending", end.Add(6 * time.Hour), false, false},
		{"rejected still cooling", "rejected", end.Add(time.Hour), false, true},
		{"rejected retry", "rejected", end.Add(6 * time.Hour), false, false},
		{"never remove a paid session", "approved", end.Add(7 * time.Hour), false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resume, err := guestGraceRetry(&model.GuestPurchaseRequest{Status: tc.status, GraceExpiresAt: &end}, tc.now)
			if resume != tc.resume || (err != nil) != tc.blocked {
				t.Fatalf("resume=%v err=%v", resume, err)
			}
		})
	}
	if resume, err := guestGraceRetry(nil, end); resume || err != nil {
		t.Fatal("new client blocked")
	}
	if _, err := guestGraceRetry(&model.GuestPurchaseRequest{Status: "pending"}, end); err == nil {
		t.Fatal("missing deadline accepted")
	}
}
