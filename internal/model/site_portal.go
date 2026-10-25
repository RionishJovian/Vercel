package model

import "time"

// SitePortal stores the site-specific draft and published portal experience.
// Config is kept as a typed PortalConfig in Go while the database retains a
// JSON document so adding future portal fields does not require a destructive
// schema change. BusinessID is repeated for tenant/RLS enforcement.
type SitePortal struct {
	SiteID            string        `json:"site_id"`
	BusinessID        string        `json:"business_id"`
	DraftConfig       *PortalConfig `json:"draft_config"`
	PublishedConfig   *PortalConfig `json:"published_config"`
	DraftRevision     int64         `json:"draft_revision"`
	PublishedRevision int64         `json:"published_revision"`
	PublishedAt       *time.Time    `json:"published_at,omitempty"`
	CreatedAt         time.Time     `json:"created_at"`
	UpdatedAt         time.Time     `json:"updated_at"`
}
