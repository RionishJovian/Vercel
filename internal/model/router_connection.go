package model

import "time"

// RouterConnection is a saved management connection (cloud/hub) to a router
// fleet, ported from Mikromagic. Devices attach to one via
// devices.router_connection_id, and a connection may be bound to a site.
//
// SECURITY: the ported `legacy_password_ciphertext` column is NEVER exposed in
// JSON. It is Supabase-encrypted with the old stack's key, so it cannot be
// decrypted here and is useless as a credential. The API reports only whether
// one is on file (CredentialsNeedReentry) so the UI can prompt for re-entry.
type RouterConnection struct {
	ID             string  `json:"id"`
	BusinessID     string  `json:"business_id"`
	Name           string  `json:"name"`
	Host           *string `json:"host,omitempty"`
	Port           *int    `json:"port,omitempty"`
	UseTLS         bool    `json:"use_tls"`
	IsDefault      bool    `json:"is_default"`
	SiteID         *string `json:"site_id,omitempty"`
	SiteName       *string `json:"site_name,omitempty"`
	ConnectionMode *string `json:"connection_mode,omitempty"`
	// CloudStatus is reported by the cloud handshake, not set by clients.
	CloudStatus     *string    `json:"cloud_status,omitempty"`
	CloudLastSeenAt *time.Time `json:"cloud_last_seen_at,omitempty"`
	Environment     *string    `json:"environment,omitempty"`
	DeviceCount     int        `json:"device_count"`

	// CredentialsNeedReentry is true when an unusable legacy ciphertext is on
	// file — the UI must ask the operator to re-enter the password.
	CredentialsNeedReentry bool `json:"credentials_need_reentry"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// RouterConnectionRequest is the create/update body. cloud_status and
// cloud_last_seen_at are deliberately absent: they belong to the cloud
// handshake, so a client must not be able to assert a connection is "online".
type RouterConnectionRequest struct {
	Name           string  `json:"name" binding:"required,min=1,max=120"`
	Host           *string `json:"host" binding:"omitempty,max=255"`
	Port           *int    `json:"port" binding:"omitempty,gte=1,lte=65535"`
	UseTLS         *bool   `json:"use_tls"`
	IsDefault      *bool   `json:"is_default"`
	SiteID         *string `json:"site_id" binding:"omitempty,uuid4"`
	ConnectionMode *string `json:"connection_mode" binding:"omitempty,max=32"`
	Environment    *string `json:"environment" binding:"omitempty,max=32"`
}
