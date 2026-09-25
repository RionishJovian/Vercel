package routeros

import (
	"context"
	"fmt"
	"time"

	"mega-magic/internal/provisioner"
)

// Prober is the REAL provisioner.Prober over SSH + RouterOS. It gathers
// handshake/traffic evidence and data-plane reachability to classify a peer.
type Prober struct {
	run        CommandRunner
	iface      string
	pingTarget string
	tDown      time.Duration
}

// NewProber builds the real prober. pingTarget is the peer address probed for
// data-plane reachability (empty => data plane is not checked).
func NewProber(run CommandRunner, iface, pingTarget string, tDown time.Duration) *Prober {
	if iface == "" {
		iface = DefaultInterface
	}
	if tDown <= 0 {
		tDown = 3 * time.Minute
	}
	return &Prober{run: run, iface: iface, pingTarget: pingTarget, tDown: tDown}
}

var _ provisioner.Prober = (*Prober)(nil)

// Observe SSHes in, reads the WireGuard interface/peer state, and pings the peer
// to produce an Observation + Connectivity.
//
// Limitation (flagged): RouterOS 7's CLI (`peers print value-list detail`) does
// not expose a handshake timestamp, so freshness is approximated from rx/tx
// traffic counters (rx+tx>0 => fresh) combined with the ping result. The
// authoritative handshake-age signal still comes from the probe/monitoring
// pipeline; this is the on-box fallback.
func (p *Prober) Observe(ctx context.Context, peerID string) (provisioner.Observation, provisioner.Connectivity, error) {
	if p.run == nil {
		return provisioner.Observation{}, provisioner.Connectivity{}, fmt.Errorf("routeros: nil command runner")
	}
	conn := provisioner.Connectivity{Checked: true, Endpoint: p.iface}

	peersOut, err := p.run.Run(ctx, fmt.Sprintf("/interface wireguard peers print value-list detail where interface=%s", p.iface))
	if err != nil {
		return provisioner.Observation{}, conn, fmt.Errorf("routeros: probe peers: %w", err)
	}
	ifaceOut, err := p.run.Run(ctx, fmt.Sprintf("/interface wireguard print value-list detail where name=%s", p.iface))
	if err != nil {
		return provisioner.Observation{}, conn, fmt.Errorf("routeros: probe interface: %w", err)
	}

	pkv := parseValueList(peersOut)
	ikv := parseValueList(ifaceOut)

	obs := provisioner.Observation{PeerID: peerID, ListenPort: atoiOr(ikv["listen-port"], 0)}
	if v := pkv["persistent-keepalive"]; v != "" {
		if d, derr := time.ParseDuration(v); derr == nil {
			obs.Keepalive = d
			obs.KeepaliveKnown = true
		}
	}

	reachable := false
	pingDetail := "no ping target configured"
	if p.pingTarget != "" {
		out, perr := p.run.Run(ctx, fmt.Sprintf("/ping %s count=2", p.pingTarget))
		if perr != nil {
			pingDetail = "ping failed: " + perr.Error()
		} else {
			reachable = pingOK(out)
			pingDetail = pingSummary(out)
		}
	}
	obs.DataPlaneUp = reachable

	fresh := hasTraffic(pkv)
	obs.State = classifyState(fresh, reachable)

	conn.Reachable = reachable
	conn.Detail = fmt.Sprintf("iface=%s listen=%d keepalive=%s rx=%s tx=%s handshake_fresh(proxy)=%v ping={%s}",
		p.iface, obs.ListenPort, pkv["persistent-keepalive"], pkv["rx"], pkv["tx"], fresh, pingDetail)
	return obs, conn, nil
}

// classifyState mirrors the probe classifier's decision table.
func classifyState(fresh, dataPlaneUp bool) string {
	switch {
	case fresh && dataPlaneUp:
		return provisioner.StateUp
	case !fresh && dataPlaneUp:
		return provisioner.StateStale
	case !fresh && !dataPlaneUp:
		return provisioner.StateOffline
	default:
		return provisioner.StateUnknown
	}
}

// hasTraffic is the freshness proxy: any rx or tx bytes implies the tunnel has
// carried traffic recently.
func hasTraffic(kv map[string]string) bool {
	return atoiOr(kv["rx"], 0) > 0 || atoiOr(kv["tx"], 0) > 0
}
