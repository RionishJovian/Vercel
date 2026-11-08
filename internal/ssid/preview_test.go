package ssid

import (
 "errors"
 "testing"
 "time"
)

func previewAP(now time.Time,id,ssid string) APCurrentState {
 d:=evidence(now)
 d.DeviceID=id
 return APCurrentState{Device:d,SSID:ssid,SSIDReadVerified:true,
  SecurityCompatible:true,VLANCompatible:true,HotspotCompatible:true}
}

func TestBuildPreviewDeterministicAndNoWrites(t *testing.T) {
 now:=time.Now().UTC()
 desired:=DesiredSSID{TenantID:"tenant-a",SiteID:"site-a",Revision:"v1",SSID:"Shop Guest"}
 a:=previewAP(now,"ap-1","Old Guest")
 b:=previewAP(now,"ap-2","Shop Guest")
 c:=previewAP(now,"ap-3","Other Guest")
 c.Device.Authenticated=false
 one,err:=BuildPreview(desired,[]APCurrentState{c,b,a},now)
 if err!=nil {t.Fatal(err)}
 if one.EligibleCount!=1 || one.BlockedCount!=1 || one.WriteEnabled ||
  len(one.Impacts)!=3 || one.Impacts[0].Status!="review_candidate" ||
  one.Impacts[1].Status!="unchanged" || one.Impacts[2].Status!="blocked" {
  t.Fatalf("unexpected preview: %+v",one)
 }
 two,err:=BuildPreview(desired,[]APCurrentState{a,b,c},now)
 if err!=nil || one.ReviewHash!=two.ReviewHash || one.ReviewHash=="" {
  t.Fatalf("review digest not stable across input ordering: %v / %+v",err,two)
 }
 desired.Revision="v2"
 three,err:=BuildPreview(desired,[]APCurrentState{a,b,c},now)
 if err!=nil || three.ReviewHash==one.ReviewHash {t.Fatal("revision must fence preview hash")}
}

func TestBuildPreviewNeverTreatsMissingCompatibilityAsSafe(t *testing.T) {
 now:=time.Now().UTC()
 desired:=DesiredSSID{TenantID:"tenant-a",SiteID:"site-a",Revision:"42",SSID:"Shop Guest"}
 for _,name:=range []string{"no-ssid-read","no-writer","security-unknown","vlan-unknown","hotspot-unknown"} {
  t.Run(name,func(t *testing.T){
   ap:=previewAP(now,"ap-1","Old")
   switch name {
   case "no-ssid-read": ap.SSIDReadVerified=false
   case "no-writer": ap.Device.Capabilities[1].Supported=false
   case "security-unknown":ap.SecurityCompatible=false
   case "vlan-unknown":ap.VLANCompatible=false
   case "hotspot-unknown":ap.HotspotCompatible=false
   }
   p,err:=BuildPreview(desired,[]APCurrentState{ap},now)
   if err!=nil || p.BlockedCount!=1 || p.EligibleCount!=0 ||
    p.Impacts[0].Status!="blocked" || p.WriteEnabled {
    t.Fatalf("%s incorrectly safe: %+v %v",name,p,err)
   }
  })
 }
}

func TestBuildPreviewRejectsCrossTenantDuplicateAndInvalidSSID(t *testing.T) {
 now:=time.Now().UTC()
 desired:=DesiredSSID{TenantID:"tenant-a",SiteID:"site-a",Revision:"v1",SSID:"Shop Guest"}
 ap:=previewAP(now,"ap-1","Old")
 for _,scenario:=range []string{"cross-tenant","cross-site","duplicate","too-long","newline","empty-revision","invalid-utf8"} {
  t.Run(scenario,func(t *testing.T){
   d,a,aps:=desired,ap,[]APCurrentState{ap}
   switch scenario {
   case "cross-tenant": a.Device.TenantID="tenant-b";aps=[]APCurrentState{a}
   case "cross-site":a.Device.SiteID="site-b";aps=[]APCurrentState{a}
   case "duplicate":aps=[]APCurrentState{ap,ap}
   case "too-long":d.SSID="abcdefghijklmnopqrstuvwxyzABCDEFG"
   case "newline":d.SSID="bad\nname"
   case "empty-revision":d.Revision=""
   case "invalid-utf8":d.SSID=string([]byte{0xff})
   }
   result,err:=BuildPreview(d,aps,now)
   if !errors.Is(err,ErrInvalidPreview) || result.ReviewHash!="" {
    t.Fatalf("%s accepted unsafe preview: %+v %v",scenario,result,err)
   }
  })
 }
}
