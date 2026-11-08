package discovery

import (
 "reflect"
 "testing"
 "time"

 "mega-magic/internal/provisioner/routeros"
)

func TestTrustedExternalInventoryMergesByMACWithoutInventingManagement(t *testing.T){
 now:=time.Now().UTC()
 base:=Build("gateway-1",routeros.NetworkInventory{
  Available:map[string]bool{"arp":true},
  Objects:map[string][]routeros.NetworkObject{
   "arp":{{Values:map[string]string{"mac-address":"A0:10:20:30:40:50","address":"192.168.88.10"}}},
  },
 },nil)
 report:=ExternalReport{
  Source:"controller:site-1",GatewayID:"gateway-1",Verified:true,ObservedAt:now,
  Devices:[]ExternalDevice{
   {MAC:"A0:10:20:30:40:50",Name:"Known AP",ReportedVendor:"iKuai",ReportedModel:"AP-v1"},
   {MAC:"B0:10:20:30:40:51",Name:"Another AP",ReportedVendor:"Ruijie",ReportedModel:"RG-RAP"},
  },
 }
 combined,err:=MergeVerifiedInventory(base,report,now)
 if err!=nil{t.Fatal(err)}
 if len(combined.Devices)!=2 ||len(base.Devices)!=1 {t.Fatalf("merge missed a registered AP or mutated base: %+v",combined)}
 if combined.Devices[0].MAC!="A0:10:20:30:40:50"||combined.Devices[0].ReportedVendor!="iKuai"{
  t.Fatalf("MAC-based model evidence not merged: %+v",combined.Devices[0])
 }
 for _,device:=range combined.Devices {
  if device.OnlineVerified||len(device.Attachments)!=0 && device.MAC=="B0:10:20:30:40:51"{
   t.Fatalf("controller inventory invented online state or physical cable: %+v",device)
  }
  for _,cap:=range device.Capabilities {
   if cap.Name=="ssid_configuration"||cap.Name=="radio_optimization"||cap.Name=="third_party_management"{
    if cap.Status!="unknown"{t.Fatal("controller listing falsely granted write capability")}
   }
  }
 }
 again,err:=MergeVerifiedInventory(combined,report,now)
 if err!=nil{t.Fatal(err)}
 if len(again.Devices)!=2||len(again.Devices[0].Sources)!=len(combined.Devices[0].Sources){
  t.Fatal("repeated controller inventory duplicated a device or source")
 }
}

func TestExternalInventoryRefusesUntrustedSourceWithoutPartialMutation(t *testing.T){
 now:=time.Now().UTC()
 base:=Build("gateway-1",routeros.NetworkInventory{
  Available:map[string]bool{"arp":true},
  Objects:map[string][]routeros.NetworkObject{"arp":{{Values:map[string]string{
   "mac-address":"A0:10:20:30:40:50","address":"192.168.88.10",
  }}}},
 },nil)
 original:=append([]Device(nil),base.Devices...)
 good:=ExternalReport{
  Source:"controller:site-1",GatewayID:"gateway-1",Verified:true,ObservedAt:now,
  Devices:[]ExternalDevice{
   {MAC:"A0:10:20:30:40:50",ReportedVendor:"iKuai"},
   {MAC:"A0:10:20:30:40:50",ReportedVendor:"Ruijie"},
  },
 }
 bad:=[]ExternalReport{
  good,
  {Source:good.Source,GatewayID:good.GatewayID,ObservedAt:now,Devices:good.Devices},
  {Source:"controller:site-1",GatewayID:"other-gateway",Verified:true,ObservedAt:now,Devices:good.Devices[:1]},
  {Source:"untrusted:site-1",GatewayID:"gateway-1",Verified:true,ObservedAt:now,Devices:good.Devices[:1]},
  {Source:good.Source,GatewayID:"gateway-1",Verified:true,ObservedAt:now.Add(-10*time.Minute),Devices:good.Devices[:1]},
 }
 for i,rep:=range bad {
  if _,err:=MergeVerifiedInventory(base,rep,now);err==nil{t.Fatalf("untrusted report %d merged",i)}
  if !reflect.DeepEqual(base.Devices,original){t.Fatal("failed external merge changed the gateway snapshot")}
 }
}
