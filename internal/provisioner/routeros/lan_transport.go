package routeros

import (
	"context"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"mega-magic/internal/lanaccess"
)

// GatewayLANTransport forwards one exact TCP destination through the owned
// gateway's pinned SSH connection. Source must be the CIDR-restricted tenant
// credential source. It never enables forwarding or alters router/firewall
// settings; forwarding-disabled routers fail closed. No direct LAN fallback.
type GatewayLANTransport struct {
	Source  TargetSource
	Timeout time.Duration
	dial    func(context.Context, string, string) (net.Conn, error) // isolated test seam
}

var _ lanaccess.Transport = GatewayLANTransport{}

func (t GatewayLANTransport) Dial(ctx context.Context, g lanaccess.Grant) (net.Conn, error) {
	now := time.Now()
	if t.Source == nil || ctx.Err() != nil || g.BusinessID == "" || g.GatewayID == "" || g.UserID == "" || g.Port == 0 ||
		!g.Target.Is4() || !g.Target.IsPrivate() || !g.Target.IsGlobalUnicast() || g.CreatedAt.IsZero() || now.Before(g.CreatedAt) ||
		!now.Before(g.ExpiresAt) || g.ExpiresAt.Sub(g.CreatedAt) > lanaccess.MaxDuration || !g.RevokedAt.IsZero() {
		return nil, lanaccess.ErrDenied
	}
	target, err := t.Source.TargetForDevice(ctx, g.BusinessID, g.GatewayID)
	if err != nil || target == nil || target.DeviceID != g.GatewayID {
		return nil, lanaccess.ErrDenied
	}
	host, _, err := net.SplitHostPort(target.Addr)
	ip, ipErr := netip.ParseAddr(host)
	if err != nil || ipErr != nil || !ip.IsPrivate() || !ip.IsGlobalUnicast() {
		return nil, lanaccess.ErrDenied
	}
	timeout := t.Timeout
	if timeout <= 0 || timeout > 15*time.Second {
		timeout = 10 * time.Second
	}
	cfg, err := SSHConfigFromTarget(target, timeout)
	if err != nil {
		return nil, lanaccess.ErrDenied
	}
	clientCfg, err := NewSSHTransport(cfg).clientConfig()
	if err != nil {
		return nil, lanaccess.ErrDenied
	}
	work, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	dial := t.dial
	if dial == nil {
		dial = (&net.Dialer{Timeout: timeout}).DialContext
	}
	raw, err := dial(work, "tcp", cfg.Addr)
	if err != nil {
		return nil, lanaccess.ErrDenied
	}
	owned := false
	defer func() {
		if !owned {
			raw.Close()
		}
	}()
	deadline, _ := work.Deadline()
	if g.ExpiresAt.Before(deadline) {
		deadline = g.ExpiresAt
	}
	if raw.SetDeadline(deadline) != nil {
		return nil, lanaccess.ErrDenied
	}
	stop := context.AfterFunc(work, func() { raw.Close() })
	defer stop()
	connection, channels, requests, err := ssh.NewClientConn(raw, cfg.Addr, clientCfg)
	if err != nil {
		return nil, lanaccess.ErrDenied
	}
	client := ssh.NewClient(connection, channels, requests)
	forward, err := client.DialContext(work, "tcp", net.JoinHostPort(g.Target.String(), strconv.Itoa(int(g.Port))))
	if err != nil || work.Err() != nil {
		if forward != nil {
			forward.Close()
		}
		client.Close()
		return nil, lanaccess.ErrDenied
	}
	// One SSH connection per stream: a socket deadline provides an independent
	// expiry fence, even if the session manager's timer is delayed.
	if raw.SetDeadline(g.ExpiresAt) != nil {
		forward.Close()
		client.Close()
		return nil, lanaccess.ErrDenied
	}
	owned = true
	return &gatewayLANConn{Conn: forward, client: client}, nil
}

type gatewayLANConn struct {
	net.Conn
	client *ssh.Client
	once   sync.Once
	err    error
}

func (c *gatewayLANConn) Close() error {
	c.once.Do(func() { c.err = c.Conn.Close(); c.client.Close() })
	return c.err
}
