// Package discovery builds a vendor-neutral, evidence-based LAN inventory from
// reports already collected over a tenant-scoped Magic Hub/MikroTik connection.
// It never runs a scanner, probes devices, or writes to a gateway/AP.
package discovery

import (
	"net"
	"sort"
	"strings"
	"unicode"

	"mega-magic/internal/provisioner/routeros"
)

const (
	maxDevices = 512
	maxRows = 2500
)

var evidenceSources = []string{"arp", "bridge_hosts", "discovery_leases", "hotspot_sessions", "neighbors", "wifi_clients"}

// Label is an owner-provided display override. It is NOT evidence of device
// reachability, credentials, capabilities or an authenticated vendor identity.
type Label struct {
	MAC  string
	Name string
	Kind string
}

// SourceStatus never collapses an unreadable menu into "no devices found".
type SourceStatus struct {
	Name string `json:"name"`
	Available bool `json:"available"`
	Count int `json:"count"`
}

// Attachment records how the gateway observed a MAC address. No attachment
// is proof of a direct cable, actual parent switch or an LLDP physical hop.
type Attachment struct {
	Source string `json:"source"`
	GatewayInterface string `json:"gateway_interface"`
	Relation string `json:"relation"`
	DirectPhysicalLinkVerified bool `json:"direct_physical_link_verified"`
}

type Capability struct {
	Name string `json:"name"`
	Status string `json:"status"` // observed | unknown
	Evidence string `json:"evidence,omitempty"`
}

// Device is an observed MAC, never a promised manageable/online router or AP.
type Device struct {
	MAC string `json:"mac"`
	Name string `json:"name"`
	Kind string `json:"kind"`
	KindEvidence string `json:"kind_evidence"`
	ReportedVendor string `json:"reported_vendor,omitempty"`
	ReportedModel string `json:"reported_model,omitempty"`
	VendorEvidence string `json:"vendor_evidence,omitempty"`
	Addresses []string `json:"addresses"`
	Sources []string `json:"sources"`
	Attachments []Attachment `json:"attachments"`
	Capabilities []Capability `json:"capabilities"`
	PossiblyRandomizedMAC bool `json:"possibly_randomized_mac"`
	OnlineVerified bool `json:"online_verified"`
}

// Snapshot is source-specific evidence, not an active host-scan result.
type Snapshot struct {
	GatewayID string `json:"gateway_id"`
	ObservedAt string `json:"observed_at,omitempty"`
	Devices []Device `json:"devices"`
	Sources []SourceStatus `json:"sources"`
	Partial bool `json:"partial"`
	Truncated bool `json:"truncated"`
	ReadsOnly bool `json:"reads_only"`
	PhysicalLinksVerified bool `json:"physical_links_verified"`
	Notes []string `json:"notes"`
}

func normalizeMAC(raw string) string {
	h, err := net.ParseMAC(strings.TrimSpace(raw))
	if err != nil || len(h) != 6 || h[0]&1 != 0 {
		return ""
	}
	zero := true
	for _, b := range h { if b != 0 {zero = false; break} }
	if zero { return "" }
	return strings.ToUpper(h.String())
}

func sanitize(raw string, maxLen int) string {
	raw = strings.TrimSpace(raw)
	if len(raw) > maxLen {raw = raw[:maxLen]}
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if !unicode.IsPrint(r) || unicode.IsControl(r) {return -1}
		return r
	}, raw))
}

func vendorHint(platform, board string) string {
	s := strings.ToLower(platform+" "+board)
	switch {
	case strings.Contains(s, "mikrotik"), strings.Contains(s, "routeros"):
		return "MikroTik"
	case strings.Contains(s, "ruijie"), strings.Contains(s, "reyee"):
		return "Ruijie/Reyee"
	case strings.Contains(s, "ikuai"), strings.Contains(s, "爱快"):
		return "iKuai"
	case strings.Contains(s, "huawei"):
		return "Huawei"
	case strings.Contains(s, "tp-link"), strings.Contains(s, "omada"):
		return "TP-Link"
	case strings.Contains(s, "ubiquiti"), strings.Contains(s, "unifi"):
		return "Ubiquiti"
	default:
		return ""
	}
}

func sourceRank(source string) int {
	switch source {
	case "neighbors": return 3
	case "discovery_leases": return 2
	case "arp": return 1
	default: return 0
	}
}

type aggregate struct {
	d Device
	ips map[string]bool
	sources map[string]bool
	attachments map[string]Attachment
	nameRank int
}

func observedPort(values map[string]string, source string) (string, string) {
	var port, relation string
	switch source {
	case "neighbors":
		port = values["interface"]
		relation = "neighbor_seen_on_gateway_interface"
	case "bridge_hosts":
		port = values["on-interface"]
		relation = "bridge_mac_learned_via_port"
	case "wifi_clients":
		port = values["interface"]
		relation = "station_reported_on_gateway_wifi"
	default:
		return "", ""
	}
	port = sanitize(port, 64)
	if port == "" {return "", ""}
	return port, relation
}

// Build uses only fixed, existing RouterOS read-only observation tables and
// optional tenant-owned labels. No arbitrary source paths or probe targets are
// accepted, and no generic/configuration capabilities are invented.
func Build(gatewayID string, inv routeros.NetworkInventory, labels []Label) Snapshot {
	snap := Snapshot{
		GatewayID: gatewayID,
		ObservedAt: inv.ObservedAt,
		Devices: []Device{},
		Sources: []SourceStatus{},
		ReadsOnly: true,
		Notes: []string{
			"Observations originate at the selected gateway; no client-side LAN scan or third-party login was attempted.",
			"MACs learned through bridges and neighbours do not prove direct physical cable connections.",
			"Vendor and model are hints reported by neighbours, not authenticated management capabilities.",
			"Configuration, AP optimization, SNMP and LLDP access are unknown until individually verified by a compatible adapter.",
		},
	}

	ownMACs := map[string]bool{}
	if inv.Available["interfaces"] {
		for _, row := range inv.Objects["interfaces"] {
			if m := normalizeMAC(row.Values["mac-address"]); m != "" {ownMACs[m] = true}
		}
	}
	labelByMAC := map[string]Label{}
	for _, l := range labels {
		m := normalizeMAC(l.MAC)
		if m == "" {continue}
		l.Name = sanitize(l.Name, 120)
		if l.Name == "" {continue}
		switch l.Kind {
		case "ap", "switch", "controller", "computer", "printer", "other":
			labelByMAC[m] = l
		}
	}

	found := map[string]*aggregate{}
	totalRows := 0
	for _, source := range evidenceSources {
		available := inv.Available[source]
		rows := inv.Objects[source]
		snap.Sources = append(snap.Sources, SourceStatus{Name: source, Available: available, Count: func() int {
			if available {return len(rows)}
			return 0
		}()})
		if !available {
			snap.Partial = true
			continue
		}
		for _, row := range rows {
			totalRows++
			if totalRows > maxRows { snap.Truncated = true; break }
			v := row.Values
			if source == "bridge_hosts" && (strings.EqualFold(v["local"], "true") || strings.EqualFold(v["local"], "yes")) {
				continue
			}
			mac := normalizeMAC(v["mac-address"])
			if mac == "" || ownMACs[mac] {continue}
			a := found[mac]
			if a == nil {
				if len(found) >= maxDevices {snap.Truncated = true; continue}
				a = &aggregate{
					d: Device{MAC: mac, Kind: "unknown", KindEvidence: "not_verified", Addresses: []string{}, Sources: []string{}, Attachments: []Attachment{}, Capabilities: []Capability{}, PossiblyRandomizedMAC: false},
					ips: map[string]bool{}, sources: map[string]bool{}, attachments: map[string]Attachment{},
				}
				// A locally administered MAC can rotate; never use it as a
				// durable person or physical-device identity for enforcement.
				mh, _ := net.ParseMAC(mac)
				a.d.PossiblyRandomizedMAC = len(mh) == 6 && mh[0]&2 != 0
				found[mac] = a
			}
			a.sources[source] = true
			rank := sourceRank(source)
			candidateName := ""
			if source == "neighbors" {candidateName = v["identity"]}
			if source == "discovery_leases" {candidateName = v["host-name"]}
			if candidateName = sanitize(candidateName, 120); candidateName != "" && rank > a.nameRank {
				a.d.Name, a.nameRank = candidateName, rank
			}
			if source == "neighbors" {
				if board := sanitize(v["board"], 120); board != "" {
					a.d.ReportedModel = board
					a.d.VendorEvidence = "router_neighbor_reported"
				}
				if vendor := vendorHint(v["platform"], v["board"]); vendor != "" {
					a.d.ReportedVendor = vendor
					a.d.VendorEvidence = "router_neighbor_reported"
				}
			}
			var ipValue string
			switch source {
			case "arp", "discovery_leases", "neighbors", "hotspot_sessions":
				ipValue = strings.TrimSpace(v["address"])
			}
			if ip := net.ParseIP(ipValue); ip != nil && !ip.IsUnspecified() && !ip.IsMulticast() {
				a.ips[ip.String()] = true
			}
			if port, relation := observedPort(v, source); port != "" {
				key := source+"|"+port
				a.attachments[key] = Attachment{
					Source: source, GatewayInterface: port, Relation: relation,
					DirectPhysicalLinkVerified: false,
				}
			}
		}
		if totalRows > maxRows {break}
	}
	keys := make([]string, 0, len(found))
	for mac := range found {keys = append(keys, mac)}
	sort.Strings(keys)
	for _, mac := range keys {
		a := found[mac]
		d := a.d
		if label, ok := labelByMAC[mac]; ok {
			d.Name, d.Kind, d.KindEvidence = label.Name, label.Kind, "owner_label_unverified"
		}
		if d.Name == "" {d.Name = "Unidentified device"}
		for ip := range a.ips {d.Addresses = append(d.Addresses, ip)}
		sort.Strings(d.Addresses)
		for source := range a.sources {d.Sources = append(d.Sources, source)}
		sort.Strings(d.Sources)
		edges := make([]string, 0, len(a.attachments))
		for key := range a.attachments {edges = append(edges, key)}
		sort.Strings(edges)
		for _, key := range edges {d.Attachments = append(d.Attachments, a.attachments[key])}
		d.Capabilities = []Capability{
			{Name: "gateway_observation", Status: "observed", Evidence: strings.Join(d.Sources, ",")},
			{Name: "third_party_management", Status: "unknown"},
			{Name: "ap_clients", Status: "unknown"},
			{Name: "ssid_configuration", Status: "unknown"},
			{Name: "radio_optimization", Status: "unknown"},
			{Name: "physical_topology", Status: "unknown"},
		}
		snap.Devices = append(snap.Devices, d)
	}
	return snap
}
