package routeros

import (
	"context"
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// All commands are fixed reads. No scheduler, agent, credentials or configuration
// is installed on the router. Byte counters include CPU-observed interface traffic;
// hardware-offloaded traffic may not be counted by RouterOS.
const TelemetryInterfacesCommand = `:foreach row in=[/interface print stats as-value] do={ :local id ($row->".id"); :put ("id: ".$id); :put ("name: ".[/interface get $id name]); :put ("type: ".[/interface get $id type]); :put ("running: ".[/interface get $id running]); :put ("disabled: ".[/interface get $id disabled]); :put ("rx-byte: ".($row->"rx-byte")); :put ("tx-byte: ".($row->"tx-byte")); }`
const TelemetryHotspotCountCommand = "/ip hotspot active print count-only"
const TelemetryDHCPCountCommand = "/ip dhcp-server lease print count-only where status=bound"

var ErrTelemetryRead = errors.New("router telemetry read failed")

type RouterResources struct {
	CPUPercent  *float64 `json:"cpu_percent"`
	MemoryTotal *uint64  `json:"memory_total_bytes,string"`
	MemoryUsed  *uint64  `json:"memory_used_bytes,string"`
	Uptime      *uint64  `json:"uptime_seconds"`
	Board       string   `json:"board_name,omitempty"`
	Version     string   `json:"routeros_version,omitempty"`
}
type InterfaceTelemetry struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Type            string   `json:"type,omitempty"`
	Running         *bool    `json:"running"`
	Disabled        *bool    `json:"disabled"`
	RXBytes         *uint64  `json:"rx_bytes,string"`
	TXBytes         *uint64  `json:"tx_bytes,string"`
	RXBitsPerSecond *float64 `json:"rx_bps"`
	TXBitsPerSecond *float64 `json:"tx_bps"`
}
type RouterTelemetry struct {
	Observed        map[string]time.Time `json:"observed_at"`
	DeviceID        string               `json:"device_id"`
	ReadAt          time.Time            `json:"read_at"`
	Resources       RouterResources      `json:"resources"`
	Interfaces      []InterfaceTelemetry `json:"interfaces"`
	HotspotSessions *uint64              `json:"hotspot_sessions"`
	DHCPLeases      *uint64              `json:"dhcp_bound_leases"`
	Issues          []string             `json:"issues"`
}

func (a *DeviceApplier) ReadTelemetry(ctx context.Context, business, device string) (RouterTelemetry, error) {
	runner, err := runnerForDevice(ctx, a.src, a.factory, a.timeout, business, device)
	if err != nil {
		return RouterTelemetry{}, ErrTelemetryRead
	}
	return collectTelemetry(ctx, runner, device)
}

func collectTelemetry(ctx context.Context, runner CommandRunner, device string) (RouterTelemetry, error) {
	result := RouterTelemetry{Observed: map[string]time.Time{}, DeviceID: device, Interfaces: []InterfaceTelemetry{}, Issues: []string{}}
	output, err := runner.Run(ctx, SystemResourceCommand)
	if err != nil || len(output) > 16384 {
		return RouterTelemetry{}, ErrTelemetryRead
	}
	result.ReadAt = time.Now().UTC()
	fields := parseValueList(output)
	result.Resources.Board = fields["board-name"]
	result.Resources.Version = fields["version"]
	if n, err := strconv.ParseFloat(strings.TrimSuffix(fields["cpu-load"], "%"), 64); err == nil && n >= 0 && n <= 100 && !math.IsNaN(n) && !math.IsInf(n, 0) {
		result.Resources.CPUPercent = &n
		result.Observed["cpu"] = result.ReadAt
	}
	if uptime, ok := telemetryUptime(fields["uptime"]); ok {
		result.Resources.Uptime = &uptime
		result.Observed["uptime"] = result.ReadAt
	}
	total, totalOK := telemetryBytes(fields["total-memory"])
	free, freeOK := telemetryBytes(fields["free-memory"])
	if totalOK && freeOK && total > 0 && free <= total {
		used := total - free
		result.Resources.MemoryTotal = &total
		result.Resources.MemoryUsed = &used
		result.Observed["memory"] = result.ReadAt
	}
	if result.Resources.Uptime == nil {
		return RouterTelemetry{}, ErrTelemetryRead
	}
	if result.Resources.CPUPercent == nil {
		result.Issues = append(result.Issues, "cpu_read_pending")
	}
	if result.Resources.MemoryTotal == nil {
		result.Issues = append(result.Issues, "memory_read_pending")
	}
	if output, err = runner.Run(ctx, TelemetryInterfacesCommand); err == nil {
		result.Interfaces, err = parseTelemetryInterfaces(output)
		if err == nil {
			result.Observed["interfaces"] = time.Now().UTC()
		}
	}
	if err != nil {
		result.Interfaces = nil
		result.Issues = append(result.Issues, "interface_read_pending")
	}
	for _, item := range []struct {
		command, issue, source string
		target                 **uint64
	}{{TelemetryHotspotCountCommand, "hotspot_read_pending", "hotspot", &result.HotspotSessions}, {TelemetryDHCPCountCommand, "dhcp_read_pending", "dhcp", &result.DHCPLeases}} {
		output, err = runner.Run(ctx, item.command)
		if err == nil {
			n, parseErr := strconv.ParseUint(strings.TrimSpace(output), 10, 64)
			if parseErr == nil && n <= 10000000 {
				*item.target = &n
				result.Observed[item.source] = time.Now().UTC()
				continue
			}
		}
		result.Issues = append(result.Issues, item.issue)
	}
	return result, nil
}

var telemetryBytePattern = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?)\s*(B|KiB|MiB|GiB|TiB|kB|MB|GB)?$`)

func telemetryBytes(value string) (uint64, bool) {
	match := telemetryBytePattern.FindStringSubmatch(strings.TrimSpace(value))
	if match == nil {
		return 0, false
	}
	n, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return 0, false
	}
	factor := map[string]float64{"": 1, "B": 1, "KiB": 1024, "MiB": 1 << 20, "GiB": 1 << 30, "TiB": 1 << 40, "kB": 1000, "MB": 1e6, "GB": 1e9}[match[2]]
	bytes := n * factor
	if bytes > 1<<60 || math.IsInf(bytes, 0) || math.IsNaN(bytes) {
		return 0, false
	}
	return uint64(bytes), true
}

var telemetryDurationPattern = regexp.MustCompile(`(?:(\d+)w)?(?:(\d+)d)?(?:(\d+)h)?(?:(\d+)m)?(?:(\d+)s)?`)
var telemetryClockPattern = regexp.MustCompile(`^(?:(\d+)w)?(?:(\d+)d)?(\d{2}):(\d{2}):(\d{2})$`)

func telemetryUptime(value string) (uint64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	var parts []string
	factors := []uint64{604800, 86400, 3600, 60, 1}
	if match := telemetryClockPattern.FindStringSubmatch(value); match != nil {
		parts = match[1:]
		for _, index := range []int{3, 4} {
			n, _ := strconv.Atoi(parts[index])
			if n > 59 {
				return 0, false
			}
		}
		h, _ := strconv.Atoi(parts[2])
		if h > 23 {
			return 0, false
		}
	} else {
		match := telemetryDurationPattern.FindStringSubmatch(value)
		if match == nil || match[0] != value {
			return 0, false
		}
		parts = match[1:]
	}
	var seconds uint64
	for i, part := range parts {
		if part == "" {
			continue
		}
		n, err := strconv.ParseUint(part, 10, 64)
		if err != nil || n > (math.MaxUint64-seconds)/factors[i] {
			return 0, false
		}
		seconds += n * factors[i]
	}
	return seconds, true
}
func telemetryBool(value string) *bool {
	var b bool
	switch value {
	case "true", "yes":
		b = true
	case "false", "no":
		b = false
	default:
		return nil
	}
	return &b
}
func telemetryUint(value string) *uint64 {
	n, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return nil
	}
	return &n
}
func parseTelemetryInterfaces(output string) ([]InterfaceTelemetry, error) {
	if len(output) > 256*1024 {
		return nil, ErrTelemetryRead
	}
	rows := []InterfaceTelemetry{}
	var current *InterfaceTelemetry
	seen := map[string]bool{}
	fields := map[string]bool{}
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r", ""), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ": ")
		if !ok {
			return nil, ErrTelemetryRead
		}
		if key == "id" {
			if !networkIDPattern.MatchString(value) || seen[value] || len(rows) >= 128 {
				return nil, ErrTelemetryRead
			}
			seen[value] = true
			fields = map[string]bool{}
			rows = append(rows, InterfaceTelemetry{ID: value})
			current = &rows[len(rows)-1]
			continue
		}
		if current == nil || fields[key] {
			return nil, ErrTelemetryRead
		}
		fields[key] = true
		switch key {
		case "name":
			if value == "" || len(value) > 128 || strings.ContainsAny(value, "\n\t\x00") {
				return nil, ErrTelemetryRead
			}
			current.Name = value
		case "type":
			current.Type = value
		case "running":
			current.Running = telemetryBool(value)
		case "disabled":
			current.Disabled = telemetryBool(value)
		case "rx-byte":
			current.RXBytes = telemetryUint(value)
		case "tx-byte":
			current.TXBytes = telemetryUint(value)
		default:
			return nil, ErrTelemetryRead
		}
	}
	for _, row := range rows {
		if row.Name == "" {
			return nil, ErrTelemetryRead
		}
	}
	return rows, nil
}
