package database

import (
	"database/sql"
	"errors"
	"fmt"

	"mega-magic/internal/model"
)

// ErrSiteNotFound is returned when a site id does not exist inside the caller's
// business.
var ErrSiteNotFound = errors.New("site not found")

// SiteRepository reads and writes sites. Every method takes a *sql.Tx because all
// access must run inside a tenant-bound transaction (RLS context set).
type SiteRepository struct {
	db *DB
}

// NewSiteRepository creates a site repository.
func NewSiteRepository(db *DB) *SiteRepository {
	return &SiteRepository{db: db}
}

// siteColumns is shared by every read so the shape cannot drift between queries.
// latitude/longitude are cast to float8 because the columns are numeric(9,6) and
// scanning numeric directly is unreliable.
const siteColumns = `
	s.site_id, s.business_id, s.name, s.location, s.timezone,
	s.latitude::float8, s.longitude::float8, s.notes,
	COALESCE((SELECT count(*) FROM devices d WHERE d.site_id = s.site_id), 0) AS device_count,
	s.created_at, s.updated_at`

func scanSite(row interface{ Scan(...any) error }) (*model.Site, error) {
	s := &model.Site{}
	var location, timezone, notes sql.NullString
	var lat, lng sql.NullFloat64
	if err := row.Scan(
		&s.SiteID, &s.BusinessID, &s.Name, &location, &timezone,
		&lat, &lng, &notes, &s.DeviceCount, &s.CreatedAt, &s.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if location.Valid {
		s.Location = &location.String
	}
	if timezone.Valid {
		s.Timezone = &timezone.String
	}
	if lat.Valid {
		s.Latitude = &lat.Float64
	}
	if lng.Valid {
		s.Longitude = &lng.Float64
	}
	if notes.Valid {
		s.Notes = &notes.String
	}
	return s, nil
}

// ListSitesTXN returns every site in the business, alphabetically, each with the
// number of devices attached.
func (r *SiteRepository) ListSitesTXN(tx *sql.Tx, businessID string) ([]*model.Site, error) {
	rows, err := tx.Query(`SELECT `+siteColumns+` FROM sites s WHERE s.business_id = $1 ORDER BY s.name`, businessID)
	if err != nil {
		return nil, fmt.Errorf("list sites: %w", err)
	}
	defer rows.Close()

	out := []*model.Site{}
	for rows.Next() {
		s, err := scanSite(rows)
		if err != nil {
			return nil, fmt.Errorf("scan site: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sites: %w", err)
	}
	return out, nil
}

// GetSiteTXN returns one site, or ErrSiteNotFound.
func (r *SiteRepository) GetSiteTXN(tx *sql.Tx, businessID, siteID string) (*model.Site, error) {
	s, err := scanSite(tx.QueryRow(
		`SELECT `+siteColumns+` FROM sites s WHERE s.site_id = $1 AND s.business_id = $2`,
		siteID, businessID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSiteNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get site: %w", err)
	}
	return s, nil
}

// CreateSiteTXN inserts a site and returns it (with device_count = 0).
func (r *SiteRepository) CreateSiteTXN(tx *sql.Tx, businessID string, req *model.SiteRequest) (*model.Site, error) {
	var id string
	if err := tx.QueryRow(
		`INSERT INTO sites (business_id, name, location, timezone, latitude, longitude, notes)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING site_id`,
		businessID, req.Name, req.Location, req.Timezone, req.Latitude, req.Longitude, req.Notes,
	).Scan(&id); err != nil {
		return nil, fmt.Errorf("create site: %w", err)
	}
	return r.GetSiteTXN(tx, businessID, id)
}

// UpdateSiteTXN replaces a site's editable fields. Returns ErrSiteNotFound when
// the id is not in this business, so an update can never silently affect zero
// rows and look successful.
func (r *SiteRepository) UpdateSiteTXN(tx *sql.Tx, businessID, siteID string, req *model.SiteRequest) (*model.Site, error) {
	res, err := tx.Exec(
		`UPDATE sites
		    SET name = $1, location = $2, timezone = $3,
		        latitude = $4, longitude = $5, notes = $6, updated_at = now()
		  WHERE site_id = $7 AND business_id = $8`,
		req.Name, req.Location, req.Timezone, req.Latitude, req.Longitude, req.Notes, siteID, businessID)
	if err != nil {
		return nil, fmt.Errorf("update site: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrSiteNotFound
	}
	return r.GetSiteTXN(tx, businessID, siteID)
}

// DeleteSiteTXN removes a site. Devices reference a site with ON DELETE SET NULL,
// so the devices survive and simply become unassigned — deleting a site must not
// delete the routers that were placed there.
func (r *SiteRepository) DeleteSiteTXN(tx *sql.Tx, businessID, siteID string) error {
	res, err := tx.Exec(`DELETE FROM sites WHERE site_id = $1 AND business_id = $2`, siteID, businessID)
	if err != nil {
		return fmt.Errorf("delete site: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrSiteNotFound
	}
	return nil
}
