package database

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"mega-magic/internal/model"
)

// SitePortalRepository owns the site-specific portal lifecycle. The legacy
// business portal repository remains the fallback until all public routes are
// switched to site resolution.
type SitePortalRepository struct{}

func NewSitePortalRepository() *SitePortalRepository { return &SitePortalRepository{} }

const sitePortalCols = `site_id, business_id, draft_config, published_config,
    draft_revision, published_revision, published_at, created_at, updated_at`

func scanSitePortal(row interface{ Scan(...any) error }) (*model.SitePortal, error) {
	var p model.SitePortal
	var draft, published []byte
	if err := row.Scan(&p.SiteID, &p.BusinessID, &draft, &published,
		&p.DraftRevision, &p.PublishedRevision, &p.PublishedAt, &p.CreatedAt, &p.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get site portal: %w", err)
	}
	p.DraftConfig = &model.PortalConfig{}
	p.PublishedConfig = &model.PortalConfig{}
	if len(draft) > 0 && string(draft) != "{}" {
		if err := json.Unmarshal(draft, p.DraftConfig); err != nil {
			return nil, fmt.Errorf("decode site portal draft: %w", err)
		}
	}
	if len(published) > 0 && string(published) != "{}" {
		if err := json.Unmarshal(published, p.PublishedConfig); err != nil {
			return nil, fmt.Errorf("decode site portal published config: %w", err)
		}
	}
	return &p, nil
}

func (r *SitePortalRepository) GetBySiteTXN(tx *sql.Tx, businessID, siteID string) (*model.SitePortal, error) {
	return scanSitePortal(tx.QueryRow(`SELECT `+sitePortalCols+` FROM site_portals WHERE business_id = $1 AND site_id = $2`, businessID, siteID))
}

func (r *SitePortalRepository) UpsertDraftTXN(tx *sql.Tx, portal *model.SitePortal) error {
	draft, err := json.Marshal(portal.DraftConfig)
	if err != nil {
		return fmt.Errorf("encode site portal draft: %w", err)
	}
	_, err = tx.Exec(`INSERT INTO site_portals (site_id, business_id, draft_config, draft_revision, updated_at)
        VALUES ($1, $2, $3, GREATEST($4, 1), now())
        ON CONFLICT (site_id) DO UPDATE SET business_id = EXCLUDED.business_id,
            draft_config = EXCLUDED.draft_config,
            draft_revision = site_portals.draft_revision + 1,
            updated_at = now()`, portal.SiteID, portal.BusinessID, draft, portal.DraftRevision)
	if err != nil {
		return fmt.Errorf("upsert site portal draft: %w", err)
	}
	return nil
}

func (r *SitePortalRepository) PublishTXN(tx *sql.Tx, businessID, siteID string) error {
	result, err := tx.Exec(`UPDATE site_portals
        SET published_config = draft_config,
            published_revision = draft_revision,
            published_at = now(),
            updated_at = now()
        WHERE business_id = $1 AND site_id = $2`, businessID, siteID)
	if err != nil {
		return fmt.Errorf("publish site portal: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return fmt.Errorf("site portal not found: %s", siteID)
	}
	return nil
}
