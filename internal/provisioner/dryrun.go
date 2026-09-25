// Package provisioner implements the safe, side-effect-free half of the router
// recovery provisioner described in DESIGN-provisioner.md section 1 (Dry-Run
// Mode).
//
// Everything here is a *plan*: the prober gathers read-only evidence, the
// validators inspect the provisioning artifact, and PlanRecovery describes the
// changes that WOULD be applied. Nothing in this package writes to a device, a
// database, or the message bus. The apply half (sections 2-4) lands separately
// behind the Lead's manual approve-gate.
package provisioner

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Peer states understood by the recovery planner. These mirror the probe
// classifier output (internal/probe) so a plan can be built directly from an
// Evaluate/StateForPeer result. They are re-declared here to keep this package
// free of the DB/cache wiring the probe service carries.
const (
	StateUp      = "up"
	StateStale   = "stale"
	StateOffline = "offline"
	StateUnknown = "unknown"
)

// Recovery action names. These are the ladder from WIREGUARD_SELF_HEALING_SPEC
// section 4.2 step 3, plus the provisioning-plane keepalive fix from 4.1.
const (
	ActionEnsureKeepalive  = "ensure_keepalive"
	ActionReprobe          = "reprobe"
	ActionResetPeer        = "reset_peer"
	ActionRotateListenPort = "rotate_listen_port"
	ActionEscalate         = "escalate"
)

// ExpectedKeepalive is the keepalive every provisioned peer must carry
// (spec 4.1: `PersistentKeepalive = 25s`).
const ExpectedKeepalive = 25 * time.Second

// ExpectedListenPort is the Magic Hub WireGuard listener port.
const ExpectedListenPort = 13231

// ---------------------------------------------------------------------------
// Config validation
// ---------------------------------------------------------------------------

// Issue is one problem found while validating a provisioning config artifact.
type Issue struct {
	Severity string `json:"severity"` // "error" | "warning"
	Code     string `json:"code"`
	Detail   string `json:"detail"`
}

// ValidateProvisioningConfig applies the spec-4.1 invariants to the raw text of
// a RouterOS provisioning artifact (provisioning/*.rsc). It is a pure function:
// same input, same ordered output, no I/O.
//
// Rules:
//   - persistent-keepalive must be present and exactly 25s
//   - the WireGuard listen-port must be declared
//   - allowed-address must be narrowed to the hub /32 (not 0.0.0.0/0)
//   - an input-chain udp accept for the WG port must sit above any input drop
func ValidateProvisioningConfig(content []byte) []Issue {
	text := string(content)
	var issues []Issue

	// 1) PersistentKeepalive.
	if ka, ok := parseIntoSuffix(text, "persistent-keepalive="); ok {
		if ka != "25s" {
			issues = append(issues, Issue{
				Severity: "error",
				Code:     "keepalive_wrong",
				Detail:   fmt.Sprintf("persistent-keepalive is %q, want 25s (spec 4.1)", ka),
			})
		}
	} else {
		issues = append(issues, Issue{
			Severity: "error",
			Code:     "keepalive_missing",
			Detail:   "no persistent-keepalive on the WireGuard peer; tunnel can die silently (spec 4.1)",
		})
	}

	// 2) Listen-port.
	if _, ok := parseIntoSuffix(text, "listen-port="); !ok {
		issues = append(issues, Issue{
			Severity: "error",
			Code:     "listen_port_missing",
			Detail:   "no WireGuard listen-port declared",
		})
	}

	// 3) Allowed-address must be narrowed to the hub /32 (FIX-4).
	allowed := valuesAfter(text, "allowed-address=")
	if len(allowed) == 0 {
		issues = append(issues, Issue{
			Severity: "error",
			Code:     "allowed_address_missing",
			Detail:   "no allowed-address on the hub peer",
		})
	}
	for _, a := range allowed {
		if a == "0.0.0.0/0" || a == "::/0" {
			issues = append(issues, Issue{
				Severity: "error",
				Code:     "allowed_address_too_broad",
				Detail:   fmt.Sprintf("allowed-address %s routes all traffic over the hub; narrow to the hub /32 (FIX-4)", a),
			})
		}
	}

	// 4) Firewall: accept-above-drop for the WireGuard port (spec 4.1).
	if !wgAcceptAboveDrop(text) {
		issues = append(issues, Issue{
			Severity: "error",
			Code:     "wg_accept_rule_missing",
			Detail:   "no chain=input protocol=udp accept for the WG listen-port above the drop rules (spec 4.1)",
		})
	}

	return issues
}

// HasErrors reports whether any issue is severity "error".
func HasErrors(issues []Issue) bool {
	for _, i := range issues {
		if i.Severity == "error" {
			return true
		}
	}
	return false
}

// parseIntoSuffix finds the first "key=value" occurrence and returns the value
// token (trimmed of quotes and RouterOS line continuations).
func parseIntoSuffix(text, key string) (string, bool) {
	i := strings.Index(text, key)
	if i < 0 {
		return "", false
	}
	rest := text[i+len(key):]
	rest = strings.TrimLeft(rest, " \t")
	if strings.HasPrefix(rest, "\"") {
		rest = rest[1:]
		end := strings.IndexAny(rest, "\"\n\r")
		if end < 0 {
			return "", false
		}
		return rest[:end], true
	}
	end := strings.IndexAny(rest, " \t\n\r\\")
	if end < 0 {
		end = len(rest)
	}
	return rest[:end], true
}

var allowedAddrRe = regexp.MustCompile(`(?m)allowed-address=([^\s\\]+)`)

// valuesAfter returns every value for a repeated key, in file order.
func valuesAfter(text, key string) []string {
	var out []string
	if key == "allowed-address=" {
		for _, m := range allowedAddrRe.FindAllStringSubmatch(text, -1) {
			out = append(out, strings.Trim(m[1], `"`))
		}
		return out
	}
	// Fallback: single-value key.
	if v, ok := parseIntoSuffix(text, key); ok {
		out = append(out, v)
	}
	return out
}

// wgAcceptAboveDrop walks the input chain in file order and reports whether a
// udp accept for the WG listen-port appears before any input drop. It is
// deliberately line-tolerant: RouterOS scripts are edited by hand (see the
// FIX-NOTE comments in v1.1), so this errs toward accepting real-world layout.
func wgAcceptAboveDrop(text string) bool {
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.Contains(line, "chain=input") {
			continue
		}
		if strings.Contains(line, "protocol=udp") && strings.Contains(line, "action=accept") {
			if strings.Contains(line, "dst-port=13231") || strings.Contains(line, "dst-port=$") {
				return true
			}
			// A port-list accept still covers the WG port if 13231 is present.
			if strings.Contains(line, "dst-port=") && strings.Contains(line, "13231") {
				return true
			}
		}
		if strings.Contains(line, "action=drop") {
			return false
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Recovery planning
// ---------------------------------------------------------------------------

// Change is one recovery action the provisioner WOULD apply. In dry-run it is
// only ever described, never executed.
type Change struct {
	Action   string `json:"action"`
	Target   string `json:"target"`
	Current  string `json:"current"`
	Proposed string `json:"proposed"`
	Reason   string `json:"reason"`
	Risk     string `json:"risk"` // "low" | "medium" | "high"
}

// Observation is the read-only evidence gathered about a peer before planning.
// It is a plain value so plans can be unit-tested with no clock or network.
type Observation struct {
	PeerID    string
	State     string // up | stale | offline | unknown
	Keepalive time.Duration
	// KeepaliveKnown distinguishes "configured without keepalive" (Known=true,
	// Keepalive=0) from "we have no telemetry for this peer" (Known=false).
	// Without it an unprobed peer would look like a missing keepalive and
	// produce a bogus ensure_keepalive change.
	KeepaliveKnown bool
	DataPlaneUp    bool
	ListenPort     int
	// ExpectedListenPort lets a "wedged listener" be detected without inventing
	// a new state; zero falls back to ExpectedListenPort.
	ExpectedListenPort int
}

// PlanRecovery is the pure simulation: given the current evidence, it returns
// the ordered ladder of changes that WOULD be applied, stopping at the action
// expected to succeed. Ordering is deterministic and the result is never nil.
func PlanRecovery(obs Observation) []Change {
	changes := make([]Change, 0, 5)

	// Provisioning-plane fix first: a peer known to lack the 25s keepalive cannot
	// hold a NAT pinhole open (spec 4.1). An unknown keepalive is left untouched
	// rather than guessed at.
	if obs.KeepaliveKnown && obs.Keepalive != ExpectedKeepalive {
		changes = append(changes, Change{
			Action:   ActionEnsureKeepalive,
			Target:   fmt.Sprintf("peer:%s/wireguard", obs.PeerID),
			Current:  keepaliveString(obs.Keepalive),
			Proposed: "persistent-keepalive=25s",
			Reason:   "keepalive missing/mismatched; tunnel can die silently without a 25s pinhole",
			Risk:     "low",
		})
	}

	switch obs.State {
	case StateUp:
		// Healthy: nothing to recover.
	case StateStale:
		// keepalive online but no inbound reply -> stale conntrack pinhole.
		changes = append(changes, reprobeChange(obs))
		changes = append(changes, resetPeerChange(obs))
	case StateOffline:
		// No handshake, no ping: the full ladder runs before escalation.
		changes = append(changes, reprobeChange(obs))
		changes = append(changes, resetPeerChange(obs))
		changes = append(changes, rotatePortChange(obs))
		changes = append(changes, escalateChange(obs))
	case StateUnknown, "":
		// Insufficient evidence: only the cheap, safe re-probe is planned.
		changes = append(changes, reprobeChange(obs))
	default:
		changes = append(changes, reprobeChange(obs))
	}

	return changes
}

func reprobeChange(obs Observation) Change {
	return Change{
		Action:   ActionReprobe,
		Target:   fmt.Sprintf("peer:%s", obs.PeerID),
		Current:  obs.State,
		Proposed: "send fresh handshake and re-evaluate",
		Reason:   "cheapest step; clears most transient states before touching the hub",
		Risk:     "low",
	}
}

func resetPeerChange(obs Observation) Change {
	return Change{
		Action:   ActionResetPeer,
		Target:   fmt.Sprintf("peer:%s/wireguard", obs.PeerID),
		Current:  "peer enabled",
		Proposed: "disable peer for > UDP timeout, then re-enable",
		Reason:   "busts a stale conntrack pinhole (keepalive online, no reply)",
		Risk:     "medium",
	}
}

func rotatePortChange(obs Observation) Change {
	return Change{
		Action:   ActionRotateListenPort,
		Target:   fmt.Sprintf("listener:%d", listenPort(obs)),
		Current:  strconv.Itoa(listenPort(obs)),
		Proposed: "rotate the WireGuard listen-port",
		Reason:   "busts a wedged listener after reset_peer failed",
		Risk:     "medium",
	}
}

func escalateChange(obs Observation) Change {
	return Change{
		Action:   ActionEscalate,
		Target:   fmt.Sprintf("peer:%s", obs.PeerID),
		Current:  obs.State,
		Proposed: "raise incident and webhook; stop in-band recovery",
		Reason:   "no handshake and no ping; in-band recovery cannot reach a dead peer",
		Risk:     "high",
	}
}

func listenPort(obs Observation) int {
	if obs.ExpectedListenPort != 0 {
		return obs.ExpectedListenPort
	}
	if obs.ListenPort != 0 {
		return obs.ListenPort
	}
	return ExpectedListenPort
}

func keepaliveString(d time.Duration) string {
	if d == 0 {
		return "(unset)"
	}
	return d.String()
}

// ---------------------------------------------------------------------------
// Dry-run orchestration
// ---------------------------------------------------------------------------

// Connectivity is the read-only result of probing a peer.
type Connectivity struct {
	Checked   bool   `json:"checked"`
	Reachable bool   `json:"reachable"`
	Endpoint  string `json:"endpoint,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

// Report is the full dry-run output: what was checked and what WOULD change.
type Report struct {
	PeerID       string       `json:"peer_id"`
	ConfigPath   string       `json:"config_path"`
	DryRun       bool         `json:"dry_run"`
	WritesToProd bool         `json:"writes_to_prod"` // always false; asserted by tests
	ConfigValid  bool         `json:"config_valid"`
	Issues       []Issue      `json:"issues"`
	Connectivity Connectivity `json:"connectivity"`
	Changes      []Change     `json:"changes"`
	Summary      string       `json:"summary"`
}

// Prober gathers read-only evidence about a peer. Implementations MUST NOT
// mutate anything; a Prober that writes would defeat dry-run.
type Prober interface {
	Observe(ctx context.Context, peerID string) (Observation, Connectivity, error)
}

// StaticProber returns a fixed observation. Useful for CLI dry-runs against a
// peer whose telemetry is supplied out of band, and for tests.
type StaticProber struct {
	Obs  Observation
	Conn Connectivity
}

func (s StaticProber) Observe(_ context.Context, peerID string) (Observation, Connectivity, error) {
	obs := s.Obs
	obs.PeerID = peerID
	return obs, s.Conn, nil
}

// Provisioner is the dry-run planner. It holds only a Prober and an injectable
// file reader; it owns no DB, cache, or message-bus handle.
type Provisioner struct {
	probe    Prober
	readFile func(string) ([]byte, error)
}

// New builds a dry-run provisioner. If probe is nil the plan is built from an
// unknown-state observation (no network access).
func New(probe Prober) *Provisioner {
	if probe == nil {
		probe = StaticProber{Obs: Observation{State: StateUnknown}}
	}
	return &Provisioner{probe: probe, readFile: defaultReadFile}
}

// WithFileReader injects the config reader (tests pass a synthetic reader).
func (p *Provisioner) WithFileReader(f func(string) ([]byte, error)) *Provisioner {
	p.readFile = f
	return p
}

// DryRunAction implements DESIGN-provisioner.md section 1: validate the
// provisioning config file, check connectivity to the peer, and simulate the
// recovery changes. It NEVER writes to production — it only reports.
func (p *Provisioner) DryRunAction(ctx context.Context, peerID, cfgFile string) (*Report, error) {
	if peerID == "" {
		return nil, fmt.Errorf("provisioner: peer id is required")
	}

	report := &Report{
		PeerID:     peerID,
		ConfigPath: cfgFile,
		DryRun:     true,
	}

	// 1) Validate the provisioning config artifact (read-only).
	content, err := p.readFile(cfgFile)
	if err != nil {
		return nil, fmt.Errorf("provisioner: read config %s: %w", cfgFile, err)
	}
	report.Issues = ValidateProvisioningConfig(content)
	report.ConfigValid = !HasErrors(report.Issues)

	// 2) Check connectivity to the peer (read-only probe).
	obs, conn, err := p.probe.Observe(ctx, peerID)
	if err != nil {
		return nil, fmt.Errorf("provisioner: probe peer %s: %w", peerID, err)
	}
	report.Connectivity = conn
	obs.PeerID = peerID

	// 3) Simulate the recovery changes. Planning only — nothing is applied.
	report.Changes = PlanRecovery(obs)
	report.Summary = summarize(report)
	return report, nil
}

func summarize(r *Report) string {
	changes := len(r.Changes)
	switch {
	case !r.ConfigValid && changes == 0:
		return "config invalid; no recovery changes proposed"
	case !r.ConfigValid:
		return fmt.Sprintf("config invalid (%d issue(s)); %d change(s) would be applied", len(r.Issues), changes)
	case changes == 0:
		return "config valid; peer healthy, nothing to change"
	default:
		return fmt.Sprintf("config valid; %d change(s) would be applied", changes)
	}
}
