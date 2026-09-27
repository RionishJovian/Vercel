package model

import "time"

// Site is a physical location a business operates, carrying the geo data ported
// from Mikromagic (location, timezone, latitude/longitude). Devices, router
// connections, daily closes and syslog events all hang off a site.
type Site struct {
	SiteID      string    `json:"site_id"`
	BusinessID  string    `json:"business_id"`
	Name        string    `json:"name"`
	Location    *string   `json:"location,omitempty"`
	Timezone    *string   `json:"timezone,omitempty"`
	Latitude    *float64  `json:"latitude,omitempty"`
	Longitude   *float64  `json:"longitude,omitempty"`
	Notes       *string   `json:"notes,omitempty"`
	DeviceCount int       `json:"device_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// HasGeo reports whether the site has usable coordinates — the UI uses this to
// decide between a map link and an empty state.
func (s *Site) HasGeo() bool {
	return s.Latitude != nil && s.Longitude != nil
}

// SiteRequest is the create/update body. Latitude/longitude are range-checked
// here so an out-of-range coordinate never reaches the CHECK constraint.
type SiteRequest struct {
	Name      string   `json:"name"      binding:"required,min=1,max=120"`
	Location  *string  `json:"location"`
	Timezone  *string  `json:"timezone"`
	Latitude  *float64 `json:"latitude"  binding:"omitempty,gte=-90,lte=90"`
	Longitude *float64 `json:"longitude" binding:"omitempty,gte=-180,lte=180"`
	Notes     *string  `json:"notes"`
}
