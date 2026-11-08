package routeros

import "testing"

func TestCAPsMANInventoryMakesUnknownDifferentFromZero(t *testing.T) {
	in:=NetworkInventory{
		Available:map[string]bool{
			"capsman_modern_caps":true,"capsman_modern_radios":true,
			"capsman_modern_profiles":true,
			"capsman_legacy_caps":false,
		},
		Objects:map[string][]NetworkObject{
			"capsman_modern_caps":{},
			"capsman_modern_profiles":{},
			"capsman_legacy_caps":{{
				ID:"*A",Values:map[string]string{"identity":"stale-cap"},
			}},
		},
	}
	view:=CAPsMANInventory(in)
	if len(view.Controllers)!=2 || !view.Controllers[0].InventoryAvailable ||
		len(view.Controllers[0].CAPs)!=0 || view.Controllers[1].InventoryAvailable ||
		len(view.Controllers[1].CAPs)!=0 || view.WriteEnabled {
		t.Fatalf("unavailable controller must not return stale APs: %+v",view)
	}
}

func TestCAPsMANInventoryNeverClaimsSSIDBroadcastOrReachability(t *testing.T) {
	in:=NetworkInventory{
		Available:map[string]bool{
			"capsman_modern_caps":true,"capsman_modern_profiles":true,
		},
		Objects:map[string][]NetworkObject{
			"capsman_modern_caps":{
				{ID:"*2",Values:map[string]string{"identity":"ap-b","state":"running","address":"192.168.50.2"}},
				{ID:"*1",Values:map[string]string{"identity":"ap-a","state":"unknown"}},
			},
			"capsman_modern_profiles":{
				{ID:"*3",Values:map[string]string{"name":"guest","ssid":"Shop WiFi"}},
				{ID:"*4",Values:map[string]string{"name":"staff","ssid":"Staff WiFi"}},
			},
		},
	}
	view:=CAPsMANInventory(in)
	if !view.Controllers[0].InventoryAvailable || len(view.Controllers[0].CAPs)!=2 ||
		view.Controllers[0].CAPs[0].Identity!="ap-a" || view.Controllers[0].CAPs[1].Identity!="ap-b" {
		t.Fatalf("unexpected CAP evidence: %+v",view)
	}
	for _,cap:=range view.Controllers[0].CAPs {
		if cap.OnlineVerified || cap.SSIDVerified {t.Fatalf("controller listing promoted to verified AP: %+v",cap)}
	}
	if len(view.Profiles)!=2 || view.Profiles[0].Name!="guest" ||
		view.Profiles[0].SSID!="Shop WiFi" || view.WriteEnabled {
		t.Fatalf("profile evidence incorrect: %+v",view)
	}
	for _,profile:=range view.Profiles {
		if profile.BroadcastingVerified {t.Fatalf("configured profile incorrectly asserted live broadcast: %+v",profile)}
	}
}

func TestCAPsMANInventoryRejectsIncompleteOrUnavailableRows(t *testing.T) {
	in:=NetworkInventory{
		Available:map[string]bool{"capsman_legacy_caps":true,"capsman_legacy_profiles":false},
		Objects:map[string][]NetworkObject{
			"capsman_legacy_caps":{
				{ID:"*1",Values:map[string]string{"address":"10.1.1.1"}},
				{ID:"*2",Values:map[string]string{"identity":"leg-cap"}},
			},
			"capsman_legacy_profiles":{
				{ID:"*3",Values:map[string]string{"name":"not-readable","ssid":"MustNotShow"}},
			},
		},
	}
	view:=CAPsMANInventory(in)
	if len(view.Controllers[1].CAPs)!=1 || view.Controllers[1].CAPs[0].StateKnown ||
		len(view.Profiles)!=0 {t.Fatalf("invalid or unavailable rows surfaced: %+v",view)}
}
