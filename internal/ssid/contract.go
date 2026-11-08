// Package ssid defines fail-closed evidence requirements for site-wide SSID
// synchronization. This stage contains no network client or configuration write.
package ssid

import (
 "errors"
 "strings"
 "time"
)

type Capability string

const (
 ReadSSID Capability = "read_ssid"
 WriteSSID Capability = "write_ssid"
 VerifySSID Capability = "verify_ssid"
 RollbackSSID Capability = "rollback_ssid"
)

type Evidence struct {
 Capability Capability `json:"capability"`
 Supported bool `json:"supported"`
 Source string `json:"source,omitempty"`
 ObservedAt time.Time `json:"observed_at,omitempty"`
}

// DeviceEvidence is returned only by an authenticated, tenant-scoped adapter.
// A vendor name or a DHCP/neighbor observation is not an adapter authorization.
type DeviceEvidence struct {
 TenantID string `json:"-"`
 SiteID string `json:"site_id"`
 DeviceID string `json:"device_id"`
 Vendor string `json:"vendor,omitempty"`
 Model string `json:"model,omitempty"`
 Firmware string `json:"firmware,omitempty"`
 AdapterID string `json:"adapter_id,omitempty"`
 Authenticated bool `json:"authenticated"`
 Capabilities []Evidence `json:"capabilities"`
}

type Readiness string

const (
 Unknown Readiness = "unknown"
 ReadOnly Readiness = "read_only"
 ReadyForReview Readiness = "ready_for_review"
)

type Assessment struct {
 Readiness Readiness `json:"readiness"`
 Reason string `json:"reason"`
 // WriteAllowed is always false in this phase; the caller must implement
 // separate durable owner approval, revision fencing, rollback and readback.
 WriteAllowed bool `json:"write_allowed"`
}

var ErrInvalidScope = errors.New("ssid: tenant, site and device must be scoped")

// Assess checks independent and fresh vendor-interface evidence. "Ready for
// review" does NOT authorize applying an SSID change. Evidence from generic
// network discovery cannot manufacture API permissions.
func Assess(device DeviceEvidence, tenantID, siteID string, now time.Time) (Assessment, error) {
 if tenantID == "" || siteID == "" || device.TenantID != tenantID ||
  device.SiteID != siteID || strings.TrimSpace(device.DeviceID) == "" {
  return Assessment{}, ErrInvalidScope
 }
 unknown := Assessment{Readiness:Unknown,Reason:"No verified, fresh model-specific management evidence"}
 if !device.Authenticated || device.AdapterID == "" || device.Model == "" ||
  device.Firmware == "" || now.IsZero() {
  return unknown,nil
 }
 seen := make(map[Capability]bool)
 for _,e := range device.Capabilities {
  if !e.Supported || strings.TrimSpace(e.Source) == "" ||
   e.ObservedAt.IsZero() || e.ObservedAt.After(now.Add(time.Second)) ||
   now.Sub(e.ObservedAt) > 2*time.Minute {
   continue
  }
  seen[e.Capability] = true
 }
 if !seen[ReadSSID] { return unknown,nil }
 if !seen[WriteSSID] || !seen[VerifySSID] || !seen[RollbackSSID] {
  return Assessment{Readiness:ReadOnly,Reason:"SSID inventory verified; full write, readback and rollback capabilities unverified"},nil
 }
 return Assessment{Readiness:ReadyForReview,Reason:"Adapter capability evidence available; owner approval and preflight still required"},nil
}
