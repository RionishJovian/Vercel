package routeros

import (
	"context"
	"errors"
	"testing"
	"time"
)

type capabilitySource struct{}

func (capabilitySource) TargetForDevice(_ context.Context, businessID, deviceID string) (*DeviceTarget, error) {
	if businessID != "biz-1" || deviceID != "dev-1" {
		return nil, ErrNoTarget
	}
	return &DeviceTarget{
		DeviceID: deviceID, Addr: "10.99.0.3:22", User: "magic", Secret: "secret", Kind: "password",
		Fingerprint: "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	}, nil
}

type capabilityRunner struct {
	outputs map[string]string
	errors  map[string]error
}

func (r *capabilityRunner) Run(_ context.Context, command string) (string, error) {
	if err := r.errors[command]; err != nil {
		return "", err
	}
	return r.outputs[command], nil
}

func TestInspectCapabilitiesSeparatesGatewayFromWireless(t *testing.T) {
	runner := &capabilityRunner{outputs: map[string]string{
		SystemResourceCommand:                                 "version: 7.15.3 (stable)\narchitecture-name: x86_64\nboard-name: CHR\n",
		"/ip hotspot print count-only":                        "1",
		"/ip dhcp-server print count-only":                    "1",
		"/radius print count-only":                            "2",
		"/interface wifi print count-only":                    "",
		"/interface wireless print count-only":                "",
		"/interface wifi capsman remote-cap print count-only": "0",
		"/caps-man remote-cap print count-only":               "0",
		"/ip service print value-list detail":                 "name: www\ndisabled: true\nport: 80\n\nname: www-ssl\ndisabled: false\naddress: 10.99.0.0/24\nport: 443\n",
	}, errors: map[string]error{
		"/interface wifi print count-only":     errors.New("bad command name wifi"),
		"/interface wireless print count-only": errors.New("bad command name wireless"),
	}}
	probe := NewDeviceProber(capabilitySource{}, time.Second, func(SSHConfig) CommandRunner { return runner })

	caps, err := probe.InspectCapabilities(context.Background(), "biz-1", "dev-1")
	if err != nil {
		t.Fatal(err)
	}
	if !caps.Gateway || !caps.Hotspot.Supported || !caps.Hotspot.Configured || !caps.DHCPServer.Configured || caps.Radius.Count != 2 {
		t.Fatalf("gateway services not discovered: %+v", caps)
	}
	if caps.ModernWiFi.Supported || caps.LegacyWireless.Supported {
		t.Fatalf("CHR must not invent a local radio: %+v", caps)
	}
	if !caps.ExternalAPsLikely {
		t.Fatalf("gateway without radio should report external AP topology: %+v", caps)
	}
	if !caps.WebFig.HTTPSEnabled || caps.WebFig.HTTPSPort != 443 || !caps.WebFig.TrustedOnly || caps.WebFig.HTTPEnabled {
		t.Fatalf("WebFig services parsed incorrectly: %+v", caps.WebFig)
	}
}

func TestInspectCapabilitiesDoesNotGateOnBoardName(t *testing.T) {
	runner := &capabilityRunner{outputs: map[string]string{
		SystemResourceCommand:                                 "version: 7.18.2 (stable)\narchitecture-name: arm64\nboard-name: Example Future RouterBOARD\n",
		"/ip hotspot print count-only":                        "0",
		"/ip dhcp-server print count-only":                    "0",
		"/radius print count-only":                            "0",
		"/interface wifi print count-only":                    "2",
		"/interface wireless print count-only":                "0",
		"/interface wifi capsman remote-cap print count-only": "0",
		"/caps-man remote-cap print count-only":               "0",
	}}
	probe := NewDeviceProber(capabilitySource{}, time.Second, func(SSHConfig) CommandRunner { return runner })

	caps, err := probe.InspectCapabilities(context.Background(), "biz-1", "dev-1")
	if err != nil {
		t.Fatal(err)
	}
	if !caps.Gateway || !caps.ModernWiFi.Supported || caps.ModernWiFi.Count != 2 {
		t.Fatalf("unknown model must still be capability-detected: %+v", caps)
	}
	if caps.BoardName != "Example Future RouterBOARD" {
		t.Fatalf("board name should be descriptive only, got %q", caps.BoardName)
	}
}
