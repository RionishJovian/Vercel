package routeros

import (
	"context"
	"strconv"
	"strings"
)

// MenuCapability describes a RouterOS facility discovered from the router
// itself. Supported means the RouterOS menu exists; Configured means it
// currently has at least one record. Keeping those facts separate prevents a
// blank-but-capable CCR from being mistaken for unsupported hardware.
type MenuCapability struct {
	Supported  bool `json:"supported"`
	Configured bool `json:"configured"`
	Count      int  `json:"count"`
}

// WebFigCapability reports the two RouterOS web-management services. Mega
// Magic never treats an enabled public service as permission to expose it; this
// is discovery data used by the tenant-scoped launch policy.
type WebFigCapability struct {
	HTTPEnabled  bool `json:"http_enabled"`
	HTTPSEnabled bool `json:"https_enabled"`
	HTTPSPort    int  `json:"https_port,omitempty"`
	TrustedOnly  bool `json:"trusted_only"`
}

// RouterCapabilities is deliberately capability-driven. Board/model names are
// returned as descriptive facts only and never gate a feature. Gateway duties
// and wireless duties are independent: a CCR or CHR can be a full hotspot,
// DHCP and RADIUS gateway while its radios live on external APs.
type RouterCapabilities struct {
	DeviceID          string           `json:"device_id"`
	Detected          bool             `json:"detected"`
	BoardName         string           `json:"board_name,omitempty"`
	Architecture      string           `json:"architecture,omitempty"`
	RouterOSVersion   string           `json:"routeros_version,omitempty"`
	Gateway           bool             `json:"gateway"`
	Hotspot           MenuCapability   `json:"hotspot"`
	DHCPServer        MenuCapability   `json:"dhcp_server"`
	Radius            MenuCapability   `json:"radius"`
	ModernWiFi        MenuCapability   `json:"modern_wifi"`
	LegacyWireless    MenuCapability   `json:"legacy_wireless"`
	ModernCAPsMAN     MenuCapability   `json:"modern_capsman"`
	LegacyCAPsMAN     MenuCapability   `json:"legacy_capsman"`
	WebFig            WebFigCapability `json:"webfig"`
	ExternalAPsLikely bool             `json:"external_aps_likely"`
	Notes             []string         `json:"notes"`
}

func countCapability(out string, err error) MenuCapability {
	if err != nil {
		return MenuCapability{}
	}
	n, convErr := strconv.Atoi(strings.TrimSpace(out))
	if convErr != nil || n < 0 {
		n = 0
	}
	return MenuCapability{Supported: true, Configured: n > 0, Count: n}
}

func enabledService(services []provisionerService, name string) (enabled bool, port int, trusted bool) {
	for _, service := range services {
		if service.name != name {
			continue
		}
		return !service.disabled, service.port, strings.TrimSpace(service.address) != ""
	}
	return false, 0, false
}

// provisionerService is a small private projection of the service snapshot.
// It avoids exporting rollback internals as part of the discovery API.
type provisionerService struct {
	name     string
	disabled bool
	address  string
	port     int
}

func capabilityServices(raw string) []provisionerService {
	out := make([]provisionerService, 0, 8)
	current := map[string]string{}
	flush := func() {
		name := strings.TrimSpace(current["name"])
		if validServiceName(name) {
			disabled := strings.TrimSpace(current["disabled"])
			out = append(out, provisionerService{
				name: name, disabled: strings.EqualFold(disabled, "true") || strings.EqualFold(disabled, "yes"),
				address: strings.TrimSpace(current["address"]), port: atoiOr(current["port"], 0),
			})
		}
		current = map[string]string{}
	}
	for _, rawLine := range strings.Split(raw, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			if len(current) > 0 {
				flush()
			}
			continue
		}
		i := strings.Index(line, ":")
		if i < 0 {
			continue
		}
		key, value := strings.TrimSpace(line[:i]), strings.TrimSpace(line[i+1:])
		if key == "name" && current["name"] != "" {
			flush()
		}
		current[key] = value
	}
	if len(current) > 0 {
		flush()
	}
	return out
}

// InspectCapabilities reads feature menus from the exact tenant/device target.
// Optional menus fail closed to unsupported, while the system-resource probe is
// mandatory so an unreachable router cannot be presented as detected.
func (p *DeviceProber) InspectCapabilities(ctx context.Context, businessID, peerID string) (RouterCapabilities, error) {
	runner, err := p.runnerForDevice(ctx, businessID, peerID)
	if err != nil {
		return RouterCapabilities{}, err
	}
	resourceOut, err := runner.Run(ctx, SystemResourceCommand)
	if err != nil {
		return RouterCapabilities{}, err
	}
	resource := parseValueList(resourceOut)
	version, _ := parseSystemResource(resourceOut)

	runCount := func(command string) MenuCapability {
		out, commandErr := runner.Run(ctx, command)
		return countCapability(out, commandErr)
	}

	caps := RouterCapabilities{
		DeviceID:        peerID,
		Detected:        true,
		BoardName:       resource["board-name"],
		Architecture:    resource["architecture-name"],
		RouterOSVersion: version,
		Gateway:         true,
		Hotspot:         runCount("/ip hotspot print count-only"),
		DHCPServer:      runCount("/ip dhcp-server print count-only"),
		Radius:          runCount("/radius print count-only"),
		ModernWiFi:      runCount("/interface wifi print count-only"),
		LegacyWireless:  runCount("/interface wireless print count-only"),
		ModernCAPsMAN:   runCount("/interface wifi capsman remote-cap print count-only"),
		LegacyCAPsMAN:   runCount("/caps-man remote-cap print count-only"),
		Notes:           []string{},
	}
	serviceOut, serviceErr := runner.Run(ctx, "/ip service print value-list detail")
	if serviceErr == nil {
		services := capabilityServices(serviceOut)
		caps.WebFig.HTTPEnabled, _, _ = enabledService(services, "www")
		caps.WebFig.HTTPSEnabled, caps.WebFig.HTTPSPort, caps.WebFig.TrustedOnly = enabledService(services, "www-ssl")
	}

	localRadio := caps.ModernWiFi.Count+caps.LegacyWireless.Count > 0
	controller := caps.ModernCAPsMAN.Supported || caps.LegacyCAPsMAN.Supported
	caps.ExternalAPsLikely = !localRadio && (caps.Hotspot.Supported || caps.DHCPServer.Supported)
	if !localRadio {
		caps.Notes = append(caps.Notes, "No onboard wireless interface detected; gateway services remain available for external access points.")
	}
	if controller {
		caps.Notes = append(caps.Notes, "A CAPsMAN control menu is available for managed access points.")
	}
	if !caps.WebFig.HTTPSEnabled {
		caps.Notes = append(caps.Notes, "HTTPS WebFig is not enabled; Mega Magic will not expose plaintext WebFig.")
	}
	return caps, nil
}
