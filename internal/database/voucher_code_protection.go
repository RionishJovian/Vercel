package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"

	"mega-magic/internal/vouchercode"
)

// Voucher codes at rest (migration 20261114090000_voucher_code_protection.sql, internal/vouchercode).
//
// A row is in one of two forms. Legacy: code holds the plaintext. Protected: code is NULL and code_enc/code_hmac hold
// the sealed code and its blind index. Every read path below understands both, so the flag
// security.protect_voucher_codes can be switched on or off without breaking existing rows, and the backfill can run
// while the app is serving.

// ErrVoucherCodeLocked is returned when a protected code is read but no protection key is loaded. Callers must treat
// the voucher as unavailable; it is never replaced by an empty or partial code.
var ErrVoucherCodeLocked = errors.New("voucher code is protected and no voucher code key is configured")

// voucherCodeSealedPrefix marks the protected form inside the code expression. Generated codes are letters and digits
// only, so a plaintext code can never carry it.
const voucherCodeSealedPrefix = "enc:v1:"

// voucherCodeSQL replaces the bare column in every SELECT and RETURNING list: plaintext for legacy rows, the sealed
// value (tagged) for protected rows. ScanVoucherCode turns either back into the plaintext code.
const voucherCodeSQL = `COALESCE(code, 'enc:v1:' || code_enc)`

// VoucherCodeSQL is voucherCodeSQL for a table alias, for queries outside this package.
func VoucherCodeSQL(alias string) string {
	return `COALESCE(` + alias + `.code, 'enc:v1:' || ` + alias + `.code_enc)`
}

var voucherCodes struct {
	mu         sync.RWMutex
	protector  *vouchercode.Protector
	protectNew bool
}

// ConfigureVoucherCodes loads the protection key. protectNew=true makes every newly created voucher protected; with a
// key but protectNew=false the app still reads protected rows and writes plaintext, as before. protectNew without a
// key is refused so a misconfiguration can never silently store plaintext while claiming protection.
func ConfigureVoucherCodes(p *vouchercode.Protector, protectNew bool) error {
	if protectNew && p == nil {
		return vouchercode.ErrNoKey
	}
	voucherCodes.mu.Lock()
	defer voucherCodes.mu.Unlock()
	voucherCodes.protector, voucherCodes.protectNew = p, protectNew
	return nil
}

func voucherCodeState() (*vouchercode.Protector, bool) {
	voucherCodes.mu.RLock()
	defer voucherCodes.mu.RUnlock()
	return voucherCodes.protector, voucherCodes.protectNew
}

// voucherCodeColumns returns what to INSERT for a code: the plaintext column, the blind index and the sealed value.
// Exactly one form is produced, never both, so a protected row never keeps its plaintext.
func voucherCodeColumns(code string) (plain, blind, sealed sql.NullString, err error) {
	p, protectNew := voucherCodeState()
	if !protectNew {
		return sql.NullString{String: code, Valid: true}, sql.NullString{}, sql.NullString{}, nil
	}
	enc, err := p.Seal(code)
	if err != nil {
		return sql.NullString{}, sql.NullString{}, sql.NullString{}, fmt.Errorf("seal voucher code: %w", err)
	}
	return sql.NullString{}, sql.NullString{String: p.Blind(code), Valid: true}, sql.NullString{String: enc, Valid: true}, nil
}

// voucherCodeLookup returns the two values a WHERE clause compares against: the plaintext (legacy rows) and the blind
// index (protected rows). Without a key the index is "", which equals no stored value.
func voucherCodeLookup(code string) (plain, blind string) {
	p, _ := voucherCodeState()
	if p == nil {
		return code, ""
	}
	return code, p.Blind(code)
}

// VoucherCodeBlind is voucherCodeLookup's index for callers outside this package ("" without a key).
func VoucherCodeBlind(code string) string {
	_, blind := voucherCodeLookup(code)
	return blind
}

// voucherCodeScanner reads a code column produced by voucherCodeSQL into a plain string.
type voucherCodeScanner struct{ dst *string }

// ScanVoucherCode is the Scan destination for a voucherCodeSQL column: it returns the plaintext code for both forms.
func ScanVoucherCode(dst *string) sql.Scanner { return voucherCodeScanner{dst: dst} }

func (s voucherCodeScanner) Scan(src any) error {
	var raw string
	switch v := src.(type) {
	case nil:
		*s.dst = ""
		return nil
	case string:
		raw = v
	case []byte:
		raw = string(v)
	default:
		return fmt.Errorf("voucher code: unsupported column type %T", src)
	}
	if !strings.HasPrefix(raw, voucherCodeSealedPrefix) {
		*s.dst = raw
		return nil
	}
	p, _ := voucherCodeState()
	if p == nil {
		*s.dst = ""
		return ErrVoucherCodeLocked
	}
	plain, err := p.Open(strings.TrimPrefix(raw, voucherCodeSealedPrefix))
	if err != nil {
		*s.dst = ""
		return fmt.Errorf("open voucher code: %w", err)
	}
	*s.dst = plain
	return nil
}
