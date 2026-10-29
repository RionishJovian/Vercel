package routeros

import (
	"context"
	"fmt"
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
	return readObservations(ctx, r)
}

var observationMenus = map[string]networkMenu{
	"hotspot_sessions": {"/ip hotspot active", []string{"user", "address", "mac-address", "uptime", "bytes-in", "bytes-out", "server"}},
	"discovery_leases": {"/ip dhcp-server lease", []string{"address", "mac-address", "host-name", "status", "server"}},
	"arp":              {"/ip arp", []string{"address", "mac-address", "interface"}},
	"neighbors":        {"/ip neighbor", []string{"address", "mac-address", "identity", "platform", "board", "interface"}},
	"bridge_hosts":     {"/interface bridge host", []string{"mac-address", "on-interface", "bridge", "local"}},
	"wifi_clients":     {"/interface wifi registration-table", []string{"mac-address", "interface", "signal"}},
	"wifi_interfaces":  {"/interface wifi", []string{"name", "configuration.mode", "configuration.ssid", "disabled", "running"}},
	"interfaces":       {"/interface", []string{"name", "type", "mac-address", "running", "disabled"}},
}

var observationOrder = []string{"hotspot_sessions", "discovery_leases", "arp", "neighbors", "bridge_hosts", "wifi_clients", "wifi_interfaces", "interfaces"}

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
			result.Objects[key] = objects
		}
	}
	if len(result.Available) != len(observationOrder) {
		return NetworkInventory{}, fmt.Errorf("incomplete observation inventory")
	}
	return result, nil
}
