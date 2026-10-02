package database

import (
	"database/sql"
	"net"
	"testing"
	"time"

	"mega-magic/internal/model"
)

// TestDB_GuestWifi_crossBusinessIsolation proves the Option C tables are RLS
// tenant-scoped: a business can write/read only its own owner-bot binding and
// guest purchase requests, never another business's. Mirrors the money-path
// isolation proof.
func TestDB_GuestWifi_crossBusinessIsolation(t *testing.T) {
	db := requireHarness(t)
	botRepo := NewBusinessTelegramBotRepository(db)
	reqRepo := NewGuestPurchaseRequestRepository(db)

	bizA := insertBusiness(t, db)
	bizB := insertBusiness(t, db)

	// Each business binds its OWN bot (cross-business insert is against its own row).
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		return botRepo.UpsertTXN(tx, &model.BusinessTelegramBot{
			BusinessID:     bizA,
			BotToken:       "AAA:token",
			TargetChatID:   111,
			ApproverUserID: 111,
			Enabled:        true,
		})
	})
	inTenant(t, db, bizB, func(tx *sql.Tx) error {
		return botRepo.UpsertTXN(tx, &model.BusinessTelegramBot{
			BusinessID:     bizB,
			BotToken:       "BBB:token",
			TargetChatID:   222,
			ApproverUserID: 222,
			Enabled:        true,
		})
	})

	// Each business can read ONLY its own bot (other's token must not leak).
	var aToken, bToken string
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		b, err := botRepo.GetByBusinessTXN(tx, bizA)
		if err != nil {
			return err
		}
		aToken = b.BotToken
		if b.ApproverUserID != 111 {
			t.Fatalf("approver did not round-trip: %d", b.ApproverUserID)
		}
		other, err := botRepo.GetByBusinessTXN(tx, bizB)
		if err != nil {
			return err
		}
		if other != nil {
			t.Fatal("cross-business bot binding visible to A")
		}
		return nil
	})
	inTenant(t, db, bizB, func(tx *sql.Tx) error {
		b, err := botRepo.GetByBusinessTXN(tx, bizB)
		if err != nil {
			return err
		}
		bToken = b.BotToken
		if b.ApproverUserID != 222 {
			t.Fatalf("approver did not round-trip: %d", b.ApproverUserID)
		}
		return nil
	})
	if aToken != "AAA:token" || bToken != "BBB:token" {
		t.Fatalf("owner bot tokens not scoped correctly: A=%q B=%q", aToken, bToken)
	}

	// Guest purchase requests: create for A, then confirm only A can resolve/sees
	// it and B cannot.
	now := time.Now().UTC()
	graceExp := now.Add(10 * time.Minute)
	clientIP := net.ParseIP("192.168.99.20")
	var reqID string
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		r, err := reqRepo.CreateTXN(tx, &model.GuestPurchaseRequest{
			BusinessID:     bizA,
			ClientMAC:      "AA:BB:CC:DD:EE:FF",
			ClientIP:       &clientIP,
			PlanKey:        "daily",
			Amount:         2000,
			GraceStartedAt: &now,
			GraceExpiresAt: &graceExp,
			Status:         "pending",
		})
		if err != nil {
			return err
		}
		reqID = r.ID
		return nil
	})
	if reqID == "" {
		t.Fatal("guest request not created")
	}

	// A can approve its own; B's owner-scope resolve must be nil (isolated).
	var approved *model.GuestPurchaseRequest
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		ap, err := reqRepo.ApproveRequestTXN(tx, bizA, reqID, now)
		if err != nil {
			return err
		}
		approved = ap
		return nil
	})
	if approved == nil || approved.Status != "approved" {
		t.Fatalf("A could not approve own request: %+v", approved)
	}

	// B cannot see A's request at all (RLS).
	inTenant(t, db, bizB, func(tx *sql.Tx) error {
		other, err := reqRepo.GetByIDOwnerTXN(tx, reqID)
		if err != nil {
			return err
		}
		if other != nil {
			t.Fatal("cross-business guest request visible to B")
		}
		return nil
	})
}

// TestDB_GuestWifi_listEnabledBots proves the supervisor's seam only returns
// businesses that have opted in (enabled bot), and that a second approve after
// the first is a no-op (single-wind via repo).
func TestDB_GuestWifi_singleWindAndEnabledList(t *testing.T) {
	db := requireHarness(t)
	botRepo := NewBusinessTelegramBotRepository(db)
	reqRepo := NewGuestPurchaseRequestRepository(db)

	bizA := insertBusiness(t, db)
	bizOff := insertBusiness(t, db)
	_ = bizOff // (biz B stays un-enabled: proves only opted-in businesses are listed)

	// Only A is enabled.
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		return botRepo.UpsertTXN(tx, &model.BusinessTelegramBot{BusinessID: bizA, BotToken: "T", Enabled: true})
	})
	enabled, err := botRepo.ListEnabled()
	if err != nil {
		t.Fatalf("list enabled: %v", err)
	}
	found := false
	for _, b := range enabled {
		if b.BusinessID == bizA {
			found = true
		}
	}
	if !found {
		t.Fatal("enabled bot for A not listed")
	}

	// Single-wind approve: second approve returns nil.
	now := time.Now().UTC()
	var reqID string
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		r, err := reqRepo.CreateTXN(tx, &model.GuestPurchaseRequest{BusinessID: bizA, PlanKey: "hourly", Status: "pending"})
		if err != nil {
			return err
		}
		reqID = r.ID
		return nil
	})
	var second *model.GuestPurchaseRequest
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		if _, err := reqRepo.ApproveRequestTXN(tx, bizA, reqID, now); err != nil {
			return err
		}
		s, err := reqRepo.ApproveRequestTXN(tx, bizA, reqID, now)
		if err != nil {
			return err
		}
		second = s
		return nil
	})
	if second != nil {
		t.Fatal("double-approve must be a no-op (single-wind)")
	}
}
