package routeros

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

type networkLabPeerSource struct{}

func (networkLabPeerSource) TargetForDevice(ctx context.Context, business, device string) (*DeviceTarget, error) {
	if device != "peer-1" {
		return nil, ErrNoTarget
	}
	target, err := (capabilitySource{}).TargetForDevice(ctx, business, "dev-1")
	if err == nil {
		target.DeviceID = device
	}
	return target, err
}

// Both ports are loopback-only Docker mappings. The two guests share virtual
// Ethernet, not a customer network. An explicit launch confirmation and two
// separately pinned host identities are required before any fixture writes.
func TestNetworkCHRPeerAuthenticationAndDHCP(t *testing.T) {
	serverPin, peerPin := os.Getenv("MM_NETWORK_LAB_FINGERPRINT"), os.Getenv("MM_NETWORK_PEER_FINGERPRINT")
	if serverPin == "" || peerPin == "" {
		t.Skip("two isolated CHR guests required")
	}
	if os.Getenv("MM_NETWORK_LAB_CONFIRM") != "mm-network-independent-20261003" || os.Getenv("MM_NETWORK_PEER_CONFIRM") != "mm-network-client-20261003" || ValidateHostKeyFingerprint(serverPin) != nil || ValidateHostKeyFingerprint(peerPin) != nil {
		t.Fatal("explicit isolated guests and valid pins required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	server := NewSSHTransport(SSHConfig{Addr: "127.0.0.1:3222", User: "admin", HostKeyFingerprint: serverPin, Timeout: 10 * time.Second})
	peer := NewSSHTransport(SSHConfig{Addr: "127.0.0.1:4322", User: "admin", HostKeyFingerprint: peerPin, Timeout: 10 * time.Second})
	identity, err := server.Run(ctx, ":put [/system identity get name]")
	if err != nil || (strings.TrimSpace(identity) != "mm-background-restored" && strings.TrimSpace(identity) != "mm-network-disposable-acceptance") {
		t.Fatal("server is not the prepared disposable fixture")
	}
	identity, err = peer.Run(ctx, ":put [/system identity get name]")
	if err != nil || (strings.TrimSpace(identity) != "MikroTik" && strings.TrimSpace(identity) != "mm-network-disposable-client") {
		t.Fatal("client is not a fresh disposable fixture")
	}
	for _, command := range []string{
		`/system identity set name="mm-network-disposable-client"`,
		`:if ([:len [/ip route find where comment="mm-peer-management"]] = 0) do={ /ip route add dst-address=172.17.0.1/32 gateway=10.0.2.2%ether1 comment="mm-peer-management" }`,
		`/ip dhcp-client set [find where interface="ether1"] add-default-route=no`,
	} {
		if _, err = peer.Run(ctx, command); err != nil {
			t.Fatal("client fixture preparation failed (output withheld)")
		}
	}
	serverApplier := NewDeviceApplier(capabilitySource{}, 10*time.Second, func(SSHConfig) CommandRunner { return server }, DefaultInterface, 0, 0)
	peerApplier := NewDeviceApplier(networkLabPeerSource{}, 10*time.Second, func(SSHConfig) CommandRunner { return peer }, DefaultInterface, 0, 0)
	apply := func(a *DeviceApplier, device string, request NetworkRequest) {
		t.Helper()
		review, err := a.PreviewNetwork(ctx, "biz-1", device, request)
		if err != nil {
			t.Fatal(err)
		}
		request.Confirmation = review.Confirmation
		result, err := a.ApplyNetwork(ctx, "biz-1", device, request)
		if err != nil || !result.Verified {
			t.Fatalf("isolated %s: %+v %v", request.Kind, result, err)
		}
	}
	n, err := networkCount(ctx, peer, `/interface pppoe-client print count-only where name="mm-acceptance-peer"`)
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		apply(peerApplier, "peer-1", NetworkRequest{Kind: "pppoe_client", Values: map[string]string{"name": "acceptance-peer", "interface": "ether2", "user": "acceptance-user", "password": "disposable-test-only", "disabled": "no", "distance": "1"}})
	}
	deadline := time.Now().Add(45 * time.Second)
	connected := false
	for time.Now().Before(deadline) {
		n, err = networkCount(ctx, peer, `/interface print count-only where name="mm-acceptance-peer" running=yes`)
		if err == nil && n == 1 {
			connected = true
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !connected {
		t.Fatal("PPPoE subscriber did not authenticate on the private test Ethernet")
	}
	n, err = networkCount(ctx, server, `/ppp active print count-only where name="acceptance-user" service="pppoe"`)
	if err != nil || n != 1 {
		t.Fatal("server did not confirm the PPPoE session")
	}
	for _, probe := range []string{"1.1.1.1", "8.8.8.8"} {
		n, err = networkProbe(ctx, peer, probe)
		if err != nil || n < 1 {
			t.Fatal("authenticated PPPoE client has no verified internet replies")
		}
	}
	t.Log("PASS real PPPoE authentication, server session, and internet through the subscriber link")
	n, err = networkCount(ctx, server, `/ip dhcp-server lease print count-only where mac-address="52:54:00:22:00:03"`)
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		apply(serverApplier, "dev-1", NetworkRequest{Kind: "lease", Values: map[string]string{"address": "192.168.75.99", "mac-address": "52:54:00:22:00:03", "server": "mm-acceptance-lan", "disabled": "no"}})
	}
	if _, err = peer.Run(ctx, `:if ([:len [/ip dhcp-client find where interface="ether3"]] = 0) do={ /ip dhcp-client add interface=ether3 add-default-route=no use-peer-dns=no disabled=no }`); err != nil {
		t.Fatal("DHCP client fixture failed")
	}
	deadline = time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		address, err := peer.Run(ctx, `:put [/ip dhcp-client get [find where interface="ether3"] address]`)
		if err == nil && strings.TrimSpace(address) == "192.168.75.99/24" {
			t.Log("PASS real DHCP client received the app-created static reservation")
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("DHCP client did not receive its static reservation")
}
