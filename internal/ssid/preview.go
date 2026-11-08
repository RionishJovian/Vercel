package ssid

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "sort"
 "strings"
 "time"
 "unicode/utf8"
)

var ErrInvalidPreview = errors.New("ssid: invalid site-wide SSID preview")

// DesiredSSID is a read-only proposed revision, not a saved configuration or
// an instruction to apply changes. Network/security settings remain untouched.
type DesiredSSID struct {
 TenantID string `json:"-"`
 SiteID string `json:"site_id"`
 Revision string `json:"revision"`
 SSID string `json:"ssid"`
}

type APCurrentState struct {
 Device DeviceEvidence `json:"device"`
 SSID string `json:"ssid,omitempty"`
 SSIDReadVerified bool `json:"ssid_read_verified"`
 SecurityCompatible bool `json:"security_compatible"`
 VLANCompatible bool `json:"vlan_compatible"`
 HotspotCompatible bool `json:"hotspot_compatible"`
}

type APImpact struct {
 DeviceID string `json:"device_id"`
 Vendor string `json:"vendor,omitempty"`
 CurrentSSID string `json:"current_ssid,omitempty"`
 TargetSSID string `json:"target_ssid"`
 Status string `json:"status"` // unchanged | review_candidate | blocked
 Reason string `json:"reason"`
}

type Preview struct {
 SiteID string `json:"site_id"`
 Revision string `json:"revision"`
 SSID string `json:"ssid"`
 ReviewHash string `json:"review_hash"`
 Impacts []APImpact `json:"impacts"`
 EligibleCount int `json:"eligible_count"`
 BlockedCount int `json:"blocked_count"`
 WriteEnabled bool `json:"write_enabled"`
}

// BuildPreview is intentionally pure. It never grants write permissions, and
// each AP must have authenticated fresh capability evidence + compatible guest
// security/VLAN/HotSpot. Absent information is blocked, never guessed.
func BuildPreview(desired DesiredSSID, devices []APCurrentState, now time.Time) (Preview,error) {
 if desired.TenantID=="" || desired.SiteID=="" || desired.Revision=="" ||
  strings.TrimSpace(desired.Revision)!=desired.Revision || len(desired.Revision)>128 ||
  !utf8.ValidString(desired.SSID) || len(desired.SSID)<1 || len(desired.SSID)>32 ||
  strings.TrimSpace(desired.SSID)!=desired.SSID || strings.ContainsAny(desired.SSID,"\x00\r\n") ||
  len(devices)>512 || now.IsZero() {
  return Preview{},ErrInvalidPreview
 }
 out:=Preview{SiteID:desired.SiteID,Revision:desired.Revision,SSID:desired.SSID,
  Impacts:make([]APImpact,0,len(devices)),WriteEnabled:false}
 seen:=make(map[string]bool,len(devices))
 for _,item:=range devices {
  assessment,err:=Assess(item.Device,desired.TenantID,desired.SiteID,now)
  if err!=nil || seen[item.Device.DeviceID] {
   return Preview{},ErrInvalidPreview
  }
  seen[item.Device.DeviceID]=true
  impact:=APImpact{DeviceID:item.Device.DeviceID,Vendor:item.Device.Vendor,
   CurrentSSID:item.SSID,TargetSSID:desired.SSID,Status:"blocked"}
  switch {
  case !item.SSIDReadVerified:
   impact.Reason="Current SSID has no authenticated readback"
  case assessment.Readiness!=ReadyForReview:
   impact.Reason=assessment.Reason
  case !item.SecurityCompatible || !item.VLANCompatible || !item.HotspotCompatible:
   impact.Reason="Security, VLAN or HotSpot compatibility remains unverified"
  case item.SSID==desired.SSID:
   impact.Status="unchanged"
   impact.Reason="Verified current SSID matches proposed name"
  default:
   impact.Status="review_candidate"
   impact.Reason="Potential client interruption; owner review and live preflight required before any apply"
   out.EligibleCount++
  }
  if impact.Status=="blocked" {out.BlockedCount++}
  out.Impacts=append(out.Impacts,impact)
 }
 sort.Slice(out.Impacts,func(i,j int)bool{return out.Impacts[i].DeviceID<out.Impacts[j].DeviceID})
 serialized,err:=json.Marshal(struct{
  TenantID string
  SiteID string
  Revision string
  SSID string
  Impacts []APImpact
 }{desired.TenantID,out.SiteID,out.Revision,out.SSID,out.Impacts})
 if err!=nil {return Preview{},ErrInvalidPreview}
 sum:=sha256.Sum256(serialized)
 out.ReviewHash=hex.EncodeToString(sum[:])
 return out,nil
}
