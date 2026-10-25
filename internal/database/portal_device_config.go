package database

import (
	"database/sql"
	"errors"

	"mega-magic/internal/model"
)

// GetByDeviceTXN resolves the policy visible on this gateway's public portal.
// Drafts do not affect guests until published. Missing/foreign devices cannot
// fall back to another tenant's business-wide configuration.
func (r *PortalConfigRepository) GetByDeviceTXN(tx *sql.Tx, businessID, deviceID string) (*model.PortalConfig, error) {
	var siteID sql.NullString
	err := tx.QueryRow(`SELECT site_id FROM devices WHERE device_id=$1 AND business_id=$2`, deviceID, businessID).Scan(&siteID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if siteID.Valid && siteID.String != "" {
		portal, err := NewSitePortalRepository().GetBySiteTXN(tx, businessID, siteID.String)
		if err != nil {
			return nil, err
		}
		if portal != nil && portal.PublishedRevision > 0 && portal.PublishedConfig != nil && portal.PublishedConfig.BusinessName != "" {
			return portal.PublishedConfig, nil
		}
	}
	return r.GetByBusinessTXN(tx, businessID)
}
