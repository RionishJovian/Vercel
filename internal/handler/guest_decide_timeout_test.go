package handler

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"mega-magic/internal/database"
	"mega-magic/internal/model"
	"mega-magic/internal/provisioner/routeros"
	"mega-magic/internal/telegram"
)

// hangingRouter never answers; it returns only when its context is cancelled, the
// way an offline router behaves behind a real transport.
type hangingRouter struct{ sawDeadline bool }

func (r *hangingRouter) ConfigureGuestGrace(context.Context, string, string, routeros.GuestGrace) error {
	return nil
}
func (r *hangingRouter) DecideGuestAccess(ctx context.Context, _, _, _, _ string, _ bool, _, _, _, _ int) error {
	_, r.sawDeadline = ctx.Deadline()
	<-ctx.Done()
	return ctx.Err()
}

func TestDecideGuestDoesNotHangOnAnOfflineRouter(t *testing.T) {
	db := harness_testdb(t)
	previous := guestRouterDecisionTimeout
	guestRouterDecisionTimeout = 200 * time.Millisecond
	t.Cleanup(func() { guestRouterDecisionTimeout = previous })

	biz := insertBusiness(db)
	var device string
	if err := db.QueryRow(`INSERT INTO devices(business_id,hardware_name) VALUES($1,'Offline gateway') RETURNING device_id`, biz).Scan(&device); err != nil {
		t.Fatal(err)
	}
	repo := database.NewGuestPurchaseRequestRepository(db)
	h := NewGuestWifiHandler(db, database.NewDeviceRepository(db), database.NewBusinessTelegramBotRepository(db), repo, database.NewPortalConfigRepository(db), database.NewBusinessMeteredQuotaRepository(db), t.TempDir(), func(string) (telegram.Messenger, bool) { return nil, false })
	router := &hangingRouter{}
	h.EnableGuestGrace(router)

	minutes := 90
	var id string
	if err := withTenant(db, biz, func(tx *sql.Tx) error {
		r, err := repo.CreateTXN(tx, &model.GuestPurchaseRequest{BusinessID: biz, DeviceID: &device, ClientMAC: "02:00:00:00:09:09", PlanKey: "daily", Amount: 1500, PurchasedMinutes: &minutes, Status: "pending"})
		if err == nil {
			id = r.ID
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}

	started := time.Now()
	err := h.DecidePurchaseRequest(biz, id, true)
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("an offline router must end in a deadline error, got %v", err)
	}
	if time.Since(started) > 5*time.Second {
		t.Fatal("the router call was not bounded")
	}
	if !router.sawDeadline {
		t.Fatal("the router call carried no deadline")
	}
	// The transaction rolled back: the guest's request is still pending and can be retried.
	var status string
	if err := db.QueryRow(`SELECT status FROM guest_purchase_requests WHERE id=$1`, id).Scan(&status); err != nil || status != "pending" {
		t.Fatalf("request must stay pending after a router timeout, got %q (%v)", status, err)
	}
}
