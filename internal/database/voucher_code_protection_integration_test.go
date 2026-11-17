package database

import (
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"mega-magic/internal/model"
	"mega-magic/internal/vouchercode"
)

// useVoucherCodeProtection loads a fresh key for one test and restores the default (no key, plaintext) afterwards.
func useVoucherCodeProtection(t *testing.T, protectNew bool) *vouchercode.Protector {
	t.Helper()
	key, err := vouchercode.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	p, err := vouchercode.New(key)
	if err != nil {
		t.Fatal(err)
	}
	if err := ConfigureVoucherCodes(p, protectNew); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ConfigureVoucherCodes(nil, false) })
	return p
}

func storedCodeForm(t *testing.T, db *DB, id string) (plain sql.NullString, enc, blind sql.NullString) {
	t.Helper()
	if err := db.QueryRow(`SELECT code, code_enc, code_hmac FROM vouchers WHERE id=$1`, id).Scan(&plain, &enc, &blind); err != nil {
		t.Fatal(err)
	}
	return
}

// A protected row must hold no plaintext, yet every read path still returns the real code.
func TestDB_VoucherCodeProtection_StoresNoPlaintextAndStaysReadable(t *testing.T) {
	db := requireHarness(t)
	p := useVoucherCodeProtection(t, true)
	repo := NewVoucherRepository(db)
	biz := insertBusiness(t, db)

	v := newActiveVoucher(t, repo, biz, 60, nil)
	plain, enc, blind := storedCodeForm(t, db, v.ID)
	if plain.Valid {
		t.Fatalf("protected voucher still stores the plaintext code: %q", plain.String)
	}
	if !enc.Valid || strings.Contains(enc.String, v.Code) {
		t.Fatal("sealed value is missing or contains the code")
	}
	if !blind.Valid || blind.String != p.Blind(v.Code) {
		t.Fatal("blind index does not match the code")
	}

	got, err := repo.GetVoucherByCode(biz, v.Code)
	if err != nil || got == nil || got.ID != v.ID || got.Code != v.Code {
		t.Fatalf("lookup by typed code failed: %+v %v", got, err)
	}
	byID, err := repo.GetVoucherByID(v.ID)
	if err != nil || byID == nil || byID.Code != v.Code {
		t.Fatalf("read by id must return the real code: %+v %v", byID, err)
	}
	if miss, err := repo.GetVoucherByCode(biz, v.Code+"X"); err != nil || miss != nil {
		t.Fatalf("a different code must not match: %+v %v", miss, err)
	}
	other := insertBusiness(t, db)
	if cross, err := repo.GetVoucherByCode(other, v.Code); err != nil || cross != nil {
		t.Fatalf("another business must not find the code: %+v %v", cross, err)
	}
}

// Single redemption must stay atomic when the lookup goes through the blind index.
func TestDB_VoucherCodeProtection_ClaimStillExactlyOnce(t *testing.T) {
	db := requireHarness(t)
	useVoucherCodeProtection(t, true)
	repo := NewVoucherRepository(db)
	biz := insertBusiness(t, db)
	v := newActiveVoucher(t, repo, biz, 60, nil)
	now := time.Now().UTC()

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	first, err := repo.ClaimVoucherTXN(tx, biz, v.Code, now)
	if err != nil || first == nil || first.Code != v.Code {
		t.Fatalf("first claim: %+v %v", first, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	tx2, _ := db.Begin()
	second, err := repo.ClaimVoucherTXN(tx2, biz, v.Code, now)
	_ = tx2.Rollback()
	if err != nil || second != nil {
		t.Fatalf("second claim must be a no-op: %+v %v", second, err)
	}
}

// Uniqueness per business must survive: the plaintext constraint no longer sees protected rows.
func TestDB_VoucherCodeProtection_DuplicateCodeRejectedPerBusiness(t *testing.T) {
	db := requireHarness(t)
	useVoucherCodeProtection(t, true)
	repo := NewVoucherRepository(db)
	biz, other := insertBusiness(t, db), insertBusiness(t, db)
	v := newActiveVoucher(t, repo, biz, 60, nil)

	dup := &model.Voucher{BusinessID: biz, PlanKey: "sovereign", PlanLabel: "Sovereign", Code: v.Code, Status: "active"}
	if _, err := repo.CreateVoucher(dup); err == nil {
		t.Fatal("a duplicate protected code in the same business must be rejected")
	}
	inTenant(t, db, biz, func(tx *sql.Tx) error {
		if _, err := repo.CreateVoucherTXN(tx, dup); !errors.Is(err, ErrVoucherCodeCollision) {
			t.Fatalf("TXN duplicate: want ErrVoucherCodeCollision, got %v", err)
		}
		return nil
	})
	dup.BusinessID = other
	if _, err := repo.CreateVoucher(dup); err != nil {
		t.Fatalf("the same code in another business is allowed: %v", err)
	}
}

// Flag changes must never strand data: legacy plaintext rows keep working after protection is on, protected rows keep
// working after it is turned off, and without the key a protected row fails closed instead of reading as empty.
func TestDB_VoucherCodeProtection_MixedRowsAndFailClosed(t *testing.T) {
	db := requireHarness(t)
	repo := NewVoucherRepository(db)
	biz := insertBusiness(t, db)

	legacy := newActiveVoucher(t, repo, biz, 60, nil) // protection off: plaintext row
	if plain, enc, blind := storedCodeForm(t, db, legacy.ID); !plain.Valid || enc.Valid || blind.Valid {
		t.Fatal("with protection off a row must stay plaintext-only, as before")
	}

	useVoucherCodeProtection(t, true)
	if got, err := repo.GetVoucherByCode(biz, legacy.Code); err != nil || got == nil || got.Code != legacy.Code {
		t.Fatalf("legacy row must stay readable once protection is on: %+v %v", got, err)
	}
	protected := newActiveVoucher(t, repo, biz, 60, nil)

	// Flag off, key still loaded: reads of protected rows continue, new rows are plaintext again.
	p, _ := voucherCodeState()
	if err := ConfigureVoucherCodes(p, false); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.GetVoucherByCode(biz, protected.Code); err != nil || got == nil || got.Code != protected.Code {
		t.Fatalf("protected row must stay readable with the flag off: %+v %v", got, err)
	}
	if plain, _, _ := storedCodeForm(t, db, newActiveVoucher(t, repo, biz, 60, nil).ID); !plain.Valid {
		t.Fatal("flag off must write plaintext again")
	}

	// No key at all: a protected row is an error, never an empty code.
	if err := ConfigureVoucherCodes(nil, false); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.GetVoucherByID(protected.ID); !errors.Is(err, ErrVoucherCodeLocked) {
		t.Fatalf("protected row without a key: want ErrVoucherCodeLocked, got %+v %v", got, err)
	}
	if got, err := repo.GetVoucherByID(legacy.ID); err != nil || got == nil || got.Code != legacy.Code {
		t.Fatalf("plaintext rows must not depend on the key: %+v %v", got, err)
	}
	if err := ConfigureVoucherCodes(nil, true); err == nil {
		t.Fatal("protection on without a key must be refused")
	}
}

// Owner search: a complete code finds a protected row; a fragment cannot (there is no plaintext to scan), but the
// plan label and plaintext rows still search as before.
func TestDB_VoucherCodeProtection_ListSearchAndCodeFilter(t *testing.T) {
	db := requireHarness(t)
	repo := NewVoucherRepository(db)
	biz := insertBusiness(t, db)
	legacy := newActiveVoucher(t, repo, biz, 60, nil)
	useVoucherCodeProtection(t, true)
	protected := newActiveVoucher(t, repo, biz, 60, nil)

	list := func(f VoucherListFilter) map[string]bool {
		out := map[string]bool{}
		inTenant(t, db, biz, func(tx *sql.Tx) error {
			f.Limit = 50
			page, err := repo.ListVouchersPageTXN(tx, biz, f)
			if err != nil {
				t.Fatal(err)
			}
			for _, v := range page.Vouchers {
				out[v.ID] = true
				if v.Code == "" {
					t.Fatalf("listed voucher %s has no code", v.ID)
				}
			}
			return nil
		})
		return out
	}

	if got := list(VoucherListFilter{Query: strings.ToLower(protected.Code)}); !got[protected.ID] || got[legacy.ID] {
		t.Fatalf("full-code search (any case) must find exactly the protected row: %v", got)
	}
	if got := list(VoucherListFilter{Query: legacy.Code[:5]}); !got[legacy.ID] || got[protected.ID] {
		t.Fatalf("fragment search still works on plaintext rows only: %v", got)
	}
	if got := list(VoucherListFilter{Query: protected.Code[:5]}); got[protected.ID] {
		t.Fatal("a fragment must not match a protected row")
	}
	if got := list(VoucherListFilter{Codes: []string{protected.Code, legacy.Code}}); !got[protected.ID] || !got[legacy.ID] {
		t.Fatalf("code list filter (printing) must find both forms: %v", got)
	}
}

// The guest redemption path (row lock by typed code) must resolve a protected code.
func TestDB_VoucherCodeProtection_GuestReserveFindsProtectedCode(t *testing.T) {
	db := requireHarness(t)
	for _, name := range []string{"../../migrations/20261001200000_guest_voucher_grants.sql"} {
		m, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(m)); err != nil {
			t.Fatal(err)
		}
	}
	applyVoucherActivationMigration(t, db)
	useVoucherCodeProtection(t, true)
	biz := insertBusiness(t, db)
	device := insertDeviceRow(t, db, biz, nil, "protected voucher test", "MAIN_GATEWAY")
	repo := GuestVoucherRepository{}
	var v *model.Voucher
	inTenant(t, db, biz, func(tx *sql.Tx) error {
		var err error
		v, err = NewVoucherRepository(db).CreateVoucherTXN(tx, &model.Voucher{BusinessID: biz, PlanKey: "guest-daily", PlanLabel: "Guest daily", Code: "PROTECTED" + strings.ToUpper(randHex(6)), Status: "unused"})
		if err != nil {
			return err
		}
		return repo.IssueTXN(tx, biz, v.ID, device, 1, 75)
	})
	if plain, _, _ := storedCodeForm(t, db, v.ID); plain.Valid {
		t.Fatal("guest voucher kept its plaintext code")
	}
	// The guest types the code in lower case with spaces; the path upper-cases and trims it.
	typed := " " + strings.ToLower(v.Code) + " "
	var g *model.GuestVoucherGrant
	inTenant(t, db, biz, func(tx *sql.Tx) error {
		var err error
		g, err = repo.ReserveTXN(tx, biz, device, typed, "AA:BB:CC:DD:EE:01")
		return err
	})
	if g == nil || g.VoucherID != v.ID {
		t.Fatalf("protected guest code was not resolved: %+v", g)
	}
	inTenant(t, db, biz, func(tx *sql.Tx) error {
		if _, err := repo.ReserveTXN(tx, biz, device, v.Code+"Z", "AA:BB:CC:DD:EE:02"); !errors.Is(err, ErrGuestVoucherUnavailable) {
			t.Fatalf("an unknown code must stay unavailable, got %v", err)
		}
		return nil
	})
}
