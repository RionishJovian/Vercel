package routeros

import (
	"context"
	"fmt"
	"strings"
	"time"

	"mega-magic/internal/provisioner"
)

// Defaults for the live Applier.
const (
	DefaultInterface = "wg-magichub"
	DefaultKeepalive = "25s"
)

// Step is one RouterOS console action produced by translating a planned change.
// An empty Command means "no device write" (e.g. a re-probe or escalation).
type Step struct {
	Command string
	Sleep   time.Duration
	Note    string
}

// Applier is the REAL provisioner.Applier over SSH + RouterOS. It applies only
// the translated changes and can revert to a captured snapshot.
type Applier struct {
	run        CommandRunner
	iface      string
	rotatePort int
	resetHold  time.Duration
	sleep      func(time.Duration)
	now        func() time.Time
}

// NewApplier builds the real applier. rotatePort is the listen-port that
// rotate_listen_port switches to (0 => that action fails closed).
func NewApplier(run CommandRunner, iface string, rotatePort int, resetHold time.Duration) *Applier {
	if iface == "" {
		iface = DefaultInterface
	}
	return &Applier{
		run:        run,
		iface:      iface,
		rotatePort: rotatePort,
		resetHold:  resetHold,
		sleep:      time.Sleep,
		now:        func() time.Time { return time.Now().UTC() },
	}
}

var _ provisioner.Applier = (*Applier)(nil)

// Snapshot captures the pre-change state: an on-device .rsc export and a binary
// backup, plus the current WireGuard values and routes needed to revert in place.
func (a *Applier) Snapshot(ctx context.Context, peerID string, changes []provisioner.Change) (provisioner.RollbackSnapshot, error) {
	if a.run == nil {
		return provisioner.RollbackSnapshot{}, fmt.Errorf("routeros: nil command runner")
	}
	name := snapshotName(peerID, a.now())

	if _, err := a.run.Run(ctx, fmt.Sprintf("/export file=%s", name)); err != nil {
		return provisioner.RollbackSnapshot{}, fmt.Errorf("routeros: snapshot export: %w", err)
	}
	if _, err := a.run.Run(ctx, fmt.Sprintf("/system backup save name=%s", name)); err != nil {
		return provisioner.RollbackSnapshot{}, fmt.Errorf("routeros: snapshot backup: %w", err)
	}

	wgIface, err := a.run.Run(ctx, fmt.Sprintf("/interface wireguard print value-list detail where name=%s", a.iface))
	if err != nil {
		return provisioner.RollbackSnapshot{}, fmt.Errorf("routeros: snapshot wg iface: %w", err)
	}
	wgPeers, err := a.run.Run(ctx, fmt.Sprintf("/interface wireguard peers print value-list detail where interface=%s", a.iface))
	if err != nil {
		return provisioner.RollbackSnapshot{}, fmt.Errorf("routeros: snapshot wg peers: %w", err)
	}
	routes, err := a.run.Run(ctx, "/ip route print detail")
	if err != nil {
		return provisioner.RollbackSnapshot{}, fmt.Errorf("routeros: snapshot routes: %w", err)
	}

	ifaceKV := parseValueList(wgIface)
	peerKV := parseValueList(wgPeers)

	return provisioner.RollbackSnapshot{
		PeerID:               peerID,
		CapturedAt:           a.now(),
		WireGuardConfig:      strings.TrimSpace(wgIface) + "\n" + strings.TrimSpace(wgPeers),
		IPRoutes:             nonEmptyLines(routes),
		Protecting:           changes,
		ExportFile:           name + ".rsc",
		BackupName:           name,
		WGListenPort:         atoiOr(ifaceKV["listen-port"], 0),
		WGPeerKeepalive:      peerKV["persistent-keepalive"],
		WGPeerAllowedAddress: peerKV["allowed-address"],
	}, nil
}

// Apply translates and executes ONLY the given changes, in order.
func (a *Applier) Apply(ctx context.Context, _ string, changes []provisioner.Change) error {
	if a.run == nil {
		return fmt.Errorf("routeros: nil command runner")
	}
	for _, c := range changes {
		steps, err := a.Translate(c)
		if err != nil {
			return err
		}
		for _, s := range steps {
			if s.Command != "" {
				if _, err := a.run.Run(ctx, s.Command); err != nil {
					return fmt.Errorf("routeros: apply %s: %w", c.Action, err)
				}
			}
			if s.Sleep > 0 {
				a.sleep(s.Sleep)
			}
		}
	}
	return nil
}

// Translate maps one planned recovery change to RouterOS console steps. Pure.
func (a *Applier) Translate(c provisioner.Change) ([]Step, error) {
	switch c.Action {
	case provisioner.ActionEnsureKeepalive:
		return []Step{{
			Command: fmt.Sprintf("/interface wireguard peers set [find interface=%s] persistent-keepalive=%s", a.iface, DefaultKeepalive),
			Note:    "enforce 25s keepalive (spec 4.1)",
		}}, nil
	case provisioner.ActionResetPeer:
		return []Step{
			{Command: fmt.Sprintf("/interface wireguard peers disable [find interface=%s]", a.iface), Note: "disable peer to bust a stale conntrack pinhole"},
			{Sleep: a.resetHold, Note: "hold past the UDP timeout"},
			{Command: fmt.Sprintf("/interface wireguard peers enable [find interface=%s]", a.iface), Note: "re-enable the peer"},
		}, nil
	case provisioner.ActionRotateListenPort:
		if a.rotatePort <= 0 {
			return nil, fmt.Errorf("routeros: rotate_listen_port needs a target port (provisioner.applier.listen_port_rotate_to); refusing")
		}
		return []Step{{
			Command: fmt.Sprintf("/interface wireguard set [find name=%s] listen-port=%d", a.iface, a.rotatePort),
			Note:    "rotate the WireGuard listen-port",
		}}, nil
	case provisioner.ActionReprobe:
		return []Step{{Note: "re-probe handled by the verifier; no device change"}}, nil
	case provisioner.ActionReboot:
		// Fire-and-verify: reboot is destructive and cannot be config-rolled
		// back. The explicit-action consumer handles approval + post-boot
		// re-verification; this step just issues the device command.
		return []Step{{
			Command: "/system reboot",
			Note:    "force remote reboot (explicit owner action)",
		}}, nil
	case provisioner.ActionEscalate:
		return []Step{{Note: "incident escalation; no device change"}}, nil
	default:
		return nil, fmt.Errorf("routeros: unknown recovery action %q; refusing", c.Action)
	}
}

// Restore reverts the WireGuard state captured in the snapshot and re-enables
// the peer.
//
// Deviation from the letter of the design (flagged): the design prescribes
// `/import file=<snapshot>.rsc`, but RouterOS /import is NOT idempotent in place
// — re-importing a captured full export raises `Script Error: failure: already
// have ...` and silently skips the duplicate `add` lines (so peer values would
// NOT be restored), while still exiting 0. We therefore restore the captured WG
// values with targeted `set`, which is what "revert to the snapshot" actually
// requires. The .rsc/.backup artifacts remain on the device as the audit trail.
func (a *Applier) Restore(ctx context.Context, snap provisioner.RollbackSnapshot) error {
	if a.run == nil {
		return fmt.Errorf("routeros: nil command runner")
	}
	if snap.WGListenPort > 0 {
		if _, err := a.run.Run(ctx, fmt.Sprintf("/interface wireguard set [find name=%s] listen-port=%d", a.iface, snap.WGListenPort)); err != nil {
			return fmt.Errorf("routeros: restore listen-port: %w", err)
		}
	}
	if snap.WGPeerKeepalive != "" {
		if _, err := a.run.Run(ctx, fmt.Sprintf("/interface wireguard peers set [find interface=%s] persistent-keepalive=%s", a.iface, snap.WGPeerKeepalive)); err != nil {
			return fmt.Errorf("routeros: restore keepalive: %w", err)
		}
	}
	if snap.WGPeerAllowedAddress != "" {
		if _, err := a.run.Run(ctx, fmt.Sprintf("/interface wireguard peers set [find interface=%s] allowed-address=%s", a.iface, snap.WGPeerAllowedAddress)); err != nil {
			return fmt.Errorf("routeros: restore allowed-address: %w", err)
		}
	}
	// Rollback must leave the peer ENABLED (committed design requirement).
	if _, err := a.run.Run(ctx, fmt.Sprintf("/interface wireguard peers enable [find interface=%s]", a.iface)); err != nil {
		return fmt.Errorf("routeros: restore enable peer: %w", err)
	}
	return nil
}

// snapshotName builds a filesystem-safe on-device snapshot name.
func snapshotName(peerID string, t time.Time) string {
	clean := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, peerID)
	if len(clean) > 40 {
		clean = clean[:40]
	}
	if clean == "" {
		clean = "peer"
	}
	return fmt.Sprintf("mm-snap-%s-%s", clean, t.UTC().Format("20060102-150405"))
}
