package routeros

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net"
	"net/netip"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"mega-magic/internal/lanaccess"
)

type lanSource struct {
	target *DeviceTarget
	calls  int
}

func TestGatewayLANTransportForwardsThroughPinnedGateway(t *testing.T) {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	serverCfg := &ssh.ServerConfig{PasswordCallback: func(meta ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		if meta.User() != "fixture" || string(password) != "fixture-password" {
			t.Error("wrong gateway credential")
		}
		return nil, nil
	}}
	serverCfg.AddHostKey(signer)
	done := make(chan struct{})
	go func() {
		defer close(done)
		serverSide, err := listener.Accept()
		if err != nil {
			return
		}
		defer serverSide.Close()
		conn, channels, requests, err := ssh.NewServerConn(serverSide, serverCfg)
		if err != nil {
			return
		}
		defer conn.Close()
		go ssh.DiscardRequests(requests)
		for newChannel := range channels {
			if newChannel.ChannelType() != "direct-tcpip" {
				t.Error("unexpected SSH operation")
				newChannel.Reject(ssh.Prohibited, "denied")
				continue
			}
			var destination struct {
				Address    string
				Port       uint32
				Origin     string
				OriginPort uint32
			}
			if err := ssh.Unmarshal(newChannel.ExtraData(), &destination); err != nil || destination.Address != "192.168.88.20" || destination.Port != 5000 {
				t.Error("destination expanded beyond grant")
				newChannel.Reject(ssh.Prohibited, "denied")
				continue
			}
			channel, requests, err := newChannel.Accept()
			if err != nil {
				return
			}
			go ssh.DiscardRequests(requests)
			go func() { defer channel.Close(); io.Copy(channel, channel) }()
		}
	}()
	source := &lanSource{target: &DeviceTarget{DeviceID: "gateway", Addr: "10.77.0.2:22", User: "fixture", Secret: "fixture-password", Kind: "password", Fingerprint: ssh.FingerprintSHA256(signer.PublicKey())}}
	transport := GatewayLANTransport{Source: source, Timeout: 3 * time.Second, dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		if network != "tcp" || address != "10.77.0.2:22" {
			t.Fatal("app dialed LAN directly")
		}
		return (&net.Dialer{}).DialContext(ctx, network, listener.Addr().String())
	}}
	g := lanaccess.Grant{BusinessID: "tenant", GatewayID: "gateway", UserID: "owner", Target: netip.MustParseAddr("192.168.88.20"), Port: 5000, CreatedAt: time.Now().Add(-time.Second), ExpiresAt: time.Now().Add(time.Minute)}
	conn, err := transport.Dial(context.Background(), g)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	var data [5]byte
	if _, err = io.ReadFull(conn, data[:]); err != nil || string(data[:]) != "hello" {
		t.Fatal("forwarded stream unavailable", err)
	}
	conn.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("gateway connection leaked")
	}
}

func (s *lanSource) TargetForDevice(_ context.Context, biz, device string) (*DeviceTarget, error) {
	s.calls++
	if biz != "tenant" || device != "gateway" {
		panic("untrusted credential scope")
	}
	return s.target, nil
}
func TestGatewayLANTransportDeniesUnsafeTargets(t *testing.T) {
	g := lanaccess.Grant{BusinessID: "tenant", GatewayID: "gateway", UserID: "owner", Target: netip.MustParseAddr("192.168.88.20"), Port: 5000, CreatedAt: time.Now().Add(-time.Second), ExpiresAt: time.Now().Add(time.Minute)}
	for _, target := range []*DeviceTarget{nil, {DeviceID: "foreign"}, {DeviceID: "gateway", Addr: "8.8.8.8:22"}, {DeviceID: "gateway", Addr: "127.0.0.1:22"}, {DeviceID: "gateway", Addr: "gateway.local:22"}, {DeviceID: "gateway", Addr: "10.77.0.2:22", User: "user", Secret: "secret", Kind: "password"}} {
		s := &lanSource{target: target}
		if conn, err := (GatewayLANTransport{Source: s}).Dial(context.Background(), g); err == nil || conn != nil {
			t.Fatal("unsafe gateway used")
		}
		if s.calls != 1 {
			t.Fatal("credential resolver bypassed")
		}
	}
	for _, ip := range []string{"8.8.8.8", "127.0.0.1", "169.254.169.254", "::1"} {
		s := &lanSource{}
		bad := g
		bad.Target = netip.MustParseAddr(ip)
		if _, err := (GatewayLANTransport{Source: s}).Dial(context.Background(), bad); err == nil || s.calls != 0 {
			t.Fatal("unsafe LAN destination reached source")
		}
	}
}
