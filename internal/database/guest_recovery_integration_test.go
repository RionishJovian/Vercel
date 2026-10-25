package database

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"mega-magic/internal/guestaccess"
	"mega-magic/internal/model"
)

func TestDB_GuestAccessRecovery(t *testing.T) {
	db := requireHarness(t)
	for _, path := range []string{"20261001200000_guest_voucher_grants.sql", "20261001210000_guest_access_recovery.sql", "20261002200000_guest_recovery_completion.sql", "20261021090000_guest_voucher_activation.sql"} {
		b, err := os.ReadFile("../../migrations/" + path)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 2; i++ {
			if _, err := db.Exec(string(b)); err != nil {
				t.Fatal(err)
			}
		}
	}
	biz, other := insertBusiness(t, db), insertBusiness(t, db)
	device := insertDeviceRow(t, db, biz, nil, "recovery test", "MAIN_GATEWAY")
	otherDevice := insertDeviceRow(t, db, other, nil, "other tenant", "MAIN_GATEWAY")
	vr := GuestVoucherRepository{}
	rr := GuestRecoveryRepository{DB: db}
	var v *model.Voucher
	var grant *model.GuestVoucherGrant
	var secret string
	inTenant(t, db, biz, func(tx *sql.Tx) error {
		var err error
		v, err = NewVoucherRepository(db).CreateVoucherTXN(tx, &model.Voucher{BusinessID: biz, PlanKey: "guest-recovery", PlanLabel: "Guest", Code: "RESTORE" + strings.ToUpper(randHex(8)), Status: "unused"})
		if err != nil {
			return err
		}
		if err = vr.IssueTXN(tx, biz, v.ID, device, 60, 75); err != nil {
			return err
		}
		grant, err = vr.ReserveTXN(tx, biz, device, v.Code, "AA:BB:CC:DD:EE:FE")
		if err != nil {
			return err
		}
		ok, err := vr.FinalizeTXN(tx, biz, v.ID, grant.LeaseToken)
		if err != nil {
			return err
		}
		if !ok {
			t.Fatal("fixture could not activate")
		}
		return nil
	})
	grant.State, grant.LeaseToken, grant.LeaseUntil = "active", "", nil
	// Concurrent verified retries commit exactly one credential. Subsequent
	// voucher-code/MAC retries cannot rotate it or recover its plaintext.
	var issuers sync.WaitGroup
	secrets := make(chan string, 8)
	issueErrors := make(chan error, 8)
	for i := 0; i < 8; i++ {
		issuers.Add(1)
		go func() {
			defer issuers.Done()
			s, err := rr.Issue(context.Background(), *grant)
			if err != nil {
				issueErrors <- err
			} else if s != "" {
				secrets <- s
			}
		}()
	}
	issuers.Wait()
	close(secrets)
	if len(issueErrors) != 0 || len(secrets) != 1 {
		t.Fatalf("credential issuance errors=%d secrets=%d", len(issueErrors), len(secrets))
	}
	secret = <-secrets
	if again, err := rr.Issue(context.Background(), *grant); err != nil || again != "" {
		t.Fatal("retry replaced or disclosed credential")
	}
	for _, mutate := range []func(*model.GuestVoucherGrant){
		func(g *model.GuestVoucherGrant) { g.BusinessID = other },
		func(g *model.GuestVoucherGrant) { g.DeviceID = otherDevice },
		func(g *model.GuestVoucherGrant) { g.ClientMAC = "AA:BB:CC:DD:EE:FC" },
		func(g *model.GuestVoucherGrant) { g.DataMB++ },
		func(g *model.GuestVoucherGrant) { g.State = "provisioning" },
	} {
		bad := *grant
		mutate(&bad)
		if s, err := rr.Issue(context.Background(), bad); err == nil || s != "" {
			t.Fatal("unverified grant received credential")
		}
	}
	hash, _ := guestaccess.HashSecret(secret)
	current := guestaccess.RecoveryRequest{BusinessID: biz, DeviceID: device, VoucherID: v.ID, NewMAC: grant.ClientMAC}
	first, err := rr.Lookup(context.Background(), current, hash)
	if err != nil || first.ClientMAC != grant.ClientMAC || first.PriorUsedBytes != 0 || !first.ExpiresAt.Equal(*grant.ExpiresAt) {
		t.Fatal("current binding lookup lost original entitlement")
	}
	for i := 0; i < 3; i++ {
		again, err := rr.Lookup(context.Background(), current, hash)
		if err != nil || again != first {
			t.Fatal("read-only lookup mutated recovery state")
		}
	}
	req := guestaccess.RecoveryRequest{BusinessID: biz, DeviceID: device, VoucherID: v.ID, NewMAC: "AA:BB:CC:DD:EE:FC"}
	assertCodeRetryDenied := func(mac string) {
		t.Helper()
		router := &provisionTestRouter{}
		service := guestaccess.ProvisionService{Store: GuestProvisionRepository{DB: db}, Router: router}
		result, err := service.Redeem(context.Background(), guestaccess.ProvisionRequest{BusinessID: biz, DeviceID: device, Code: v.Code, ClientMAC: mac})
		if !errors.Is(err, ErrGuestVoucherUnavailable) || result.State != "" || router.calls != 0 || router.contains != 0 {
			t.Fatal("code retry touched router or confirmed original allowance")
		}
	}
	assertCodeRetryDenied(grant.ClientMAC)
	wrong, _ := guestaccess.RecoverySecret()
	wrongHash, _ := guestaccess.HashSecret(wrong)
	if _, err := rr.Lookup(context.Background(), current, wrongHash); err == nil {
		t.Fatal("current binding lookup accepted wrong credential")
	}
	for _, mutate := range []func(*guestaccess.RecoveryRequest){
		func(r *guestaccess.RecoveryRequest) { r.BusinessID = other },
		func(r *guestaccess.RecoveryRequest) { r.DeviceID = otherDevice },
		func(r *guestaccess.RecoveryRequest) { r.NewMAC = "AA:BB:CC:DD:EE:FC" },
	} {
		bad := current
		mutate(&bad)
		if _, err := rr.Lookup(context.Background(), bad, hash); err == nil {
			t.Fatal("current binding lookup accepted foreign or different connection")
		}
	}
	if _, err := rr.Reserve(context.Background(), req, wrongHash); err == nil {
		t.Fatal("wrong credential accepted")
	}
	foreign := req
	foreign.BusinessID = other
	foreign.DeviceID = otherDevice
	if _, err := rr.Reserve(context.Background(), foreign, hash); err == nil {
		t.Fatal("foreign tenant accepted")
	}
	foreign = req
	foreign.DeviceID = otherDevice
	if _, err := rr.Reserve(context.Background(), foreign, hash); err == nil {
		t.Fatal("wrong gateway accepted")
	}
	var wg sync.WaitGroup
	results := make(chan guestaccess.RecoveryClaim, 8)
	denied := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			g, err := rr.Reserve(context.Background(), req, hash)
			if err != nil {
				denied <- err
			} else {
				results <- g
			}
		}()
	}
	wg.Wait()
	close(results)
	close(denied)
	var g guestaccess.RecoveryClaim
	wins := 0
	for claim := range results {
		g = claim
		wins++
	}
	if wins != 1 || len(denied) != 7 {
		t.Fatalf("wins=%d denied=%d", wins, len(denied))
	}
	// A transfer claim must not allow a stale original-code worker to enter
	// provisioning or containment while recovery owns the current binding.
	assertCodeRetryDenied(grant.ClientMAC)
	if !g.ExpiresAt.Equal(*grant.ExpiresAt) {
		t.Fatal("recovery moved deadline")
	}
	if err := rr.Complete(context.Background(), g); err == nil {
		t.Fatal("activated without usage checkpoint")
	}
	frozen, err := rr.FreezeUsage(context.Background(), g, 40000000)
	if err != nil {
		t.Fatal(err)
	}
	if frozen.UsedBytes != 40000000 {
		t.Fatal("usage not persisted")
	}
	if _, err := rr.FreezeUsage(context.Background(), g, 40000000); err == nil {
		t.Fatal("checkpoint applied twice")
	}
	if err := rr.Complete(context.Background(), frozen); err != nil {
		t.Fatal(err)
	}
	if _, err := rr.Lookup(context.Background(), current, hash); err == nil {
		t.Fatal("old MAC remained eligible after transfer")
	}
	current.NewMAC = g.NewMAC
	bound, err := rr.Lookup(context.Background(), current, hash)
	if err != nil || bound.PriorUsedBytes != 40000000 || bound.TotalBytes != 75000000 || !bound.ExpiresAt.Equal(first.ExpiresAt) || bound.Revision.Equal(first.Revision) {
		t.Fatal("current binding lookup replenished transferred allowance")
	}
	assertCodeRetryDenied(grant.ClientMAC)
	assertCodeRetryDenied(g.NewMAC)
	afterDenied, err := rr.Lookup(context.Background(), current, hash)
	if err != nil || afterDenied != bound {
		t.Fatal("code retry disturbed transferred binding")
	}
	if err := rr.Complete(context.Background(), frozen); err == nil {
		t.Fatal("stale transfer finalized twice")
	}
	// A later rotation must carry cumulative consumption, never a fresh quota.
	req.NewMAC = "AA:BB:CC:DD:EE:FA"
	next, err := rr.Reserve(context.Background(), req, hash)
	if err != nil {
		t.Fatal(err)
	}
	if next.OldMAC != g.NewMAC || next.UsedBytes != 40000000 || !next.ExpiresAt.Equal(*grant.ExpiresAt) {
		t.Fatal("replacement binding lost purchase ledger")
	}
	second, err := rr.FreezeUsage(context.Background(), next, 10000000)
	if err != nil {
		t.Fatal(err)
	}
	if err = rr.Complete(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	// Delayed cleanup of the previous completion cannot fence this newer one.
	if err = rr.Block(context.Background(), frozen); err != nil {
		t.Fatal(err)
	}
	current.NewMAC = second.NewMAC
	if _, err = rr.Lookup(context.Background(), current, hash); err != nil {
		t.Fatal("stale cleanup blocked newer completion")
	}
	req.NewMAC = "AA:BB:CC:DD:EE:F8"
	third, err := rr.Reserve(context.Background(), req, hash)
	if err != nil {
		t.Fatal(err)
	}
	third, err = rr.FreezeUsage(context.Background(), third, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err = rr.Complete(context.Background(), third); err != nil {
		t.Fatal(err)
	}
	req.NewMAC = "AA:BB:CC:DD:EE:F6"
	if _, err = rr.Reserve(context.Background(), req, hash); err == nil {
		t.Fatal("fourth transfer bypassed rate limit")
	}
	// Advance only the fixture's transfer rate window, not purchase validity.
	inTenant(t, db, biz, func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE guest_access_recovery SET window_started_at=clock_timestamp()-interval '6 hours' WHERE voucher_id=$1`, v.ID)
		return err
	})
	next, err = rr.Reserve(context.Background(), req, hash)
	if err != nil {
		t.Fatal(err)
	}
	if next.UsedBytes != 50000000 || !next.ExpiresAt.Equal(*grant.ExpiresAt) {
		t.Fatal("rate-window reset replenished entitlement")
	}
	if err := rr.Block(context.Background(), next); err != nil {
		t.Fatal(err)
	}
	current.NewMAC = next.OldMAC
	if _, err := rr.Lookup(context.Background(), current, hash); err == nil {
		t.Fatal("blocked transfer permitted current-binding resume")
	}
	if _, err := rr.Reserve(context.Background(), req, hash); err == nil {
		t.Fatal("uncertain transfer retried without reconciliation")
	}
	inTenant(t, db, biz, func(tx *sql.Tx) error {
		var firstMAC string
		if err := tx.QueryRow(`SELECT client_mac::text FROM guest_voucher_grants WHERE voucher_id=$1`, v.ID).Scan(&firstMAC); err != nil {
			return err
		}
		if strings.ToUpper(firstMAC) != "AA:BB:CC:DD:EE:FE" {
			t.Fatal("first-use audit MAC changed")
		}
		return nil
	})
	// Database guard denies data replenishment even outside repository helpers.
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	err = WithTenant(tx, biz, func() error {
		_, err := tx.Exec(`UPDATE guest_access_recovery SET used_bytes=0 WHERE voucher_id=$1`, v.ID)
		return err
	})
	if err == nil {
		t.Fatal("consumption was reset")
	}
}
