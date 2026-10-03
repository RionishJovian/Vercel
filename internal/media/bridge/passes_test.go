package bridge

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sync"
	"testing"
	"time"
)

func TestSettledMediaPaymentValidation(t *testing.T) {
	p := SettledPayment{PassID: string(make([]byte, 64)), Provider: "test", Reference: "settled-test", Currency: "MMK", Amount: 1500}
	p.PassID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if !p.validate() {
		t.Fatal("valid settlement rejected")
	}
	for _, mutate := range []func(*SettledPayment){
		func(p *SettledPayment) { p.PassID = "" }, func(p *SettledPayment) { p.Provider = "TEST /" },
		func(p *SettledPayment) { p.Reference = "\r\nsecret" }, func(p *SettledPayment) { p.Currency = "USD" },
		func(p *SettledPayment) { p.Amount = 0 }, func(p *SettledPayment) { p.Amount = 1000001 },
	} {
		q := p
		mutate(&q)
		if q.validate() {
			t.Fatal("invalid settlement accepted")
		}
	}
}

func TestPostgresCheckoutCannotReuseAnotherPublication(t *testing.T) {
	store, db := mediaTestStore(t)
	for _, name := range []string{"20261003120000_magic_media_publications.sql", "20261003180000_magic_media_passes.sql"} {
		raw, err := os.ReadFile("../../../migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(string(raw)); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	bid := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	if _, err := db.Exec("INSERT INTO businesses(business_id) VALUES($1)", bid); err != nil {
		t.Fatal(err)
	}
	first, err := store.Publish(ctx, bid, validDesign(), 0)
	if err != nil {
		t.Fatal(err)
	}
	guest, _ := Secret()
	hash, _ := Hash(guest)
	key, _ := Secret()
	pass, err := store.CreateMediaPass(ctx, bid, first.PublicID, hash, key)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Unpublish(ctx, bid, first.Revision); err != nil {
		t.Fatal(err)
	}
	second, err := store.Publish(ctx, bid, validDesign(), 0)
	if err != nil || second.PublicID == first.PublicID {
		t.Fatal("fresh publication required", err)
	}
	if _, err = store.CreateMediaPass(ctx, bid, second.PublicID, hash, key); !errors.Is(err, ErrPassDenied) {
		t.Fatal("idempotency key returned or rebound an old publication's order", err)
	}
	key2, _ := Secret()
	if _, err = store.CreateMediaPass(ctx, bid, second.PublicID, hash, key2); err != nil {
		t.Fatal(err)
	}
	saved, err := store.MediaPass(ctx, bid, pass.ID, hash)
	if err != nil || saved.ID != pass.ID || saved.State != "pending" {
		t.Fatal("old order was modified", err)
	}
}

func TestPostgresMediaPassPaymentAndExpiry(t *testing.T) {
	store, db := mediaTestStore(t)
	for _, file := range []string{"20261003120000_magic_media_publications.sql", "20261003180000_magic_media_passes.sql"} {
		raw, e := os.ReadFile("../../../migrations/" + file)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec(string(raw)); e != nil {
			t.Fatal(e)
		}
	}
	ctx := context.Background()
	a := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	b := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	if _, e := db.Exec(`INSERT INTO businesses(business_id)VALUES($1),($2)`, a, b); e != nil {
		t.Fatal(e)
	}
	publication, e := store.Publish(ctx, a, validDesign(), 0)
	if e != nil {
		t.Fatal(e)
	}
	guest, _ := Secret()
	guestHash, _ := Hash(guest)
	other, _ := Secret()
	key, _ := Secret()
	pass, e := store.CreateMediaPass(ctx, a, publication.PublicID, guestHash, key)
	if e != nil {
		t.Fatal(e)
	}
	if pass.Amount != 1500 || pass.DurationSeconds != 24*3600 || pass.State != "pending" {
		t.Fatal("wrong server snapshot")
	}
	if _, e = store.ActivateMediaPass(ctx, a, pass.ID, guestHash); !errors.Is(e, ErrPassDenied) {
		t.Fatal("unpaid pass activated")
	}
	if _, e = store.MediaPass(ctx, b, pass.ID, guestHash); !errors.Is(e, ErrPassDenied) {
		t.Fatal("cross tenant access")
	}
	if _, e = store.MediaPass(ctx, a, pass.ID, other); !errors.Is(e, ErrPassDenied) {
		t.Fatal("wrong guest access")
	}
	design := validDesign()
	design.Price = 9000
	design.Duration = 48
	design.Payment.BankAccount = "changed-account"
	if _, e = store.Publish(ctx, a, design, 1); e != nil {
		t.Fatal(e)
	}
	retry, e := store.CreateMediaPass(ctx, a, publication.PublicID, guestHash, key)
	if retry.Payment == nil || retry.Payment.BankAccount != "1234" {
		t.Fatal("checkout retry mutated receiving details")
	}
	if e != nil || retry.ID != pass.ID || retry.Amount != 1500 || retry.DurationSeconds != 86400 || !retry.PaymentDeadline.Equal(pass.PaymentDeadline) {
		t.Fatal("checkout retry mutated terms", e)
	}
	payment := SettledPayment{PassID: pass.ID, Provider: "synthetic_test", Reference: "test-settled-1", Currency: "MMK", Amount: 1500}
	wrong := payment
	wrong.Amount = 1499
	if _, e = store.SettleMediaPass(ctx, a, wrong); !errors.Is(e, ErrPaymentMismatch) {
		t.Fatal("wrong amount accepted")
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := store.SettleMediaPass(ctx, a, payment); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	paid, e := store.MediaPass(ctx, a, pass.ID, guestHash)
	if e != nil || paid.State != "paid" || paid.ExpiresAt != nil {
		t.Fatal("payment prematurely started access", e)
	}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := store.ActivateMediaPass(ctx, a, pass.ID, guestHash); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	active, e := store.AuthorizeMediaPass(ctx, a, pass.ID, guestHash)
	if e != nil || active.ExpiresAt == nil || active.ActivatedAt == nil || active.ExpiresAt.Sub(*active.ActivatedAt) != 24*time.Hour {
		t.Fatal("activation deadline incorrect", e)
	}
	replay, e := store.ActivateMediaPass(ctx, a, pass.ID, guestHash)
	if e != nil || !replay.ExpiresAt.Equal(*active.ExpiresAt) {
		t.Fatal("retry extended deadline", e)
	}
	// One provider payment cannot settle a second pass, even in another tenant.
	key2, _ := Secret()
	second, e := store.CreateMediaPass(ctx, a, publication.PublicID, guestHash, key2)
	if e != nil {
		t.Fatal(e)
	}
	reused := payment
	reused.PassID = second.ID
	reused.Amount = 9000
	if _, e = store.SettleMediaPass(ctx, a, reused); e == nil {
		t.Fatal("reused provider reference accepted")
	}
	if e = store.RevokeMediaPass(ctx, a, pass.ID, true); e != nil {
		t.Fatal(e)
	}
	if _, e = store.AuthorizeMediaPass(ctx, a, pass.ID, guestHash); !errors.Is(e, ErrPassDenied) {
		t.Fatal("refunded pass authorized")
	}
	if _, e = store.SettleMediaPass(ctx, a, payment); !errors.Is(e, ErrPassDenied) {
		t.Fatal("refund resurrected by payment replay")
	}
	// Database expiry, not a UI timer or background worker, enforces access.
	reused.Reference = "test-settled-2"
	if _, e = store.SettleMediaPass(ctx, a, reused); e != nil {
		t.Fatal(e)
	}
	if _, e = store.ActivateMediaPass(ctx, a, second.ID, guestHash); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`UPDATE magic_media_passes SET activated_at=now()-interval '3 days',expires_at=now()-interval '3 days'+duration_seconds*interval '1 second' WHERE id=$1`, second.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = store.AuthorizeMediaPass(ctx, a, second.ID, guestHash); !errors.Is(e, ErrPassDenied) {
		t.Fatal("expired access authorized")
	}
	if _, e = store.ActivateMediaPass(ctx, a, second.ID, guestHash); !errors.Is(e, ErrPassDenied) {
		t.Fatal("expired pass renewed")
	}
	var count int
	if e = store.tenant(ctx, b, func(tx *sql.Tx) error { return tx.QueryRow(`SELECT count(*) FROM magic_media_passes`).Scan(&count) }); e != nil || count != 0 {
		t.Fatal("RLS isolation failed", e)
	}
	// Inactive owners cannot sell or authorize, including an idempotent checkout.
	if _, e = db.Exec(`UPDATE businesses SET is_active=false WHERE business_id=$1`, a); e != nil {
		t.Fatal(e)
	}
	if _, e = store.MediaPass(ctx, a, second.ID, guestHash); !errors.Is(e, ErrPassDenied) {
		t.Fatal("inactive business pass visible")
	}
	if _, e = store.CreateMediaPass(ctx, a, publication.PublicID, guestHash, key); !errors.Is(e, ErrPassDenied) {
		t.Fatal("inactive business checkout retry succeeded")
	}
}
