package routeros

import (
 "sort"
 "strings"
)

// CAPsMANView is a read-only projection of evidence already collected from a
// single tenant-scoped RouterOS gateway. CAP and SSID records must not be
// conflated: a configuration profile is NOT proof an AP broadcasts its SSID.
type CAPsMANView struct {
 Controllers []CAPsMANController `json:"controllers"`
 Profiles []CAPsMANProfile `json:"profiles"`
 WriteEnabled bool `json:"write_enabled"`
}

type CAPsMANController struct {
 Family string `json:"family"` // modern or legacy
 InventoryAvailable bool `json:"inventory_available"`
 RadioInventoryAvailable bool `json:"radio_inventory_available"`
 CAPs []CAPsMANCap `json:"caps"`
}

type CAPsMANCap struct {
 Identity string `json:"identity,omitempty"`
 Address string `json:"address,omitempty"`
 State string `json:"state,omitempty"`
 StateKnown bool `json:"state_known"`
 Source string `json:"source"`
 // OnlineVerified always false until a separately authenticated live probe
 // proves device reachability; a CAPsMAN row can be historical or stale.
 OnlineVerified bool `json:"online_verified"`
 SSIDVerified bool `json:"ssid_verified"`
}

type CAPsMANProfile struct {
 Family string `json:"family"`
 Name string `json:"name,omitempty"`
 SSID string `json:"ssid,omitempty"`
 Source string `json:"source"`
 // Configured SSID is not evidence of actual transmitted SSID on a radio.
 BroadcastingVerified bool `json:"broadcasting_verified"`
}

func CAPsMANInventory(inventory NetworkInventory) CAPsMANView {
 out:=CAPsMANView{Controllers:make([]CAPsMANController,0,2),Profiles:[]CAPsMANProfile{},WriteEnabled:false}
 for _,family:=range []string{"modern","legacy"} {
  capKey:="capsman_"+family+"_caps"
  radioKey:="capsman_"+family+"_radios"
  profileKey:="capsman_"+family+"_profiles"
  controller:=CAPsMANController{
   Family:family,InventoryAvailable:inventory.Available[capKey],
   RadioInventoryAvailable:inventory.Available[radioKey],CAPs:[]CAPsMANCap{},
  }
  if controller.InventoryAvailable {
   for _,row:=range inventory.Objects[capKey] {
    values:=row.Values
    identity:=strings.TrimSpace(values["identity"])
    if identity=="" {continue}
    state:=strings.TrimSpace(values["state"])
    controller.CAPs=append(controller.CAPs,CAPsMANCap{
     Identity:identity,Address:strings.TrimSpace(values["address"]),
     State:state,StateKnown:state!="",Source:capKey,
     OnlineVerified:false,SSIDVerified:false,
    })
   }
  }
  sort.Slice(controller.CAPs,func(i,j int)bool{return controller.CAPs[i].Identity<controller.CAPs[j].Identity})
  out.Controllers=append(out.Controllers,controller)
  if inventory.Available[profileKey] {
   for _,row:=range inventory.Objects[profileKey] {
    values:=row.Values
    name:=strings.TrimSpace(values["name"])
    if name=="" {continue}
    out.Profiles=append(out.Profiles,CAPsMANProfile{
     Family:family,Name:name,SSID:strings.TrimSpace(values["ssid"]),
     Source:profileKey,BroadcastingVerified:false,
    })
   }
  }
 }
 sort.Slice(out.Profiles,func(i,j int)bool{
  if out.Profiles[i].Family==out.Profiles[j].Family{return out.Profiles[i].Name<out.Profiles[j].Name}
  return out.Profiles[i].Family<out.Profiles[j].Family
 })
 return out
}
