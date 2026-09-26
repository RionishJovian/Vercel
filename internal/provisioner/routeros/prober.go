package routeros

import (
	"context"
	"fmt"
	"strings"
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

// GuestQuotaState is the read-back of one hotspot user's enforced caps.
type GuestQuotaState struct {
	Identity    string
	Exists      bool
	LimitBytes  int
	LimitUptime int
	Profile     string
}

// ObserveGuestQuota reads a hotspot user back (by name) so the apply path can
// verify a guest plan's caps are actually on the router. It is read-only. A
// missing user has Exists=false (no caps applied) — verification of an enforce
// must require Exists && non-zero limits.
func (p *Prober) ObserveGuestQuota(ctx context.Context, identity string) (GuestQuotaState, error) {
	if p.run == nil {
		return GuestQuotaState{}, fmt.Errorf("routeros: nil command runner")
	}
	name := guestUserName(identity)
	out, err := p.run.Run(ctx, fmt.Sprintf("/ip hotspot user print value-list detail where name=%s", name))
	if err != nil {
		return GuestQuotaState{}, fmt.Errorf("routeros: probe hotspot user: %w", err)
	}
	kv := parseValueList(out)
	if len(kv) == 0 {
		return GuestQuotaState{Identity: identity, Exists: false}, nil
	}
	return GuestQuotaState{
		Identity:    identity,
		Exists:      true,
		LimitBytes:  atoiOr(kv["limit-bytes-total"], 0),
		LimitUptime: parseUptimeSeconds(kv["limit-uptime"]),
		Profile:     kv["profile"],
	}, nil
}

// parseUptimeSeconds converts a RouterOS uptime duration ("1h", "2h30m", "45s")
// to seconds. RouterOS prints uptime values as human durations, not raw
// seconds, so atoi would read them as 0. Returns 0 on an unparseable string.
func parseUptimeSeconds(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if n := atoiOr(s, 0); n > 0 {
		return n // already numeric
	}
	return parseDurationSeconds(s)
}

// parseDurationSeconds handles RouterOS compact durations like 1h / 2h30m / 45s.
func parseDurationSeconds(s string) int {
	var total int
	num := 0
	haveNum := false
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			num = num*10 + int(r-'0')
			haveNum = true
		case r == 'h':
			total += num * 3600
			num, haveNum = 0, false
		case r == 'm':
			total += num * 60
			num, haveNum = 0, false
		case r == 's':
			total += num
			num, haveNum = 0, false
		}
	}
	if haveNum {
		total += num // bare seconds
	}
	return total
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
