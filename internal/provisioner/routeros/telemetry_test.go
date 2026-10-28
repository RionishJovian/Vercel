package routeros

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

const telemetryResourceFixture = "uptime: 3w2d4h5m6s\ncpu-load: 0%\nfree-memory: 512.0MiB\ntotal-memory: 1GiB\nboard-name: hAP ax2\nversion: 7.24.2 (stable)\n"
const telemetryInterfaceFixture = "id: *1\nname: ether2\ntype: ether\nrunning: true\ndisabled: false\nrx-byte: 9007199254740993\ntx-byte: 0\n"

func TestTelemetryFixedReadsAndExactCounters(t *testing.T) {
	commands := []string{}
	runner := commandRunnerFunc(func(_ context.Context, command string) (string, error) {
		commands = append(commands, command)
		switch command {
		case SystemResourceCommand:
			return telemetryResourceFixture, nil
		case TelemetryInterfacesCommand:
			return telemetryInterfaceFixture, nil
		case TelemetryHotspotCountCommand:
			return "0\r\n", nil
		case TelemetryDHCPCountCommand:
			return "12", nil
		}
		t.Fatal("unexpected command", command)
		return "", nil
	})
	snapshot, err := collectTelemetry(context.Background(), runner, "device-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) != 4 || *snapshot.Resources.CPUPercent != 0 || *snapshot.HotspotSessions != 0 || *snapshot.DHCPLeases != 12 || *snapshot.Resources.MemoryUsed != 1<<29 {
		t.Fatalf("wrong readings: %+v", snapshot)
	}
	if *snapshot.Interfaces[0].RXBytes != 9007199254740993 || *snapshot.Interfaces[0].TXBytes != 0 {
		t.Fatal("counter precision lost")
	}
	data, _ := json.Marshal(snapshot)
	if !strings.Contains(string(data), `"rx_bytes":"9007199254740993"`) {
		t.Fatal(string(data))
	}
	if snapshot.ReadAt.IsZero() || len(snapshot.Issues) != 0 {
		t.Fatal("missing provenance")
	}
}
func TestTelemetryPartialMetricsNeverBecomeZero(t *testing.T) {
	runner := commandRunnerFunc(func(_ context.Context, command string) (string, error) {
		if command == SystemResourceCommand {
			return telemetryResourceFixture, nil
		}
		return "credential must never escape", errors.New("secret transport details")
	})
	snapshot, err := collectTelemetry(context.Background(), runner, "a")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Interfaces != nil || snapshot.HotspotSessions != nil || snapshot.DHCPLeases != nil || len(snapshot.Issues) != 3 {
		t.Fatal("failed metrics represented as measurements")
	}
	data, _ := json.Marshal(snapshot)
	if strings.Contains(string(data), "credential") || strings.Contains(string(data), "secret") {
		t.Fatal("transport details leaked")
	}
}
func TestTelemetryParsersRejectMalformedAndAcceptLongUptime(t *testing.T) {
	for _, value := range []string{"3w2d4h5m6s", "2d03:04:05", "365d", "00:00:00", "0s"} {
		if _, ok := telemetryUptime(value); !ok {
			t.Fatal(value)
		}
	}
	for _, value := range []string{"", "garbage1h", "1h1h", "-1s", "00:60:00", "18446744073709551615w"} {
		if _, ok := telemetryUptime(value); ok {
			t.Fatal("invalid uptime", value)
		}
	}
	for _, value := range []string{"NaN", "-1", "1MBoops", "99999999999999999999999GiB"} {
		if _, ok := telemetryBytes(value); ok {
			t.Fatal("invalid bytes", value)
		}
	}
	if _, err := parseTelemetryInterfaces(telemetryInterfaceFixture + telemetryInterfaceFixture); err == nil {
		t.Fatal("duplicate interface accepted")
	}
	if _, err := parseTelemetryInterfaces("id: *1\nname: x\npassword: secret\n"); err == nil {
		t.Fatal("unrequested property accepted")
	}
}
func TestTelemetryResolvesExactTenantAndPinnedHost(t *testing.T) {
	source := newFakeSource()
	source.put("one", "router", pwTarget("router", "192.0.2.1:22", "read-user", "test-secret"))
	calls := 0
	factory := func(config SSHConfig) CommandRunner {
		calls++
		if config.InsecureIgnoreHostKey || config.HostKeyFingerprint == "" {
			t.Fatal("pinning bypassed")
		}
		return commandRunnerFunc(func(_ context.Context, cmd string) (string, error) {
			if cmd == SystemResourceCommand {
				return telemetryResourceFixture, nil
			}
			return "", errors.New("optional menu")
		})
	}
	reader := NewDeviceApplier(source, time.Second, factory, "", 0, 0)
	if _, err := reader.ReadTelemetry(context.Background(), "two", "router"); err == nil || calls != 0 {
		t.Fatal("foreign tenant opened a transport")
	}
	if _, err := reader.ReadTelemetry(context.Background(), "one", "router"); err != nil || calls != 1 {
		t.Fatal("owned target not read", err)
	}
}
