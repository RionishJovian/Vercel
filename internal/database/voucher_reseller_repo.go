package database

import (
	"database/sql"
	"errors"
	"fmt"

	"mega-magic/internal/model"
)

// ErrResellerNotFound is returned when a reseller id is not in the caller's business.
var ErrResellerAssigned = errors.New("reseller has assigned voucher batches; reassign or unassign them before deletion")
var ErrResellerNotFound = errors.New("voucher reseller not found")

// ErrResellerPrefixInUse is returned when a prefix is already claimed in this
// business.
//
// Two shops sharing a prefix would mint indistinguishable voucher codes, so the
// rule matters. It is enforced twice on purpose: the table now carries
// `voucher_resellers_business_prefix_unique` (init.sql), and this repository
// pre-checks so the caller gets a clean 409 rather than a driver error. The
// DB-level violation is ALSO mapped here (see the INSERT), because a pre-check
// cannot survive two concurrent creates — without that mapping the loser of the
// race would surface as a 500.
var ErrResellerPrefixInUse = errors.New("code prefix already used by another reseller")

// VoucherResellerRepository manages resellers and the printed-voucher layout.
type VoucherResellerRepository struct {
	db *DB
}

// NewVoucherResellerRepository creates a voucher reseller repository.
func NewVoucherResellerRepository(db *DB) *VoucherResellerRepository {
	return &VoucherResellerRepository{db: db}
}

const resellerColumns = `
	r.id, r.business_id, r.shop_name, r.contact_name, r.location, r.phone,
	r.active, r.code_prefix, r.created_at, r.updated_at`

func scanReseller(row interface{ Scan(...any) error }) (*model.VoucherReseller, error) {
	v := &model.VoucherReseller{}
	var location, phone sql.NullString
	if err := row.Scan(&v.ID, &v.BusinessID, &v.ShopName, &v.ContactName, &location, &phone,
		&v.Active, &v.CodePrefix, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return nil, err
	}
	if location.Valid {
		v.Location = &location.String
	}
	if phone.Valid {
		v.Phone = &phone.String
	}
	return v, nil
}

// ListResellersTXN returns the business's resellers, active first.
func (r *VoucherResellerRepository) ListResellersTXN(tx *sql.Tx, businessID string) ([]*model.VoucherReseller, error) {
	rows, err := tx.Query(`SELECT `+resellerColumns+` FROM voucher_resellers r
		 WHERE r.business_id = $1 ORDER BY r.active DESC, r.shop_name`, businessID)
	if err != nil {
		return nil, fmt.Errorf("list resellers: %w", err)
	}
	defer rows.Close()

	out := []*model.VoucherReseller{}
	for rows.Next() {
		v, err := scanReseller(rows)
		if err != nil {
			return nil, fmt.Errorf("scan reseller: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// GetResellerTXN returns one reseller or ErrResellerNotFound.
func (r *VoucherResellerRepository) GetResellerTXN(tx *sql.Tx, businessID, id string) (*model.VoucherReseller, error) {
	v, err := scanReseller(tx.QueryRow(
		`SELECT `+resellerColumns+` FROM voucher_resellers r WHERE r.id = $1 AND r.business_id = $2`,
		id, businessID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrResellerNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get reseller: %w", err)
	}
	return v, nil
}

// prefixTakenTXN reports whether another reseller in this business owns prefix,
// ignoring excludeID (so an update can keep its own prefix).
func (r *VoucherResellerRepository) prefixTakenTXN(tx *sql.Tx, businessID, prefix, excludeID string) (bool, error) {
	var n int
	err := tx.QueryRow(`
		SELECT count(*) FROM voucher_resellers
		 WHERE business_id = $1 AND upper(code_prefix) = upper($2) AND id <> $3`,
		businessID, prefix, excludeID).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("check prefix: %w", err)
	}
	return n > 0, nil
}

// CreateResellerTXN adds a reseller, refusing a prefix already in use.
func (r *VoucherResellerRepository) CreateResellerTXN(tx *sql.Tx, businessID string, req *model.VoucherResellerRequest) (*model.VoucherReseller, error) {
	taken, err := r.prefixTakenTXN(tx, businessID, req.CodePrefix, "00000000-0000-0000-0000-000000000000")
	if err != nil {
		return nil, err
	}
	if taken {
		return nil, ErrResellerPrefixInUse
	}

	active := true
	if req.Active != nil {
		active = *req.Active
	}

	var id string
	if err := tx.QueryRow(`
		INSERT INTO voucher_resellers
		  (business_id, shop_name, contact_name, location, phone, active, code_prefix)
		VALUES ($1, $2, $3, $4, $5, $6, upper($7))
		RETURNING id`,
		businessID, req.ShopName, req.ContactName, req.Location, req.Phone, active, req.CodePrefix,
	).Scan(&id); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrResellerPrefixInUse
		}
		return nil, fmt.Errorf("create reseller: %w", err)
	}
	return r.GetResellerTXN(tx, businessID, id)
}

// UpdateResellerTXN edits a reseller, refusing a prefix owned by someone else.
func (r *VoucherResellerRepository) UpdateResellerTXN(tx *sql.Tx, businessID, id string, req *model.VoucherResellerRequest) (*model.VoucherReseller, error) {
	existing, err := r.GetResellerTXN(tx, businessID, id)
	if err != nil {
		return nil, err
	}
	taken, err := r.prefixTakenTXN(tx, businessID, req.CodePrefix, id)
	if err != nil {
		return nil, err
	}
	if taken {
		return nil, ErrResellerPrefixInUse
	}

	active := existing.Active
	if req.Active != nil {
		active = *req.Active
	}

	res, err := tx.Exec(`
		UPDATE voucher_resellers
		   SET shop_name = $1, contact_name = $2, location = $3, phone = $4,
		       active = $5, code_prefix = upper($6), updated_at = now()
		 WHERE id = $7 AND business_id = $8`,
		req.ShopName, req.ContactName, req.Location, req.Phone, active, req.CodePrefix, id, businessID)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrResellerPrefixInUse
		}
		return nil, fmt.Errorf("update reseller: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrResellerNotFound
	}
	return r.GetResellerTXN(tx, businessID, id)
}

// DeleteResellerTXN removes a reseller.
func (r *VoucherResellerRepository) DeleteResellerTXN(tx *sql.Tx, businessID, id string) error {
	res, err := tx.Exec(`DELETE FROM voucher_resellers WHERE id = $1 AND business_id = $2`, id, businessID)
	if err != nil {
		if isForeignKeyViolation(err) {
			return ErrResellerAssigned
		}
		return fmt.Errorf("delete reseller: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrResellerNotFound
	}
	return nil
}

// BusinessNameTXN returns the company name, used to seed a default print layout.
func (r *VoucherResellerRepository) BusinessNameTXN(tx *sql.Tx, businessID string) (string, error) {
	var name string
	err := tx.QueryRow(`SELECT company_name FROM businesses WHERE business_id = $1`, businessID).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("business name: %w", err)
	}
	return name, nil
}

const layoutColumns = `
	l.business_id, l.business_name, l.wifi_name, l.support_contact, l.terms,
	l.paper_width_mm, l.show_qr, l.show_price, l.show_expiry,
	l.a4_columns, l.a4_rows_per_page, l.thermal_columns, l.thermal_rows_per_strip,
	l.created_at, l.updated_at`

// GetPrintLayoutTXN returns the layout, or (nil, nil) when the business has never
// saved one. A read never creates a row.
func (r *VoucherResellerRepository) GetPrintLayoutTXN(tx *sql.Tx, businessID string) (*model.VoucherPrintLayout, error) {
	l := &model.VoucherPrintLayout{}
	err := tx.QueryRow(`SELECT `+layoutColumns+` FROM voucher_print_layouts l
		 WHERE l.business_id = $1`, businessID).Scan(
		&l.BusinessID, &l.BusinessName, &l.WifiName, &l.SupportContact, &l.Terms,
		&l.PaperWidthMM, &l.ShowQR, &l.ShowPrice, &l.ShowExpiry,
		&l.A4Columns, &l.A4RowsPerPage, &l.ThermalColumns, &l.ThermalRowsPerStrip,
		&l.CreatedAt, &l.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get print layout: %w", err)
	}
	return l, nil
}

// UpsertPrintLayoutTXN saves the singleton layout for a business.
func (r *VoucherResellerRepository) UpsertPrintLayoutTXN(tx *sql.Tx, businessID string, req *model.VoucherPrintLayoutRequest) (*model.VoucherPrintLayout, error) {
	showQR, showPrice, showExpiry := true, true, true
	if req.ShowQR != nil {
		showQR = *req.ShowQR
	}
	if req.ShowPrice != nil {
		showPrice = *req.ShowPrice
	}
	if req.ShowExpiry != nil {
		showExpiry = *req.ShowExpiry
	}

	_, err := tx.Exec(`
		INSERT INTO voucher_print_layouts
		  (business_id, business_name, wifi_name, support_contact, terms,
		   paper_width_mm, show_qr, show_price, show_expiry,
		   a4_columns, a4_rows_per_page, thermal_columns, thermal_rows_per_strip)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (business_id) DO UPDATE SET
		   business_name = EXCLUDED.business_name,
		   wifi_name = EXCLUDED.wifi_name,
		   support_contact = EXCLUDED.support_contact,
		   terms = EXCLUDED.terms,
		   paper_width_mm = EXCLUDED.paper_width_mm,
		   show_qr = EXCLUDED.show_qr,
		   show_price = EXCLUDED.show_price,
		   show_expiry = EXCLUDED.show_expiry,
		   a4_columns = EXCLUDED.a4_columns,
		   a4_rows_per_page = EXCLUDED.a4_rows_per_page,
		   thermal_columns = EXCLUDED.thermal_columns,
		   thermal_rows_per_strip = EXCLUDED.thermal_rows_per_strip,
		   updated_at = now()`,
		businessID, req.BusinessName, req.WifiName, req.SupportContact, req.Terms,
		req.PaperWidthMM, showQR, showPrice, showExpiry,
		req.A4Columns, req.A4RowsPerPage, req.ThermalColumns, req.ThermalRowsPerStrip)
	if err != nil {
		return nil, fmt.Errorf("upsert print layout: %w", err)
	}
	return r.GetPrintLayoutTXN(tx, businessID)
}

func isForeignKeyViolation(err error) bool {
	type sqlState interface{ SQLState() string }
	var state sqlState
	return errors.As(err, &state) && state.SQLState() == "23503"
}
