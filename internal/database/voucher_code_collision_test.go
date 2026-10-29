package database

import (
	"database/sql"
	"errors"
	"testing"

	"mega-magic/internal/model"
)

func TestDBVoucherCodeCollisionAllowsRetryInSameTransaction(t *testing.T) {
	db := requireHarness(t)
	repo := NewVoucherRepository(db)
	biz, other := insertBusiness(t, db), insertBusiness(t, db)
	makeVoucher := func(tenant, code string) *model.Voucher {
		return &model.Voucher{BusinessID: tenant, Code: code, PlanKey: "guest-internet", PlanLabel: "Daily", Status: "unused", PriceMMK: 1000}
	}
	var firstID string
	inTenant(t, db, biz, func(tx *sql.Tx) error {
		first, err := repo.CreateVoucherTXN(tx, makeVoucher(biz, "ABCDEFGH"))
		if err != nil {
			return err
		}
		firstID = first.ID
		if got, err := repo.CreateVoucherTXN(tx, makeVoucher(biz, "ABCDEFGH")); !errors.Is(err, ErrVoucherCodeCollision) || got != nil {
			t.Fatal("collision did not return a safe retry signal")
		}
		_, err = repo.CreateVoucherTXN(tx, makeVoucher(biz, "HGFEDCBA"))
		return err
	})
	inTenant(t, db, other, func(tx *sql.Tx) error {
		_, err := repo.CreateVoucherTXN(tx, makeVoucher(other, "ABCDEFGH"))
		return err
	})
	var count int
	var id string
	if err := db.QueryRow(`SELECT count(*) FROM vouchers WHERE business_id=$1`, biz).Scan(&count); err != nil || count != 2 {
		t.Fatal("retry transaction did not commit exactly two codes", err)
	}
	if err := db.QueryRow(`SELECT id FROM vouchers WHERE business_id=$1 AND code='ABCDEFGH'`, biz).Scan(&id); err != nil || id != firstID {
		t.Fatal("collision replaced the original code", err)
	}
}
