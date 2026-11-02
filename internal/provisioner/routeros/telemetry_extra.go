package routeros

import (
	"context"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Optional, read-only extras layered on the core telemetry: per-port link state,
// live traffic rates, hardware health and firmware status. Every one is a fixed read,
// each has its own short deadline, and a failure only leaves that extra out. None
// of them can turn the core reading into an error or add an issue.
//
// RouterOS computes live rates itself (/interface monitor-traffic), so a speed is
// available on the first reading. Health sensors can load the CPU of some models if
// polled hard, so health and firmware are read far less often than the 15 s page poll.
const TelemetryEthernetCommand = `:foreach i in=[/interface ethernet find] do={ :local m [/interface ethernet monitor $i once as-value]; :put ("name: ".($m->"name")); :put ("status: ".($m->"status")); :put ("rate: ".($m->"rate")); :put ("full-duplex: ".($m->"full-duplex")); :put ("auto-negotiation: ".($m->"auto-negotiation")); :put ("supported: ".($m->"supported")); :put ("link-partner-advertising: ".($m->"link-partner-advertising")); }`

// TelemetryEthernetBasicCommand is the shorter read that is known to work on real routers. It is used when the
// fuller command above is refused, so a failure of the newer fields can never cost a port its link rate.
const TelemetryEthernetBasicCommand = `:foreach i in=[/interface ethernet find] do={ :local m [/interface ethernet monitor $i once as-value]; :put ("name: ".($m->"name")); :put ("status: ".($m->"status")); :put ("rate: ".($m->"rate")); :put ("full-duplex: ".($m->"full-duplex")); :put ("auto-negotiation: ".($m->"auto-negotiation")); }`
const TelemetryEthernetStatsCommand = `:foreach row in=[/interface ethernet print stats as-value] do={ :put ("name: ".($row->"name")); :put ("rx-fcs-error: ".($row->"rx-fcs-error")); :put ("rx-align-error: ".($row->"rx-align-error")); :put ("rx-fragment: ".($row->"rx-fragment")); :put ("rx-runt: ".($row->"rx-runt")); :put ("rx-carrier-error: ".($row->"rx-carrier-error")); }`
const TelemetryTrafficCommand = `:foreach i in=[/interface find where running=yes] do={ :local n [/interface get $i name]; :local m [/interface monitor-traffic $n once as-value]; :put ("name: ".$n); :put ("rx-bps: ".($m->"rx-bits-per-second")); :put ("tx-bps: ".($m->"tx-bits-per-second")); }`
const TelemetryHealthCommand = "/system health print terse"
const TelemetryFirmwareCommand = `:do { :put ("model: ".[/system routerboard get model]) } on-error={}; :do { :put ("current-firmware: ".[/system routerboard get current-firmware]) } on-error={}; :do { :put ("upgrade-firmware: ".[/system routerboard get upgrade-firmware]) } on-error={}; :do { :put ("installed-version: ".[/system package update get installed-version]) } on-error={}; :do { :put ("latest-version: ".[/system package update get latest-version]) } on-error={}; :do { :put ("channel: ".[/system package update get channel]) } on-error={}`

const (
	extraReadTimeout = 5 * time.Second
	healthEvery      = 60 * time.Second
	firmwareEvery    = 30 * time.Minute
	extraCacheCap    = 256
)

type HealthReading struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit,omitempty"`
}
type FirmwareStatus struct {
	Model               string `json:"model,omitempty"`
	CurrentFirmware     string `json:"current_firmware,omitempty"`
	UpgradeFirmware     string `json:"upgrade_firmware,omitempty"`
	InstalledVersion    string `json:"installed_version,omitempty"`
	LatestVersion       string `json:"latest_version,omitempty"`
	Channel             string `json:"channel,omitempty"`
	UpdateAvailable     bool   `json:"update_available"`
	FirmwareUpgradeable bool   `json:"firmware_upgrade_available"`
}

type extraCache struct {
	health     []HealthReading
	firmware   *FirmwareStatus
	healthAt   time.Time
	firmwareAt time.Time
	used       time.Time
}
type extraStore struct {
	mu      sync.Mutex
	entries map[string]*extraCache
}

func (s *extraStore) entry(key string, now time.Time) *extraCache {
	if s.entries == nil {
		s.entries = map[string]*extraCache{}
	}
	e := s.entries[key]
	if e == nil {
		if len(s.entries) >= extraCacheCap {
			oldest, at := "", now
			for k, v := range s.entries {
				if oldest == "" || v.used.Before(at) {
					oldest, at = k, v.used
				}
			}
			delete(s.entries, oldest)
		}
		e = &extraCache{}
		s.entries[key] = e
	}
	e.used = now
	return e
}

// forgetFirmware drops the retained firmware status so the next reading re-reads it; used once
// the firmware has been written, since the retained value would otherwise show the old version.
func (s *extraStore) forgetFirmware(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.entries[key]; e != nil {
		e.firmware, e.firmwareAt = nil, time.Time{}
	}
}

// collectExtras adds the optional readings to result. Health and firmware are only
// re-read when due; otherwise the retained values (with their original read time) are used.
func (a *DeviceApplier) collectExtras(ctx context.Context, runner CommandRunner, business, device string, result *RouterTelemetry) {
	now := time.Now().UTC()
	a.extras.mu.Lock()
	cache := a.extras.entry(business+"/"+device, now)
	needHealth := cache.healthAt.IsZero() || now.Sub(cache.healthAt) >= healthEvery
	needFirmware := cache.firmwareAt.IsZero() || now.Sub(cache.firmwareAt) >= firmwareEvery
	a.extras.mu.Unlock()

	var health []HealthReading
	var firmware *FirmwareStatus
	var gotHealth, gotFirmware bool
	var link []linkRow
	var phyErrors map[string]uint64
	var traffic map[string][2]*float64
	run := func(command string) (string, bool) {
		sub, cancel := context.WithTimeout(ctx, extraReadTimeout)
		defer cancel()
		out, err := runner.Run(sub, command)
		return out, err == nil
	}
	out, ok := run(TelemetryEthernetCommand)
	if !ok {
		out, ok = run(TelemetryEthernetBasicCommand)
	}
	if ok {
		link = parseEthernetLinks(out)
	}
	if out, ok := run(TelemetryEthernetStatsCommand); ok {
		phyErrors = parseEthernetStats(out)
	}
	if out, ok := run(TelemetryTrafficCommand); ok {
		traffic = parseTrafficRates(out)
	}
	if needHealth {
		if out, ok := run(TelemetryHealthCommand); ok {
			health, gotHealth = parseHealth(out), true
		}
	}
	if needFirmware {
		if out, ok := run(TelemetryFirmwareCommand); ok {
			firmware = parseFirmware(out)
			gotFirmware = firmware != nil
		}
	}

	at := time.Now().UTC()
	if len(link) > 0 && len(result.Interfaces) > 0 {
		byName := map[string]linkRow{}
		for _, row := range link {
			byName[row.name] = row
		}
		for i := range result.Interfaces {
			if row, ok := byName[result.Interfaces[i].Name]; ok {
				result.Interfaces[i].LinkStatus = row.status
				result.Interfaces[i].LinkRate = row.rate
				result.Interfaces[i].FullDuplex = row.fullDuplex
				result.Interfaces[i].Negotiation = row.negotiation
				result.Interfaces[i].MaxRateMbps = row.maxMbps
				result.Interfaces[i].PartnerMaxMbps = row.partnerMbps
			}
		}
		result.Observed["link"] = at
	}
	if len(phyErrors) > 0 && len(result.Interfaces) > 0 {
		for i := range result.Interfaces {
			if n, ok := phyErrors[result.Interfaces[i].Name]; ok {
				v := n
				result.Interfaces[i].PhyErrors = &v
			}
		}
		result.Observed["phy_errors"] = at
	}
	if len(traffic) > 0 && len(result.Interfaces) > 0 {
		for i := range result.Interfaces {
			if rates, ok := traffic[result.Interfaces[i].Name]; ok {
				result.Interfaces[i].RXBitsPerSecond, result.Interfaces[i].TXBitsPerSecond = rates[0], rates[1]
			}
		}
		result.Observed["traffic"] = at
	}

	a.extras.mu.Lock()
	if gotHealth {
		cache.health, cache.healthAt = health, at
	}
	if gotFirmware {
		cache.firmware, cache.firmwareAt = firmware, at
	}
	if len(cache.health) > 0 {
		result.Health = append([]HealthReading(nil), cache.health...)
		result.Observed["health"] = cache.healthAt
	}
	if cache.firmware != nil {
		f := *cache.firmware
		result.Firmware = &f
		result.Observed["firmware"] = cache.firmwareAt
	}
	a.extras.mu.Unlock()
}

type linkRow struct {
	name, status, rate string
	fullDuplex         *bool
	negotiation        string
	maxMbps            int
	partnerMbps        int
}

var linkRatePattern = regexp.MustCompile(`^[0-9]{1,6}(?:\.[0-9])?[MGT]bps$`)
var linkStatusPattern = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

// parseEthernetLinks reads blocks of "name:/status:/rate:/full-duplex:/auto-negotiation:" lines.
// Anything that does not look like a RouterOS value is dropped rather than shown.
func parseEthernetLinks(output string) []linkRow {
	var rows []linkRow
	var cur *linkRow
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r", ""), "\n") {
		key, value, ok := strings.Cut(strings.TrimRight(line, " "), ": ")
		if !ok {
			key, ok = strings.CutSuffix(strings.TrimSpace(line), ":")
			if !ok {
				continue
			}
			value = ""
		}
		value = strings.TrimSpace(value)
		switch key {
		case "name":
			if value == "" || len(value) > 128 || len(rows) >= 128 {
				cur = nil
				continue
			}
			rows = append(rows, linkRow{name: value})
			cur = &rows[len(rows)-1]
		case "status":
			if cur != nil && linkStatusPattern.MatchString(value) {
				cur.status = value
			}
		case "rate":
			if cur != nil && linkRatePattern.MatchString(value) {
				cur.rate = value
			}
		case "full-duplex":
			if cur != nil {
				cur.fullDuplex = telemetryBool(value)
			}
		case "auto-negotiation":
			if cur != nil && (value == "done" || value == "failed" || value == "incomplete" || value == "disabled") {
				cur.negotiation = value
			}
		case "supported":
			if cur != nil {
				cur.maxMbps = maxModeMbps(value)
			}
		case "link-partner-advertising":
			if cur != nil {
				cur.partnerMbps = maxModeMbps(value)
			}
		}
	}
	return rows
}

func parseRate(value string) *float64 {
	n, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || n < 0 || n > 1e13 || math.IsNaN(n) || math.IsInf(n, 0) {
		return nil
	}
	return &n
}

// parseTrafficRates reads blocks of "name:/rx-bps:/tx-bps:" lines into bits per second.
func parseTrafficRates(output string) map[string][2]*float64 {
	out := map[string][2]*float64{}
	name := ""
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r", ""), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ": ")
		if !ok {
			continue
		}
		switch key {
		case "name":
			name = value
		case "rx-bps", "tx-bps":
			if name == "" || len(out) >= 128 {
				continue
			}
			pair := out[name]
			if key == "rx-bps" {
				pair[0] = parseRate(value)
			} else {
				pair[1] = parseRate(value)
			}
			out[name] = pair
		}
	}
	for name, pair := range out {
		if pair[0] == nil && pair[1] == nil {
			delete(out, name)
		}
	}
	return out
}

var healthPair = regexp.MustCompile(`([A-Za-z0-9_.-]+)=("[^"]*"|[^\s]+)`)
var healthNumber = regexp.MustCompile(`^(-?[0-9]+(?:\.[0-9]+)?)\s*([A-Za-z%°]*)$`)

// parseHealth reads "/system health print terse". RouterOS 7 prints one sensor per line
// ("name=temperature value=44 type=C"); older output is flat ("temperature=44C voltage=24V").
// Both are accepted. Non-numeric values are skipped, never shown as zero.
func parseHealth(output string) []HealthReading {
	var out []HealthReading
	add := func(name, value, unit string) {
		match := healthNumber.FindStringSubmatch(strings.Trim(value, `"`))
		if match == nil || name == "" || len(name) > 64 || len(out) >= 32 {
			return
		}
		n, err := strconv.ParseFloat(match[1], 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return
		}
		if unit == "" {
			unit = match[2]
		}
		out = append(out, HealthReading{Name: name, Value: n, Unit: healthUnit(name, unit)})
	}
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r", ""), "\n") {
		pairs := map[string]string{}
		var order []string
		for _, m := range healthPair.FindAllStringSubmatch(line, -1) {
			pairs[m[1]] = m[2]
			order = append(order, m[1])
		}
		if name, hasName := pairs["name"]; hasName {
			if value, hasValue := pairs["value"]; hasValue {
				add(strings.Trim(name, `"`), value, strings.Trim(pairs["type"], `"`))
			}
			continue
		}
		for _, key := range order {
			add(key, pairs[key], "")
		}
	}
	return out
}

func healthUnit(name, unit string) string {
	switch strings.ToUpper(unit) {
	case "C":
		return "°C"
	case "V":
		return "V"
	case "A":
		return "A"
	case "W":
		return "W"
	case "RPM":
		return "RPM"
	case "%":
		return "%"
	}
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "temperature"):
		return "°C"
	case strings.Contains(lower, "voltage"):
		return "V"
	case strings.Contains(lower, "power"):
		return "W"
	case strings.Contains(lower, "current"):
		return "A"
	case strings.Contains(lower, "fan"):
		return "RPM"
	}
	return unit
}

var versionPattern = regexp.MustCompile(`^([0-9]+)\.([0-9]+)(?:\.([0-9]+))?`)

func versionTuple(v string) ([3]int, bool) {
	m := versionPattern.FindStringSubmatch(strings.TrimSpace(v))
	if m == nil {
		return [3]int{}, false
	}
	var t [3]int
	for i := 0; i < 3; i++ {
		if m[i+1] == "" {
			continue
		}
		n, err := strconv.Atoi(m[i+1])
		if err != nil || n > 100000 {
			return [3]int{}, false
		}
		t[i] = n
	}
	return t, true
}

func versionNewer(candidate, installed string) bool {
	a, okA := versionTuple(candidate)
	b, okB := versionTuple(installed)
	if !okA || !okB {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}

// parseFirmware reads the firmware/version lines. Updates are claimed only when both versions
// parse and the latest is higher; a missing or unreadable value never reports "up to date".
func parseFirmware(output string) *FirmwareStatus {
	f := parseValueList(output)
	clean := func(key string) string {
		v := strings.TrimSpace(f[key])
		if len(v) > 64 || strings.ContainsAny(v, "\n\x00") {
			return ""
		}
		return v
	}
	s := &FirmwareStatus{
		Model: clean("model"), CurrentFirmware: clean("current-firmware"), UpgradeFirmware: clean("upgrade-firmware"),
		InstalledVersion: clean("installed-version"), LatestVersion: clean("latest-version"), Channel: clean("channel"),
	}
	if *s == (FirmwareStatus{}) {
		return nil
	}
	s.UpdateAvailable = versionNewer(s.LatestVersion, s.InstalledVersion)
	s.FirmwareUpgradeable = s.UpgradeFirmware != "" && s.CurrentFirmware != "" && versionNewer(s.UpgradeFirmware, s.CurrentFirmware)
	return s
}

var modePattern = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?)([MG])-`)

// maxModeMbps is the fastest link mode in a list such as "10M-baseT-half,100M-baseT-full,1G-baseT-full,2.5G-baseT",
// in Mbps. Entries it does not recognise are ignored; an empty or unreadable list gives 0, never a guess.
func maxModeMbps(list string) int {
	best := 0.0
	for _, token := range strings.FieldsFunc(list, func(r rune) bool { return r == ',' || r == ';' || r == ' ' }) {
		m := modePattern.FindStringSubmatch(token)
		if m == nil {
			continue
		}
		n, err := strconv.ParseFloat(m[1], 64)
		if err != nil || n <= 0 || n > 1000 {
			continue
		}
		if m[2] == "G" {
			n *= 1000
		}
		if n > best {
			best = n
		}
	}
	return int(best)
}

// parseEthernetStats adds up the physical-layer receive error counters per port. A port is included only if
// the router reported at least one of them, so a model without these counters is never shown as error-free.
func parseEthernetStats(output string) map[string]uint64 {
	out := map[string]uint64{}
	name, seen := "", false
	var sum uint64
	flush := func() {
		if name != "" && seen && len(out) < 128 {
			out[name] = sum
		}
	}
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r", ""), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ": ")
		if !ok {
			continue
		}
		switch key {
		case "name":
			flush()
			name, seen, sum = value, false, 0
		case "rx-fcs-error", "rx-align-error", "rx-fragment", "rx-runt", "rx-carrier-error":
			n, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
			if err != nil || name == "" || n > 1<<60 {
				continue
			}
			seen = true
			sum += n
		}
	}
	flush()
	return out
}
