package routeros

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ReadObservations uses only fixed read-only menus and fields. It never calls
// network configuration, installs scripts, resets counters or disconnects guests.
func (a *DeviceApplier) ReadObservations(ctx context.Context, businessID, deviceID string) (NetworkInventory, error) {
	r, err := runnerForDevice(ctx, a.src, a.factory, a.timeout, businessID, deviceID)
	if err != nil {
		return NetworkInventory{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	inventory, err := readObservations(ctx, r)
	if err == nil {
		inventory.PendingConfirm = networkPendingFor(businessID, deviceID)
	}
	return inventory, err
}

var observationMenus = map[string]networkMenu{
	"hotspot_sessions": {"/ip hotspot active", []string{"user", "address", "mac-address", "uptime", "bytes-in", "bytes-out", "server"}},
	// Separate reads from hotspot_sessions, so a field that fails on one RouterOS cannot lose the session list.
	"hotspot_session_times": {"/ip hotspot active", []string{"idle-time", "session-time-left"}},
	"hotspot_bindings":      {"/ip hotspot ip-binding", []string{"mac-address", "type", "comment", "disabled"}},
	"discovery_leases":      {"/ip dhcp-server lease", []string{"address", "mac-address", "host-name", "status", "server"}},
	"arp":                   {"/ip arp", []string{"address", "mac-address", "interface"}},
	"neighbors":             {"/ip neighbor", []string{"address", "mac-address", "identity", "platform", "board", "interface"}},
	"bridge_hosts":          {"/interface bridge host", []string{"mac-address", "on-interface", "bridge", "local"}},
	"wifi_clients":          {"/interface wifi registration-table", []string{"mac-address", "interface", "signal"}},
	"wifi_interfaces":       {"/interface wifi", []string{"name", "configuration.mode", "configuration.ssid", "disabled", "running"}},
	// CAPsMAN v1 inventory. Read-only RouterOS menus: keep modern and legacy
	// controller evidence separate. Missing package/menu remains unavailable,
	// never "0 APs". Exclude security profiles and wireless passwords.
	"capsman_modern_caps":   {"/interface wifi capsman remote-cap", []string{"identity", "address", "state", "radios"}},
	"capsman_modern_radios": {"/interface wifi radio", []string{"radio-mac", "interface", "remote-cap", "band"}},
	"capsman_modern_profiles": {"/interface wifi configuration", []string{"name", "ssid", "mode", "disabled"}},
	"capsman_legacy_caps":   {"/caps-man remote-cap", []string{"identity", "address", "state", "radios"}},
	"capsman_legacy_radios": {"/caps-man radio", []string{"radio-mac", "interface", "remote-cap-identity", "remote-cap-name"}},
	"capsman_legacy_profiles": {"/caps-man configuration", []string{"name", "ssid", "mode"}},

	"interfaces":            {"/interface", []string{"name", "type", "mac-address", "running", "disabled"}},
	// Separate from interfaces, so a label that cannot be read never loses the port list.
	"interface_labels": {"/interface", []string{"name", "comment"}},
	// Filtered to the app's own block rules below, so no other firewall rule is exposed.
	"device_blocks": {"/ip firewall filter", []string{"src-mac-address", "comment", "disabled"}},
	"poe_ports":     {"/interface ethernet poe", []string{"name", "poe-out"}},
}

var observationOrder = []string{"hotspot_sessions", "hotspot_session_times", "hotspot_bindings", "discovery_leases", "arp", "neighbors", "bridge_hosts", "wifi_clients", "wifi_interfaces", "capsman_modern_caps", "capsman_modern_radios", "capsman_modern_profiles", "capsman_legacy_caps", "capsman_legacy_radios", "capsman_legacy_profiles", "interfaces", "interface_labels", "device_blocks", "poe_ports"}

func readObservations(ctx context.Context, r CommandRunner) (NetworkInventory, error) {
	result := NetworkInventory{ObservedAt: time.Now().UTC().Format(time.RFC3339), Objects: map[string][]NetworkObject{}, Available: map[string]bool{}}
	if err := ctx.Err(); err != nil {
		return NetworkInventory{}, err
	}
	result.Health = readHubHealth(ctx, r)
	for _, key := range observationOrder {
		if err := ctx.Err(); err != nil {
			return NetworkInventory{}, err
		}
		result.Objects[key] = []NetworkObject{}
		menu := observationMenus[key]
		out, err := r.Run(ctx, networkReadCommand(menu))
		if err != nil || len(out) > 512*1024 {
			result.Available[key] = false
			continue
		}
		objects, err := parseNetworkObjects(menu, out, key)
		result.Available[key] = err == nil
		if err == nil {
			if key == "device_blocks" {
				objects = ownDeviceBlocks(objects)
			}
			result.Objects[key] = objects
		}
	}
	if len(result.Available) != len(observationOrder) {
		return NetworkInventory{}, fmt.Errorf("incomplete observation inventory")
	}
	return result, nil
}

func ownDeviceBlocks(objects []NetworkObject) []NetworkObject {
	own := []NetworkObject{}
	for _, object := range objects {
		if strings.HasPrefix(object.Values["comment"], deviceBlockPrefix) {
			own = append(own, object)
		}
	}
	return own
}
