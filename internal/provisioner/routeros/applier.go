package routeros

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
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
func (a *Applier) Snapshot(ctx context.Context, _, peerID string, changes []provisioner.Change) (provisioner.RollbackSnapshot, error) {
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

	var services []provisioner.RouterOSServiceSnapshot
	if containsAction(changes, provisioner.ActionHardenManagement) {
		serviceOut, serviceErr := a.run.Run(ctx, "/ip service print value-list detail")
		if serviceErr != nil {
			return provisioner.RollbackSnapshot{}, fmt.Errorf("routeros: snapshot services: %w", serviceErr)
		}
		services = parseServiceSnapshots(serviceOut)
		if len(services) == 0 {
			return provisioner.RollbackSnapshot{}, fmt.Errorf("routeros: snapshot services returned no records")
		}
	}

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
		RouterOSServices:     services,
	}, nil
}

// Apply translates and executes ONLY the given changes, in order.
func (a *Applier) Apply(ctx context.Context, _, _ string, changes []provisioner.Change) error {
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
	case provisioner.ActionEnforceGuestQuota:
		// Apply (or refresh) a guest's bought-plan hotspot-user caps. Target =
		// the guest identity (MAC / phone); Proposed = JSON GuestQuota. Fail
		// closed on a missing identity or malformed/empty limits.
		identity := strings.TrimSpace(c.Target)
		if identity == "" {
			return nil, fmt.Errorf("routeros: enforce_guest_quota needs a guest identity in Target; refusing")
		}
		q, err := parseGuestQuota(c.Proposed)
		if err != nil {
			return nil, err
		}
		if q.DataBytes < 0 || q.UptimeSeconds < 0 {
			return nil, fmt.Errorf("routeros: enforce_guest_quota has negative limit; refusing")
		}
		if q.DataBytes == 0 && q.UptimeSeconds == 0 {
			return nil, fmt.Errorf("routeros: enforce_guest_quota with zero data and zero time caps nothing; refusing")
		}
		user := guestUserName(identity)
		profile := "default"
		if q.Profile != "" {
			profile = q.Profile
		}
		// RouterOS limit-uptime is in SECONDS (not a duration string), and
		// limit-bytes-total is in bytes. Both converge regardless of prior state:
		// /set on a missing user no-ops, /add on an existing name fails — so we
		// add then set, which is idempotent.
		return []Step{
			{Command: fmt.Sprintf("/ip hotspot user add name=%s profile=%s", user, profile), Note: "ensure hotspot user exists"},
			{Command: fmt.Sprintf("/ip hotspot user set [find name=%s] limit-uptime=%d limit-bytes-total=%d", user, q.UptimeSeconds, q.DataBytes), Note: "enforce data+time caps on the guest"},
		}, nil
	case provisioner.ActionRevokeGuestQuota:
		identity := strings.TrimSpace(c.Target)
		if identity == "" {
			return nil, fmt.Errorf("routeros: revoke_guest_quota needs a guest identity; refusing")
		}
		// Remove the SAME sanitized name enforce created (guestUserName), not the
		// raw identity — otherwise revoke misses the user it added.
		return []Step{{
			Command: fmt.Sprintf("/ip hotspot user remove [find name=%s]", guestUserName(identity)),
			Note:    "remove the guest hotspot user (release caps)",
		}}, nil
	case provisioner.ActionReboot:
		// Fire-and-verify: reboot is destructive and cannot be config-rolled
		// back. The explicit-action consumer handles approval + post-boot
		// re-verification; this step just issues the device command.
		return []Step{{
			Command: "/system reboot",
			Note:    "force remote reboot (explicit owner action)",
		}}, nil
	case provisioner.ActionHardenManagement:
		var proposal struct {
			TrustedCIDRs []string `json:"trusted_cidrs"`
		}
		if err := json.Unmarshal([]byte(c.Proposed), &proposal); err != nil {
			return nil, fmt.Errorf("routeros: harden_management invalid proposal: %w", err)
		}
		trusted, err := validatedCIDRList(proposal.TrustedCIDRs)
		if err != nil {
			return nil, err
		}
		return []Step{
			{Command: "/ip service disable [find where name=telnet]", Note: "disable plaintext telnet management"},
			{Command: "/ip service disable [find where name=ftp]", Note: "disable FTP management"},
			{Command: "/ip service disable [find where name=www]", Note: "disable HTTP WebFig"},
			{Command: "/ip service disable [find where name=api]", Note: "disable plaintext RouterOS API"},
			{Command: "/ip service disable [find where name=api-ssl]", Note: "disable externally exposed API-SSL"},
			{Command: fmt.Sprintf("/ip service set [find where name=ssh] address=%s disabled=no", trusted), Note: "retain SSH only on management networks"},
			{Command: fmt.Sprintf("/ip service set [find where name=winbox] address=%s disabled=no", trusted), Note: "retain Winbox only on management networks"},
		}, nil
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
func (a *Applier) Restore(ctx context.Context, _ string, snap provisioner.RollbackSnapshot) error {
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
	for _, service := range snap.RouterOSServices {
		if !validServiceName(service.Name) {
			return fmt.Errorf("routeros: restore service has invalid name %q", service.Name)
		}
		address := strings.TrimSpace(service.Address)
		if address == "" {
			address = `""`
		} else if _, err := validatedCIDRList(strings.Split(address, ",")); err != nil {
			return fmt.Errorf("routeros: restore service %s: %w", service.Name, err)
		}
		disabled := "no"
		if service.Disabled {
			disabled = "yes"
		}
		cmd := fmt.Sprintf("/ip service set [find where name=%s] address=%s disabled=%s", service.Name, address, disabled)
		if _, err := a.run.Run(ctx, cmd); err != nil {
			return fmt.Errorf("routeros: restore service %s: %w", service.Name, err)
		}
	}
	// Rollback must leave the peer ENABLED (committed design requirement).
	if _, err := a.run.Run(ctx, fmt.Sprintf("/interface wireguard peers enable [find interface=%s]", a.iface)); err != nil {
		return fmt.Errorf("routeros: restore enable peer: %w", err)
	}
	return nil
}

func containsAction(changes []provisioner.Change, action string) bool {
	for _, change := range changes {
		if change.Action == action {
			return true
		}
	}
	return false
}

func validatedCIDRList(cidrs []string) (string, error) {
	if len(cidrs) == 0 {
		return "", fmt.Errorf("routeros: harden_management needs at least one trusted management CIDR; refusing")
	}
	clean := make([]string, 0, len(cidrs))
	for _, raw := range cidrs {
		cidr := strings.TrimSpace(raw)
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return "", fmt.Errorf("routeros: invalid management CIDR %q; refusing", raw)
		}
		clean = append(clean, cidr)
	}
	return strings.Join(clean, ","), nil
}

func validServiceName(name string) bool {
	switch name {
	case "telnet", "ftp", "www", "www-ssl", "ssh", "api", "winbox", "api-ssl":
		return true
	default:
		return false
	}
}

func parseServiceSnapshots(out string) []provisioner.RouterOSServiceSnapshot {
	services := make([]provisioner.RouterOSServiceSnapshot, 0, 8)
	current := map[string]string{}
	flush := func() {
		name := strings.TrimSpace(current["name"])
		if validServiceName(name) {
			disabled := strings.TrimSpace(current["disabled"])
			services = append(services, provisioner.RouterOSServiceSnapshot{
				Name:     name,
				Address:  strings.TrimSpace(current["address"]),
				Disabled: strings.EqualFold(disabled, "true") || strings.EqualFold(disabled, "yes"),
			})
		}
		current = map[string]string{}
	}
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			if len(current) > 0 {
				flush()
			}
			continue
		}
		i := strings.Index(line, ":")
		if i < 0 {
			continue
		}
		key, value := strings.TrimSpace(line[:i]), strings.TrimSpace(line[i+1:])
		// Some RouterOS terminals omit blank lines between value-list records.
		if key == "name" && current["name"] != "" {
			flush()
		}
		current[key] = value
	}
	if len(current) > 0 {
		flush()
	}
	return services
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
