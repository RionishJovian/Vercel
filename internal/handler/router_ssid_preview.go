package handler

import (
 "context"
 "database/sql"
 "net/http"
 "strings"
 "time"

 "github.com/gin-gonic/gin"
 "mega-magic/internal/auth"
 "mega-magic/internal/model"
 "mega-magic/internal/ssid"
)

type ssidPreviewRequest struct {
 SiteID string `json:"site_id" binding:"required"`
 Revision string `json:"revision" binding:"required"`
 SSID string `json:"ssid" binding:"required"`
}

// PreviewSSIDChange is a server-scoped, owner-only, read-only review. A CAPsMAN
// row is inventory evidence ONLY: it never proves AP authentication, SSID
// readback, or management entitlement. No device can be applied in this phase.
func (h *RouterHandler) PreviewSSIDChange(c *gin.Context) {
 businessID, deviceID:=auth.GetBusinessID(c),c.Param("id")
 var req ssidPreviewRequest
 c.Request.Body=http.MaxBytesReader(c.Writer,c.Request.Body,4096)
 if c.ShouldBindJSON(&req)!=nil || !isUUID(req.SiteID) {
  c.JSON(http.StatusBadRequest,gin.H{"error":"Valid site, revision and Wi-Fi name are required"})
  return
 }
 var device *model.Device
 err:=h.withTenantTx(c,businessID,func(tx *sql.Tx)error{
  d,e:=h.deviceRepo.GetDeviceByIDTXN(tx,deviceID)
  if e!=nil{return e}
  device=d
  return nil
 })
 if err!=nil {respondErr(c,err);return}
 if device==nil || device.BusinessID!=businessID {
  c.JSON(http.StatusNotFound,gin.H{"error":"Gateway not found"});return
 }
 if device.DeploymentLayer!="MAIN_GATEWAY" || device.SiteID==nil || *device.SiteID!=req.SiteID {
  c.JSON(http.StatusConflict,gin.H{"error":"Gateway is not assigned to the requested site","code":"site_required"});return
 }
 if h.observations==nil {
  c.JSON(http.StatusServiceUnavailable,gin.H{"error":"Read-only inventory is not enabled"});return
 }
 ctx,cancel:=context.WithTimeout(c.Request.Context(),15*time.Second)
 defer cancel()
 inv,err:=h.observations.ReadObservations(ctx,businessID,deviceID)
 if err!=nil {
  c.JSON(http.StatusBadGateway,gin.H{"error":"Gateway inventory unavailable"});return
 }
 // Only authenticated gateway observations are available here. CAPs listed
 // inside them have no verified per-AP write or rollback credentials, so all
 // CAPs are blocked. Never promote a controller profile to live broadcast.
 items:=make([]ssid.APCurrentState,0)
 for _,key:=range []string{"capsman_modern_caps","capsman_legacy_caps"} {
  if !inv.Available[key] {continue}
  for _,row:=range inv.Objects[key] {
   identity:=strings.TrimSpace(row.Values["identity"])
   if identity=="" {continue}
   // Prefix and use RouterOS record ID; duplicate/ambiguous rows are
   // skipped by the strict preview validation rather than guessed.
   items=append(items,ssid.APCurrentState{Device:ssid.DeviceEvidence{
    TenantID:businessID,SiteID:req.SiteID,DeviceID:key+":"+row.ID,
    Vendor:"MikroTik",Model:"",Firmware:"",Authenticated:false,
   },SSIDReadVerified:false})
  }
 }
 now:=time.Now().UTC()
 preview,e:=ssid.BuildPreview(ssid.DesiredSSID{
  TenantID:businessID,SiteID:req.SiteID,Revision:req.Revision,SSID:req.SSID,
 },items,now)
 if e!=nil {c.JSON(http.StatusBadRequest,gin.H{"error":"Invalid or ambiguous SSID review"});return}
 c.Header("Cache-Control","no-store")
 c.JSON(http.StatusOK,gin.H{
  "preview":preview,"gateway_id":deviceID,"observed_at":inv.ObservedAt,
  "apply_available":false,
  "note":"CAPsMAN evidence does not authorize per-AP SSID changes; unobserved third-party APs may exist",
 })
}
