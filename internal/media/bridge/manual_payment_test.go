package bridge

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestPostgresMediaReceiptOwnerDecision(t *testing.T) {
	store, db := mediaTestStore(t)
	for _, file := range []string{"20261003120000_magic_media_publications.sql", "20261003180000_magic_media_passes.sql"} {
		data, e := os.ReadFile("../../../migrations/" + file)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec(string(data)); e != nil {
			t.Fatal(e)
		}
	}
	ctx := context.Background()
	a, b := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	if _, e := db.Exec(`INSERT INTO businesses(business_id) VALUES($1),($2)`, a, b); e != nil {
		t.Fatal(e)
	}
	pub, e := store.Publish(ctx, a, validDesign(), 0)
	if e != nil {
		t.Fatal(e)
	}
	guest, _ := Secret()
	hash, _ := Hash(guest)
	create := func() MediaPass {
		key, _ := Secret()
		pass, e := store.CreateMediaPass(ctx, a, pub.PublicID, hash, key)
		if e != nil {
			t.Fatal(e)
		}
		return pass
	}
	pass := create()
	if resolved, e := store.ResolveGuestMediaPass(ctx, pub.PublicID, pass.ID, hash); e != nil || resolved != a {
		t.Fatal("matching guest order failed", e)
	}
	if _, e := store.ResolveGuestMediaPass(ctx, pub.PublicID, pass.ID, strings.Repeat("f", 64)); !errors.Is(e, ErrPassDenied) {
		t.Fatal("wrong guest resolved order")
	}
	proof := strings.Repeat("c", 64)
	receipt, e := store.SubmitMediaReceipt(ctx, a, pass.ID, hash, "/test/private/receipt", proof)
	if e != nil || !UUID.MatchString(receipt.ReviewID) || len("approve "+receipt.ReviewID) > 64 {
		t.Fatal("receipt or callback invalid", e)
	}
	if _, e = store.ActivateMediaPass(ctx, a, pass.ID, hash); !errors.Is(e, ErrPassDenied) {
		t.Fatal("upload granted access")
	}
	if _, e = store.SubmitMediaReceipt(ctx, a, pass.ID, hash, "/replacement", strings.Repeat("d", 64)); !errors.Is(e, ErrPassDenied) {
		t.Fatal("proof changed after submission")
	}
	retry, e := store.SubmitMediaReceipt(ctx, a, pass.ID, hash, "/duplicate-path", proof)
	if e != nil || retry.Path != receipt.Path || retry.ReviewID != receipt.ReviewID {
		t.Fatal("retry changed proof", e)
	}
	if _, e = store.DecideMediaReceipt(ctx, b, receipt.ReviewID, true); !errors.Is(e, ErrPassDenied) {
		t.Fatal("cross tenant approval")
	}
	// Receipt arrived on time; owners may review after the upload deadline.
	if _, e = db.Exec(`UPDATE magic_media_passes SET payment_deadline=now()-interval '1 minute' WHERE id=$1`, pass.ID); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, e := store.DecideMediaReceipt(ctx, a, receipt.ReviewID, true)
			if e != nil || p.State != "paid" || p.ActivatedAt != nil {
				t.Error("owner approval failed or started clock", e)
			}
		}()
	}
	wg.Wait()
	if _, e = store.ActivateMediaPass(ctx, a, pass.ID, hash); e != nil {
		t.Fatal("approved access failed", e)
	}
	if _, e = store.DecideMediaReceipt(ctx, a, receipt.ReviewID, false); !errors.Is(e, ErrPassDenied) {
		t.Fatal("contradictory decision")
	}
	duplicate := create()
	r, e := store.SubmitMediaReceipt(ctx, a, duplicate.ID, hash, "/test/duplicate", proof)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = store.DecideMediaReceipt(ctx, a, r.ReviewID, true); !errors.Is(e, ErrPaymentMismatch) {
		t.Fatal("same proof approved twice", e)
	}
	rejected := create()
	r, e = store.SubmitMediaReceipt(ctx, a, rejected.ID, hash, "/test/rejected", strings.Repeat("e", 64))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = store.DecideMediaReceipt(ctx, a, r.ReviewID, false); e != nil {
		t.Fatal(e)
	}
	if _, e = store.DecideMediaReceipt(ctx, a, r.ReviewID, true); !errors.Is(e, ErrPassDenied) {
		t.Fatal("rejection resurrected")
	}
	if e = store.RevokeMediaPass(ctx, a, pass.ID, true); e != nil {
		t.Fatal(e)
	}
	if _, e = store.DecideMediaReceipt(ctx, a, receipt.ReviewID, true); !errors.Is(e, ErrPassDenied) {
		t.Fatal("refund resurrected")
	}
	late := create()
	if _, e = db.Exec(`UPDATE magic_media_passes SET payment_deadline=now()-interval '1 minute' WHERE id=$1`, late.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = store.SubmitMediaReceipt(ctx, a, late.ID, hash, "/late", proof); !errors.Is(e, ErrPassDenied) {
		t.Fatal("late upload accepted")
	}
}
