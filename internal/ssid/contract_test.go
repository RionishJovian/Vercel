package ssid

import (
 "errors"
 "testing"
 "time"
)

func evidence(now time.Time) DeviceEvidence {
 return DeviceEvidence{
  TenantID:"tenant-a",SiteID:"site-a",DeviceID:"ap-01",Vendor:"test",
  Model:"real-model",Firmware:"1.0",AdapterID:"verified-controller",Authenticated:true,
  Capabilities:[]Evidence{
   {Capability:ReadSSID,Supported:true,Source:"controller:site-a",ObservedAt:now},
   {Capability:WriteSSID,Supported:true,Source:"controller:site-a",ObservedAt:now},
   {Capability:VerifySSID,Supported:true,Source:"controller:site-a",ObservedAt:now},
   {Capability:RollbackSSID,Supported:true,Source:"controller:site-a",ObservedAt:now},
  },
 }
}

func TestAssessRequiresTenantAndSiteScope(t *testing.T) {
 now:=time.Now().UTC()
 d:=evidence(now)
 for _,tc:=range []struct{tenant,site string}{
  {"tenant-b","site-a"},{"tenant-a","site-b"},{"","site-a"},{"tenant-a",""},
 } {
  if _,err:=Assess(d,tc.tenant,tc.site,now);!errors.Is(err,ErrInvalidScope){
   t.Fatalf("scope %q/%q accepted: %v",tc.tenant,tc.site,err)
  }
 }
}

func TestAssessNeverGrantsWritesFromVendorHint(t *testing.T) {
 now:=time.Now().UTC()
 for _,vendor:=range []string{"MikroTik","Ruijie/Reyee","TP-Link","iKuai","Huawei"} {
  d:=DeviceEvidence{TenantID:"tenant-a",SiteID:"site-a",DeviceID:"ap-01",Vendor:vendor}
  got,err:=Assess(d,"tenant-a","site-a",now)
  if err!=nil||got.Readiness!=Unknown||got.WriteAllowed{
   t.Fatalf("%s granted unverified management: %+v, %v",vendor,got,err)
  }
 }
}

func TestAssessRequiresPerFeatureFreshAuthenticatedEvidence(t *testing.T) {
 now:=time.Now().UTC()
 for _,name:=range []string{"no-auth","no-firmware","stale-write","future-verify","no-rollback","blank-source"} {
  t.Run(name,func(t *testing.T){
   d:=evidence(now)
   switch name {
   case "no-auth":d.Authenticated=false
   case "no-firmware":d.Firmware=""
   case "stale-write":d.Capabilities[1].ObservedAt=now.Add(-10*time.Minute)
   case "future-verify":d.Capabilities[2].ObservedAt=now.Add(10*time.Minute)
   case "no-rollback":d.Capabilities[3].Supported=false
   case "blank-source":d.Capabilities[0].Source=""
   }
   got,err:=Assess(d,"tenant-a","site-a",now)
   if err!=nil||got.Readiness==ReadyForReview||got.WriteAllowed{
    t.Fatalf("%s created false readiness: %+v, %v",name,got,err)
   }
  })
 }
}

func TestAssessReadyForReviewStillCannotWrite(t *testing.T) {
 now:=time.Now().UTC()
 got,err:=Assess(evidence(now),"tenant-a","site-a",now)
 if err!=nil||got.Readiness!=ReadyForReview||got.WriteAllowed{
  t.Fatalf("capability evidence must never be apply approval: %+v, %v",got,err)
 }
 d:=evidence(now)
 d.Capabilities[1].Supported=false
 got,err=Assess(d,"tenant-a","site-a",now)
 if err!=nil||got.Readiness!=ReadOnly||got.WriteAllowed{
  t.Fatalf("partial supported read capability wrong: %+v, %v",got,err)
 }
}
