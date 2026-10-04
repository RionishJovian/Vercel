package routeros

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type quickConfigRunner struct {
	identity    string
	dns         string
	allowRemote bool
	commands    []string
	failDNSOnce bool
	dnsGuards   map[string]bool
}

func (r *quickConfigRunner) Run(_ context.Context, command string) (string, error) {
	r.commands = append(r.commands, command)
	switch {
	case command == "/system identity print":
		return "name: " + r.identity + "\n", nil
	case command == "/ip dns print":
		remote := "no"
		if r.allowRemote {
			remote = "yes"
		}
		return "servers: " + r.dns + "\nallow-remote-requests: " + remote + "\n", nil
	case strings.HasPrefix(command, "/system identity set name="):
		r.identity = strings.Trim(command[strings.Index(command, "=")+1:], `"`)
		return "", nil
	case strings.HasPrefix(command, "/ip dns set servers="):
		if r.failDNSOnce {
			r.failDNSOnce = false
			return "", errors.New("simulated DNS write failure")
		}
		start := strings.Index(command, `servers="`) + len(`servers="`)
		end := strings.Index(command[start:], `"`)
		r.dns = command[start : start+end]
		r.allowRemote = strings.HasSuffix(command, "allow-remote-requests=yes")
		return "", nil
	case strings.HasPrefix(command, "/export file=") || strings.HasPrefix(command, "/system backup save name="):
		return "", nil
	case strings.HasPrefix(command, "/ip/firewall/filter remove [find where comment="):
		if idx := strings.Index(command, "protocol="); idx >= 0 {
			proto := command[idx+len("protocol="):]
			delete(r.dnsGuards, proto)
		}
		return "", nil
	case strings.HasPrefix(command, "/ip/firewall/filter add chain=input in-interface-list=WAN protocol="):
		proto := strings.TrimPrefix(command, "/ip/firewall/filter add chain=input in-interface-list=WAN protocol=")
		proto = proto[:strings.Index(proto, " ")]
		if r.dnsGuards == nil {
			r.dnsGuards = map[string]bool{}
		}
		r.dnsGuards[proto] = true
		return "", nil
	case strings.HasPrefix(command, "/ip/firewall/filter print count-only where comment="):
		if idx := strings.Index(command, "protocol="); idx >= 0 && r.dnsGuards[command[idx+len("protocol="):]] {
			return "1\n", nil
		}
		return "0\n", nil
	default:
		return "", errors.New("unexpected command: " + command)
	}
}

func quickConfigApplier(runner CommandRunner) *DeviceApplier {
	return NewDeviceApplier(capabilitySource{}, time.Second, func(SSHConfig) CommandRunner { return runner }, DefaultInterface, 0, 0)
}

func TestApplyQuickConfigBacksUpWritesAndVerifies(t *testing.T) {
	runner := &quickConfigRunner{identity: "old-router", dns: "1.1.1.1", allowRemote: false}
	applier := quickConfigApplier(runner)
	want := QuickConfig{Identity: "Lobby Gateway", DNSServers: []string{"8.8.8.8", "1.1.1.1"}, AllowRemoteDNS: true}

	result, err := applier.ApplyQuickConfig(context.Background(), "biz-1", "dev-1", want)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Verified || result.RolledBack || result.Backup == "" {
		t.Fatalf("unexpected result: %+v", result)
	}
	joined := strings.Join(runner.commands, "\n")
	for _, required := range []string{"/export file=", "/system backup save name=", `/system identity set name="Lobby Gateway"`, `/ip dns set servers="8.8.8.8,1.1.1.1" allow-remote-requests=yes`} {
		if !strings.Contains(joined, required) {
			t.Fatalf("missing command %q in:\n%s", required, joined)
		}
	}
	// Allowing remote DNS without dropping WAN port 53 makes the router an open
	// resolver, so both protocols must be guarded.
	for _, proto := range []string{"udp", "tcp"} {
		want := "protocol=" + proto + " dst-port=53 action=drop"
		if !strings.Contains(joined, want) {
			t.Fatalf("missing remote DNS guard for %s in:\n%s", proto, joined)
		}
	}
	if !runner.dnsGuards["udp"] || !runner.dnsGuards["tcp"] {
		t.Fatalf("remote DNS guard not applied for both protocols: %v", runner.dnsGuards)
	}
}

func TestApplyQuickConfigSkipsDNSGuardWhenRemoteDisabled(t *testing.T) {
	runner := &quickConfigRunner{identity: "old-router", dns: "1.1.1.1"}
	applier := quickConfigApplier(runner)
	want := QuickConfig{Identity: "Lobby Gateway", DNSServers: []string{"1.1.1.1"}, AllowRemoteDNS: false}

	if _, err := applier.ApplyQuickConfig(context.Background(), "biz-1", "dev-1", want); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(runner.commands, "\n"), "/ip/firewall/filter add") {
		t.Fatal("DNS guard must not be applied when remote requests are disabled")
	}
}

func TestApplyQuickConfigRollsBackTargetedValuesOnWriteFailure(t *testing.T) {
	runner := &quickConfigRunner{identity: "old-router", dns: "1.1.1.1", failDNSOnce: true}
	applier := quickConfigApplier(runner)

	result, err := applier.ApplyQuickConfig(context.Background(), "biz-1", "dev-1", QuickConfig{
		Identity: "new-router", DNSServers: []string{"9.9.9.9"}, AllowRemoteDNS: true,
	})
	if err == nil {
		t.Fatal("expected failed DNS write")
	}
	if !result.RolledBack || runner.identity != "old-router" || runner.dns != "1.1.1.1" || runner.allowRemote {
		t.Fatalf("targeted rollback failed: result=%+v runner=%+v", result, runner)
	}
}

func TestValidateQuickConfigRejectsInjectionAndBadDNS(t *testing.T) {
	if _, err := ValidateQuickConfig(QuickConfig{Identity: `router"; /system reboot`, DNSServers: []string{"1.1.1.1"}}); err == nil {
		t.Fatal("RouterOS command injection-shaped identity must be rejected")
	}
	if _, err := ValidateQuickConfig(QuickConfig{Identity: "safe-router", DNSServers: []string{"not-an-ip"}}); err == nil {
		t.Fatal("invalid DNS address must be rejected")
	}
}
