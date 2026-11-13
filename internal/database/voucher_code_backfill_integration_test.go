package database

import (
	"context"
	"testing"
	"time"
)

func TestDB_VoucherCodeBackfill_SealsThenClearsOnlyVerifiedRows(t *testing.T) {
	db := requireHarness(t)
	repo := NewVoucherRepository(db)
	biz := insertBusiness(t, db)
	a, b, c := newActiveVoucher(t, repo, biz, 60, nil), newActiveVoucher(t, repo, biz, 60, nil), newActiveVoucher(t, repo, biz, 60, nil)
	p := useVoucherCodeProtection(t, false) // key loaded, new rows still plaintext
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Dry run changes nothing.
	before, _ := db.VoucherCodeStats(ctx)
	r, err := db.ProtectVoucherCodes(ctx, p, false, false, 2)
	if err != nil || r.Applied || r.Sealed != 0 {
		t.Fatalf("dry run: %+v %v", r, err)
	}
	if after, _ := db.VoucherCodeStats(ctx); after.PlaintextOnly != before.PlaintextOnly || after.Both != before.Both {
		t.Fatal("a dry run changed rows")
	}
	if _, err := db.ProtectVoucherCodes(ctx, p, false, true, 2); err == nil {
		t.Fatal("clearing plaintext without apply must be refused")
	}

	// Step 1: sealed copies, plaintext kept, app reads and lookups unchanged. Tiny batches exercise pagination.
	r, err = db.ProtectVoucherCodes(ctx, p, true, false, 2)
	if err != nil || r.Sealed < 3 {
		t.Fatalf("seal: %+v %v", r, err)
	}
	for _, v := range []string{a.ID, b.ID, c.ID} {
		plain, enc, blind := storedCodeForm(t, db, v)
		if !plain.Valid || !enc.Valid || !blind.Valid {
			t.Fatalf("step 1 must keep plaintext and add sealed copy + index (row %s)", v)
		}
	}
	if got, err := repo.GetVoucherByCode(biz, a.Code); err != nil || got == nil || got.ID != a.ID {
		t.Fatalf("lookup after step 1: %+v %v", got, err)
	}
	// Idempotent.
	if again, err := db.ProtectVoucherCodes(ctx, p, true, false, 2); err != nil || again.Sealed != 0 {
		t.Fatalf("second run must seal nothing: %+v %v", again, err)
	}

	// Corrupt one sealed copy: the clear step must leave that row's plaintext alone and say so.
	other, _ := p.Seal("SOMETHINGELSE")
	if _, err := db.Exec(`UPDATE vouchers SET code_enc=$2 WHERE id=$1`, c.ID, other); err != nil {
		t.Fatal(err)
	}
	r, err = db.ProtectVoucherCodes(ctx, p, true, true, 2)
	if err != nil || r.Mismatched < 1 || r.Cleared < 2 {
		t.Fatalf("clear: %+v %v", r, err)
	}
	for _, id := range []string{a.ID, b.ID} {
		if plain, _, _ := storedCodeForm(t, db, id); plain.Valid {
			t.Fatalf("verified row %s kept its plaintext", id)
		}
	}
	if plain, _, _ := storedCodeForm(t, db, c.ID); !plain.Valid {
		t.Fatal("a row whose sealed copy does not match must keep its plaintext")
	}

	// Protected rows read and redeem exactly like before.
	if got, err := repo.GetVoucherByID(a.ID); err != nil || got == nil || got.Code != a.Code {
		t.Fatalf("read after clear: %+v %v", got, err)
	}
	tx, _ := db.Begin()
	claimed, err := repo.ClaimVoucherTXN(tx, biz, b.Code, time.Now().UTC())
	_ = tx.Commit()
	if err != nil || claimed == nil || claimed.Code != b.Code {
		t.Fatalf("claim by typed code after clear: %+v %v", claimed, err)
	}

	if _, err := db.ProtectVoucherCodes(ctx, nil, true, false, 2); err == nil {
		t.Fatal("running without a key must be refused")
	}
}
