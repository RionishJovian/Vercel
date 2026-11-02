package routeros

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestParseEthernetLinksKeepsOnlyPlausibleValues(t *testing.T) {
	rows := parseEthernetLinks("name: ether1\nstatus: link-ok\nrate: 1Gbps\nfull-duplex: true\nauto-negotiation: done\r\nname: ether2\nstatus: no-link\nrate: \nfull-duplex: \nname: ether3\nstatus: link-ok\nrate: <script>\nfull-duplex: yes\n")
	if len(rows) != 3 {
		t.Fatalf("rows: %+v", rows)
	}
	if rows[0].rate != "1Gbps" || rows[0].status != "link-ok" || rows[0].fullDuplex == nil || !*rows[0].fullDuplex {
		t.Fatalf("ether1: %+v", rows[0])
	}
	if rows[0].negotiation != "done" {
		t.Fatalf("the router's own negotiation result was lost: %q", rows[0].negotiation)
	}
	if rows[1].rate != "" || rows[1].fullDuplex != nil || rows[1].status != "no-link" {
		t.Fatalf("a port with no link must have no rate or duplex: %+v", rows[1])
	}
	if rows[2].rate != "" || rows[2].fullDuplex == nil || !*rows[2].fullDuplex {
		t.Fatalf("an unrecognised rate must be dropped, not shown: %+v", rows[2])
	}
}

func TestParseTrafficRatesNeverTurnsMissingIntoZero(t *testing.T) {
	rates := parseTrafficRates("name: ether1\nrx-bps: 5026\ntx-bps: 6899\nname: bridge\nrx-bps: \ntx-bps: \nname: lo\nrx-bps: 0\ntx-bps: nope\nname: ether5\nrx-bps: -4\ntx-bps: 12\n")
	if got := rates["ether1"]; got[0] == nil || *got[0] != 5026 || got[1] == nil || *got[1] != 6899 {
		t.Fatalf("ether1: %v", got)
	}
	if _, ok := rates["bridge"]; ok {
		t.Fatal("empty readings became a measurement")
	}
	if got := rates["lo"]; got[0] == nil || *got[0] != 0 || got[1] != nil {
		t.Fatalf("a real zero must stay zero and a bad value must be missing: %v", got)
	}
	if got := rates["ether5"]; got[0] != nil || got[1] == nil || *got[1] != 12 {
		t.Fatalf("a negative rate must be dropped: %v", got)
	}
}

func TestParseHealthAcceptsBothRouterOSFormats(t *testing.T) {
	v7 := "0 name=temperature value=44 type=C\n1 name=voltage value=24.1 type=V\n2 name=\"fan1-speed\" value=5200 type=RPM\n3 name=power-consumption value=9.5 type=W\n"
	got := parseHealth(v7)
	want := []HealthReading{{"temperature", 44, "°C"}, {"voltage", 24.1, "V"}, {"fan1-speed", 5200, "RPM"}, {"power-consumption", 9.5, "W"}}
	if len(got) != len(want) {
		t.Fatalf("v7: %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("v7 row %d: %+v want %+v", i, got[i], want[i])
		}
	}
	flat := parseHealth("voltage=24V temperature=40C cpu-temperature=51 board-temperature1=abc\n")
	if len(flat) != 3 || flat[0] != (HealthReading{"voltage", 24, "V"}) || flat[1] != (HealthReading{"temperature", 40, "°C"}) || flat[2] != (HealthReading{"cpu-temperature", 51, "°C"}) {
		t.Fatalf("flat: %+v", flat)
	}
	if got := parseHealth("0 name=temperature value=n/a type=C\n"); len(got) != 0 {
		t.Fatalf("an unreadable sensor became a value: %+v", got)
	}
	if got := parseHealth(""); got != nil {
		t.Fatalf("no sensors must stay empty: %+v", got)
	}
}

func TestParseFirmwareOnlyClaimsUpdatesItCanProve(t *testing.T) {
	f := parseFirmware("model: C53UiG+5HPaxD2HPaxD\ncurrent-firmware: 7.12.2\nupgrade-firmware: 7.16\ninstalled-version: 7.12.2\nlatest-version: 7.16.1\nchannel: stable\n")
	if f == nil || !f.UpdateAvailable || !f.FirmwareUpgradeable || f.Channel != "stable" {
		t.Fatalf("%+v", f)
	}
	same := parseFirmware("current-firmware: 7.12.2\nupgrade-firmware: 7.12.2\ninstalled-version: 7.12.2\nlatest-version: 7.12.2\n")
	if same.UpdateAvailable || same.FirmwareUpgradeable {
		t.Fatalf("same versions reported as upgradable: %+v", same)
	}
	older := parseFirmware("installed-version: 7.16\nlatest-version: 7.12.2\n")
	if older.UpdateAvailable {
		t.Fatal("an older 'latest' must not be an update")
	}
	unknown := parseFirmware("installed-version: 7.12.2\nlatest-version: \n")
	if unknown.UpdateAvailable {
		t.Fatal("a missing latest version must never claim an update or 'up to date'")
	}
	if parseFirmware("\n") != nil {
		t.Fatal("empty output must give no firmware status")
	}
}

type extraRunner struct {
	calls map[string]int
	fail  map[string]bool
}

func (r *extraRunner) Run(_ context.Context, command string) (string, error) {
	r.calls[command]++
	if r.fail[command] {
		return "secret transport detail", errors.New("boom")
	}
	switch command {
	case TelemetryEthernetCommand:
		return "name: ether1\nstatus: link-ok\nrate: 100Mbps\nfull-duplex: true\nauto-negotiation: done\nsupported: 10M-baseT-half,10M-baseT-full,100M-baseT-half,100M-baseT-full,1G-baseT-full\nlink-partner-advertising: 10M-baseT-half,10M-baseT-full,100M-baseT-half,100M-baseT-full,1G-baseT-full\n", nil
	case TelemetryEthernetBasicCommand:
		return "name: ether1\nstatus: link-ok\nrate: 100Mbps\nfull-duplex: true\nauto-negotiation: done\n", nil
	case TelemetryEthernetStatsCommand:
		return "name: ether1\nrx-fcs-error: 4\nrx-align-error: 1\nrx-fragment: 0\nrx-runt: 0\nrx-carrier-error: 0\nname: ether2\nrx-fcs-error: \nrx-align-error: \n", nil
	case TelemetryTrafficCommand:
		return "name: ether1\nrx-bps: 5000\ntx-bps: 7000\n", nil
	case TelemetryHealthCommand:
		return "0 name=temperature value=44 type=C\n", nil
	case TelemetryFirmwareCommand:
		return "installed-version: 7.12.2\nlatest-version: 7.16\n", nil
	}
	return "", errors.New("unexpected command")
}

func baseTelemetry() RouterTelemetry {
	return RouterTelemetry{Observed: map[string]time.Time{}, Interfaces: []InterfaceTelemetry{{ID: "*1", Name: "ether1"}, {ID: "*2", Name: "bridge"}}}
}

func TestCollectExtrasFillsPortsAndThrottlesSlowReads(t *testing.T) {
	a := &DeviceApplier{}
	runner := &extraRunner{calls: map[string]int{}}
	first := baseTelemetry()
	a.collectExtras(context.Background(), runner, "biz", "dev", &first)
	e := first.Interfaces[0]
	if e.LinkRate != "100Mbps" || e.FullDuplex == nil || !*e.FullDuplex || e.RXBitsPerSecond == nil || *e.RXBitsPerSecond != 5000 {
		t.Fatalf("ether1: %+v", e)
	}
	if first.Interfaces[1].LinkRate != "" || first.Interfaces[1].RXBitsPerSecond != nil {
		t.Fatalf("a port the router did not report must stay empty: %+v", first.Interfaces[1])
	}
	if len(first.Health) != 1 || first.Firmware == nil || !first.Firmware.UpdateAvailable {
		t.Fatalf("health/firmware missing: %+v %+v", first.Health, first.Firmware)
	}
	second := baseTelemetry()
	a.collectExtras(context.Background(), runner, "biz", "dev", &second)
	if runner.calls[TelemetryHealthCommand] != 1 || runner.calls[TelemetryFirmwareCommand] != 1 {
		t.Fatalf("slow reads were not throttled: %v", runner.calls)
	}
	if runner.calls[TelemetryEthernetCommand] != 2 || runner.calls[TelemetryTrafficCommand] != 2 {
		t.Fatalf("link and speed must be read every time: %v", runner.calls)
	}
	if len(second.Health) != 1 || !second.Observed["health"].Equal(first.Observed["health"]) {
		t.Fatal("retained health must keep its original read time")
	}
	other := baseTelemetry()
	a.collectExtras(context.Background(), runner, "biz", "other-device", &other)
	if runner.calls[TelemetryHealthCommand] != 2 {
		t.Fatal("another device must not reuse this device's readings")
	}
}

func TestCollectExtrasFailuresLeaveOnlyThatExtraOut(t *testing.T) {
	a := &DeviceApplier{}
	runner := &extraRunner{calls: map[string]int{}, fail: map[string]bool{TelemetryTrafficCommand: true, TelemetryHealthCommand: true}}
	result := baseTelemetry()
	a.collectExtras(context.Background(), runner, "biz", "dev", &result)
	if result.Interfaces[0].RXBitsPerSecond != nil || len(result.Health) != 0 {
		t.Fatalf("failed reads were shown as readings: %+v %+v", result.Interfaces[0], result.Health)
	}
	if result.Interfaces[0].LinkRate != "100Mbps" || result.Firmware == nil {
		t.Fatal("working extras were lost because others failed")
	}
	if len(result.Issues) != 0 {
		t.Fatalf("optional reads must not add issues: %v", result.Issues)
	}
	data, _ := json.Marshal(result)
	if strings.Contains(string(data), "secret") {
		t.Fatal("transport details leaked")
	}
}

func TestMaxModeMbpsReadsTheFastestModeAndNeverGuesses(t *testing.T) {
	cases := map[string]int{
		"10M-baseT-half,10M-baseT-full,100M-baseT-half,100M-baseT-full,1G-baseT-full": 1000,
		"100M-baseT-full;1G-baseT-full;2.5G-baseT":                                    2500,
		"10G-baseT":                  10000,
		"1G-baseX":                   1000,
		"":                           0,
		"nonsense,,;":                0,
		"M-baseT,0G-baseT,-1G-baseT": 0,
		"100M-baseT-full,wat":        100,
	}
	for in, want := range cases {
		if got := maxModeMbps(in); got != want {
			t.Errorf("%q: got %d want %d", in, got, want)
		}
	}
}

func TestParseEthernetLinksReadsNegotiationAndBothEndsModes(t *testing.T) {
	rows := parseEthernetLinks("name: ether1\nstatus: link-ok\nrate: 100Mbps\nfull-duplex: true\nauto-negotiation: done\nsupported: 100M-baseT-full,1G-baseT-full\nlink-partner-advertising: 100M-baseT-full\nname: ether2\nstatus: link-ok\nrate: 1Gbps\nauto-negotiation: banana\nsupported: 1G-baseT-full\nlink-partner-advertising: \nname: ether3\nstatus: no-link\nauto-negotiation: failed\n")
	if len(rows) != 3 {
		t.Fatalf("%+v", rows)
	}
	if rows[0].negotiation != "done" || rows[0].maxMbps != 1000 || rows[0].partnerMbps != 100 {
		t.Fatalf("ether1: %+v", rows[0])
	}
	if rows[1].negotiation != "" || rows[1].partnerMbps != 0 || rows[1].maxMbps != 1000 {
		t.Fatalf("an unrecognised negotiation text or an empty partner list must stay unknown: %+v", rows[1])
	}
	if rows[2].negotiation != "failed" {
		t.Fatalf("ether3: %+v", rows[2])
	}
}

func TestParseEthernetStatsSumsPhysicalErrorsAndNeverInventsZero(t *testing.T) {
	got := parseEthernetStats("name: ether1\nrx-fcs-error: 4\nrx-align-error: 1\nrx-fragment: 2\nrx-runt: 0\nrx-carrier-error: 3\nname: ether2\nrx-fcs-error: \nrx-align-error: nope\nname: ether3\nrx-fcs-error: 0\nname: ether4\nrx-fcs-error: 99999999999999999999999\n")
	if got["ether1"] != 10 {
		t.Fatalf("ether1: %v", got)
	}
	if _, ok := got["ether2"]; ok {
		t.Fatal("a port that reported no readable counter was shown as having zero errors")
	}
	if v, ok := got["ether3"]; !ok || v != 0 {
		t.Fatalf("a real zero must stay zero: %v", got)
	}
	if _, ok := got["ether4"]; ok {
		t.Fatal("an absurd counter was accepted")
	}
}

func TestCollectExtrasAddsNegotiationPartnerAndErrors(t *testing.T) {
	a := &DeviceApplier{}
	runner := &extraRunner{calls: map[string]int{}}
	result := baseTelemetry()
	a.collectExtras(context.Background(), runner, "biz", "dev", &result)
	e := result.Interfaces[0]
	if e.Negotiation != "done" || e.MaxRateMbps != 1000 || e.PartnerMaxMbps != 1000 || e.PhyErrors == nil || *e.PhyErrors != 5 {
		t.Fatalf("ether1: %+v", e)
	}
	if result.Interfaces[1].PhyErrors != nil || result.Interfaces[1].Negotiation != "" {
		t.Fatalf("a port the router did not report must stay empty: %+v", result.Interfaces[1])
	}
	runner2 := &extraRunner{calls: map[string]int{}, fail: map[string]bool{TelemetryEthernetStatsCommand: true}}
	failed := baseTelemetry()
	a.collectExtras(context.Background(), runner2, "biz2", "dev", &failed)
	if failed.Interfaces[0].PhyErrors != nil || failed.Interfaces[0].Negotiation != "done" {
		t.Fatalf("a failed counter read must only remove the counters: %+v", failed.Interfaces[0])
	}
	if len(failed.Issues) != 0 {
		t.Fatalf("an optional read added an issue: %v", failed.Issues)
	}
}

func TestRefusedFullEthernetReadFallsBackToTheProvenOneAndKeepsLinkRate(t *testing.T) {
	a := &DeviceApplier{}
	runner := &extraRunner{calls: map[string]int{}, fail: map[string]bool{TelemetryEthernetCommand: true}}
	result := baseTelemetry()
	a.collectExtras(context.Background(), runner, "biz", "dev", &result)
	e := result.Interfaces[0]
	if e.LinkRate != "100Mbps" || e.Negotiation != "done" || e.FullDuplex == nil {
		t.Fatalf("a refused extended read cost the port its link details: %+v", e)
	}
	if e.MaxRateMbps != 0 || e.PartnerMaxMbps != 0 {
		t.Fatalf("modes were invented without the extended read: %+v", e)
	}
	if runner.calls[TelemetryEthernetBasicCommand] != 1 {
		t.Fatalf("the proven command was not used: %v", runner.calls)
	}
	if len(result.Issues) != 0 {
		t.Fatalf("an optional read added an issue: %v", result.Issues)
	}
}
