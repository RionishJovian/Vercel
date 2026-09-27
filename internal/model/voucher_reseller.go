package model

import "time"

// CodePrefixAlphabet is the character set the ported CHECK constraint allows for
// voucher code prefixes. It omits I, O, 0 and 1 so printed voucher codes cannot be
// misread, while keeping L. This string is character-for-character identical to
// the class in voucher_resellers_code_prefix_check in database/init.sql — if the
// constraint ever changes, both must change together (the reseller behavioural
// test asserts the two agree by reading the constraint out of Postgres).
const CodePrefixAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// IsValidCodePrefix reports whether p satisfies the ported constraint:
// 2–4 characters, uppercase, drawn only from CodePrefixAlphabet.
func IsValidCodePrefix(p string) bool {
	if len(p) < 2 || len(p) > 4 {
		return false
	}
	for _, r := range p {
		ok := false
		for _, a := range CodePrefixAlphabet {
			if r == a {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

// VoucherReseller is a shop that sells vouchers on the business's behalf.
type VoucherReseller struct {
	ID          string    `json:"id"`
	BusinessID  string    `json:"business_id"`
	ShopName    string    `json:"shop_name"`
	ContactName string    `json:"contact_name"`
	Location    *string   `json:"location,omitempty"`
	Phone       *string   `json:"phone,omitempty"`
	Active      bool      `json:"active"`
	CodePrefix  string    `json:"code_prefix"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// VoucherResellerRequest is the create/update body. business_id is taken from the
// token, never the request.
type VoucherResellerRequest struct {
	ShopName    string  `json:"shop_name"    binding:"required,min=1,max=120"`
	ContactName string  `json:"contact_name" binding:"required,min=1,max=120"`
	Location    *string `json:"location"     binding:"omitempty,max=200"`
	Phone       *string `json:"phone"        binding:"omitempty,max=40"`
	Active      *bool   `json:"active"`
	CodePrefix  string  `json:"code_prefix"  binding:"required,min=2,max=4"`
}

// VoucherPrintLayout is the printed-voucher appearance for a business. The table
// is keyed by business_id — one row per business, i.e. a settings singleton, not
// a list.
type VoucherPrintLayout struct {
	BusinessID          string    `json:"business_id"`
	BusinessName        string    `json:"business_name"`
	WifiName            string    `json:"wifi_name"`
	SupportContact      string    `json:"support_contact"`
	Terms               string    `json:"terms"`
	PaperWidthMM        int       `json:"paper_width_mm"`
	ShowQR              bool      `json:"show_qr"`
	ShowPrice           bool      `json:"show_price"`
	ShowExpiry          bool      `json:"show_expiry"`
	A4Columns           int       `json:"a4_columns"`
	A4RowsPerPage       int       `json:"a4_rows_per_page"`
	ThermalColumns      int       `json:"thermal_columns"`
	ThermalRowsPerStrip int       `json:"thermal_rows_per_strip"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// VoucherPrintLayoutRequest is the upsert body.
type VoucherPrintLayoutRequest struct {
	BusinessName        string `json:"business_name"          binding:"required,min=1,max=120"`
	WifiName            string `json:"wifi_name"              binding:"required,min=1,max=120"`
	SupportContact      string `json:"support_contact"        binding:"required,min=1,max=200"`
	Terms               string `json:"terms"                  binding:"required,min=1,max=2000"`
	PaperWidthMM        int    `json:"paper_width_mm"         binding:"required,min=40,max=300"`
	ShowQR              *bool  `json:"show_qr"`
	ShowPrice           *bool  `json:"show_price"`
	ShowExpiry          *bool  `json:"show_expiry"`
	A4Columns           int    `json:"a4_columns"             binding:"required,min=1,max=8"`
	A4RowsPerPage       int    `json:"a4_rows_per_page"       binding:"required,min=1,max=40"`
	ThermalColumns      int    `json:"thermal_columns"        binding:"required,min=1,max=8"`
	ThermalRowsPerStrip int    `json:"thermal_rows_per_strip" binding:"required,min=1,max=40"`
}
