package discovery

import (
 "strings"
 "testing"

 "mega-magic/internal/provisioner/routeros"
)

func object(m map[string]string) routeros.NetworkObject {return routeros.NetworkObject{Values:m}}

func TestUniversalDiscoveryMergesEvidenceWithoutInventingLinks(t *testing.T) {
 inv:=routeros.NetworkInventory{
  ObservedAt:"2026-11-08T08:00:00Z",
  Available:map[string]bool{"interfaces":true,"arp":true,"bridge_hosts":true,"neighbors":true,"discovery_leases":true,"wifi_clients":true,"hotspot_sessions":false},
  Objects:map[string][]routeros.NetworkObject{
   "interfaces":{object(map[string]string{"name":"ether2","mac-address":"02:00:00:00:00:01"})},
   "arp":{object(map[string]string{"address":"192.168.88.50","mac-address":"B8:27:EB:AA:BB:CC","interface":"bridge"})},
   "bridge_hosts":{object(map[string]string{"mac-address":"B8:27:EB:AA:BB:CC","on-interface":"ether2","local":"false"})},
   "neighbors":{object(map[string]string{"address":"192.168.88.50","mac-address":"B8:27:EB:AA:BB:CC","identity":"shop-ap","platform":"iKuai","board":"IK-X","interface":"ether2"})},
   "discovery_leases":{object(map[string]string{"address":"192.168.88.50","mac-address":"B8:27:EB:AA:BB:CC","host-name":"shop-device"})},
   "wifi_clients":{object(map[string]string{"mac-address":"02:00:00:00:00:02","interface":"wifi1","signal":"-60"})},
  },
 }
 got:=Build("router-one",inv,[]Label{{MAC:"B8:27:EB:AA:BB:CC",Name:"Lobby AP",Kind:"ap"}})
 if !got.ReadsOnly||got.PhysicalLinksVerified||!got.Partial||got.Truncated{t.Fatalf("inaccurate inventory posture: %+v",got)}
 if len(got.Devices)!=2 {t.Fatalf("expected a merged AP and wifi client: %+v",got.Devices)}
 ap:=got.Devices[1]
 if ap.MAC!="B8:27:EB:AA:BB:CC"{t.Fatalf("unrecognized AP: %+v",ap)}
 if ap.ReportedVendor!="iKuai"||ap.ReportedModel!="IK-X"||ap.Kind!="ap"||
  ap.KindEvidence!="owner_label_unverified"||ap.Name!="Lobby AP"{
  t.Fatalf("model/vendor or owner label evidence lost: %+v",ap)
 }
 if len(ap.Addresses)!=1||ap.Addresses[0]!="192.168.88.50"||len(ap.Attachments)!=2{
  t.Fatalf("failed to merge IP and port evidence: %+v",ap)
 }
 for _,edge:=range ap.Attachments{if edge.DirectPhysicalLinkVerified{t.Fatal("fabricated direct physical link")}}
 if ap.OnlineVerified{t.Fatal("neighbor sighting isn't a current reachability check")}
 for _,cap:=range ap.Capabilities {
  if cap.Name!="gateway_observation"&&cap.Status!="unknown"{
   t.Fatalf("unverified management advertised: %+v",cap)
  }
 }
 if got.Devices[0].Kind!="unknown"||!got.Devices[0].PossiblyRandomizedMAC{
  t.Fatalf("locally administered client MAC not flagged: %+v",got.Devices[0])
 }
}

func TestDiscoveryRejectsGatewayMACMulticastAndSourceOutage(t *testing.T){
 inv:=routeros.NetworkInventory{Available:map[string]bool{"interfaces":true,"neighbors":true,"arp":false},
  Objects:map[string][]routeros.NetworkObject{
   "interfaces":{object(map[string]string{"mac-address":"02:00:00:00:00:01"})},
   "neighbors":{
    object(map[string]string{"mac-address":"02:00:00:00:00:01","identity":"gateway"}),
    object(map[string]string{"mac-address":"01:00:5e:00:00:01","identity":"multicast"}),
    object(map[string]string{"mac-address":"00:00:00:00:00:00","identity":"zero"}),
    object(map[string]string{"mac-address":"not-a-mac","identity":"invalid"}),
   },
   "arp":{object(map[string]string{"mac-address":"AA:BB:CC:11:22:33","address":"192.168.88.42"})},
  },
 }
 got:=Build("gw",inv,nil)
 if len(got.Devices)!=0||!got.Partial{t.Fatalf("unavailable source was trusted or router MAC leaked: %+v",got)}
 for _,status:=range got.Sources{if status.Name=="arp"&&(status.Available||status.Count!=0){t.Fatalf("failed source exposed stale data: %+v",status)}}
}

func TestDiscoveryBoundedAndLabelDoesNotGrantManageability(t *testing.T){
 entries:=make([]routeros.NetworkObject,0,700)
 for i:=0;i<700;i++{entries=append(entries,object(map[string]string{"mac-address":macForTest(i),"address":"10.0.0.2"}))}
 inv:=routeros.NetworkInventory{Available:map[string]bool{"arp":true},Objects:map[string][]routeros.NetworkObject{"arp":entries}}
 got:=Build("gw",inv,[]Label{{MAC:macForTest(0),Name:"Ruijie AP",Kind:"ap"}})
 if len(got.Devices)!=maxDevices||!got.Truncated{t.Fatalf("unbounded result: %d, %v",len(got.Devices),got.Truncated)}
 for _,d:=range got.Devices{
  if d.Kind=="ap" {
   if d.ReportedVendor!=""||d.ReportedModel!=""||d.OnlineVerified{
    t.Fatalf("owner label claimed a verified vendor/online state: %+v",d)
   }
   for _,cap:=range d.Capabilities{if cap.Name=="ssid_configuration"&&cap.Status!="unknown"{t.Fatal("label granted unverified AP control")}}
  }
 }
}

func macForTest(i int)string{
 const digits="0123456789ABCDEF"
 return "A0:10:20:30:"+string([]byte{digits[(i>>8)&15],digits[(i>>4)&15]})+":"+string([]byte{digits[i&15],'0'})
}

func TestVendorHintsAreReportsNotClaims(t *testing.T){
 for _,tc:=range []struct{board,want string}{
  {"RG-RAP - Ruijie Reyee","Ruijie/Reyee"},
  {"iKuai Access Point","iKuai"},
  {"Huawei AP","Huawei"},
  {"Omada EAP","TP-Link"},
  {"unknown-firmware",""},
 }{
  got:=vendorHint("",tc.board)
  if got!=tc.want{t.Fatalf("%s => %q wanted %q",tc.board,got,tc.want)}
 }
 if normalizeMAC(strings.Repeat("F",12))!="" {t.Fatal("broadcast MAC was accepted")}
}
