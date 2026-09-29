package provisioner

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"
)

// defaultReadFile is the production file reader (read-only).
func defaultReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// TCPProber checks connectivity by opening a short-lived TCP connection to the
// peer's management endpoint and closing it immediately. It is strictly
// read-only: it sends no payload and performs no handshake beyond TCP.
//
// Caveat: TCP reachability is not proof of a healthy WireGuard data plane. It
// is a cheap "is anything listening" signal; the authoritative state still
// comes from the probe/monitoring pipeline. When the dial succeeds the peer is
// treated as up, otherwise offline.
type TCPProber struct {
	Endpoint string
	Timeout  time.Duration
}

func (t TCPProber) Observe(ctx context.Context, _, peerID string) (Observation, Connectivity, error) {
	timeout := t.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	conn := Connectivity{Checked: true, Endpoint: t.Endpoint}

	d := net.Dialer{Timeout: timeout}
	c, err := d.DialContext(ctx, "tcp", t.Endpoint)
	if err != nil {
		conn.Reachable = false
		conn.Detail = "tcp dial failed: " + err.Error()
		return Observation{PeerID: peerID, State: StateOffline}, conn, nil
	}
	_ = c.Close()
	conn.Reachable = true
	conn.Detail = "tcp endpoint reachable (read-only dial)"

	// A successful TCP dial proves the endpoint answers; it does NOT prove the
	// peer's keepalive is configured, so KeepaliveKnown stays false.
	return Observation{
		PeerID:      peerID,
		State:       StateUp,
		DataPlaneUp: true,
	}, conn, nil
}

// ResolveEndpoint decides which TCP endpoint to probe for a peer. An explicit
// addr always wins. Otherwise, a --peer value that is an IP or already carries a
// port is used; a bare identifier (e.g. a UUID) yields no endpoint, and the
// caller should report connectivity as not checked.
func ResolveEndpoint(peer, addr string) (string, bool) {
	if addr != "" {
		if _, _, err := net.SplitHostPort(addr); err == nil {
			return addr, true
		}
		return net.JoinHostPort(addr, "443"), true
	}
	if peer == "" {
		return "", false
	}
	if host, port, err := net.SplitHostPort(peer); err == nil {
		return net.JoinHostPort(host, port), true
	}
	if net.ParseIP(peer) != nil {
		return net.JoinHostPort(peer, "443"), true
	}
	return "", false
}

// UncheckedConnectivity is the explicit "we could not probe" result, kept
// distinct from "probed and unreachable" so a dry-run never implies a false
// negative about the peer.
func UncheckedConnectivity(peer string) Connectivity {
	return Connectivity{
		Checked:   false,
		Reachable: false,
		Detail:    fmt.Sprintf("no probe endpoint resolvable for peer %q; pass --peer-addr host:port", peer),
	}
}
