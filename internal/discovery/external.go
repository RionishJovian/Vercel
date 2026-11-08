package discovery

import (
	"errors"
	"sort"
	"strings"
	"time"
)

// ExternalReport is a trusted source adapter's read-only inventory, e.g. a
// tenant-enrolled AP controller. It is intentionally not HTTP-request data.
// Adapter authentication, tenant/site scoping and network allowlists must be
// proven by the calling server before it supplies Verified=true.
type ExternalReport struct {
	Source string
	GatewayID string
	ObservedAt time.Time
	Verified bool
	Devices []ExternalDevice
}

type ExternalDevice struct {
	MAC string
	Name string
	ReportedVendor string
	ReportedModel string
}

// MergeVerifiedInventory enriches a gateway observation snapshot with a
// verified controller's device inventory. A matching MAC is the ONLY join key:
// hostnames and management IPs are not durable physical identities. Unknown
// controller identities are included but not marked online or writable.
func MergeVerifiedInventory(snapshot Snapshot, report ExternalReport, now time.Time) (Snapshot,error) {
	if !snapshot.ReadsOnly || snapshot.GatewayID == "" ||
		!report.Verified || report.GatewayID != snapshot.GatewayID ||
		report.Source == "" || len(report.Source)>96 ||
		report.ObservedAt.IsZero() || report.ObservedAt.After(now.Add(time.Second)) ||
		now.Sub(report.ObservedAt)>2*time.Minute ||
		len(report.Devices)>maxDevices {
		return snapshot, errors.New("discovery: unverified, stale or mis-scoped controller evidence")
	}
	// Accept only installed backend provider namespaces, not arbitrary
	// controller/body strings masquerading as verified capability probes.
	if !strings.HasPrefix(report.Source,"controller:") ||
		len(report.Source)<=len("controller:") {
		return snapshot,errors.New("discovery: unsupported controller provenance")
	}
	idx:=make(map[string]int,len(snapshot.Devices))
	for i,d:=range snapshot.Devices {idx[d.MAC]=i}
	seen:=map[string]bool{}
	for _,ext:=range report.Devices {
		mac:=normalizeMAC(ext.MAC)
		if mac==""||seen[mac] {
			return snapshot,errors.New("discovery: invalid or duplicate controller MAC")
		}
		seen[mac]=true
		if i,found:=idx[mac];found {
			d:=&snapshot.Devices[i]
			if !containsSource(d.Sources,report.Source) {
				d.Sources=append(d.Sources,report.Source)
				sort.Strings(d.Sources)
			}
			if d.ReportedVendor=="" {
				d.ReportedVendor=sanitize(ext.ReportedVendor,80)
				if d.ReportedVendor!="" {d.VendorEvidence="authenticated_controller_inventory"}
			}
			if d.ReportedModel=="" {d.ReportedModel=sanitize(ext.ReportedModel,120)}
			d.Capabilities=append(d.Capabilities,Capability{
				Name:"controller_inventory",Status:"observed",Evidence:report.Source,
			})
			// Controller inventory presence is not proof of live ping, radio
			// write entitlement, client association or a physical parent.
			continue
		}
		if len(snapshot.Devices)>=maxDevices {snapshot.Truncated=true;continue}
		d:=Device{
			MAC:mac,Name:sanitize(ext.Name,120),Kind:"unknown",
			KindEvidence:"not_verified",ReportedVendor:sanitize(ext.ReportedVendor,80),
			ReportedModel:sanitize(ext.ReportedModel,120),
			VendorEvidence:"authenticated_controller_inventory",
			Addresses:[]string{},Sources:[]string{report.Source},
			Attachments:[]Attachment{},OnlineVerified:false,
			Capabilities:[]Capability{
				{Name:"gateway_observation",Status:"unknown"},
				{Name:"controller_inventory",Status:"observed",Evidence:report.Source},
				{Name:"third_party_management",Status:"unknown"},
				{Name:"ap_clients",Status:"unknown"},
				{Name:"ssid_configuration",Status:"unknown"},
				{Name:"radio_optimization",Status:"unknown"},
				{Name:"physical_topology",Status:"unknown"},
			},
		}
		if d.Name=="" {d.Name="Unidentified device"}
		snapshot.Devices=append(snapshot.Devices,d)
		idx[mac]=len(snapshot.Devices)-1
	}
	sort.Slice(snapshot.Devices,func(i,j int)bool{return snapshot.Devices[i].MAC<snapshot.Devices[j].MAC})
	snapshot.Notes=append(snapshot.Notes,"Authenticated controller inventory is read-only; model, online state, wireless configuration and physical links remain unverified unless separately measured.")
	return snapshot,nil
}

func containsSource(list []string,source string)bool {
	for _,item:=range list {if item==source{return true}}
	return false
}
