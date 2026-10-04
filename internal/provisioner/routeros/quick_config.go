package routeros

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"
	"time"
)

var routerIdentityPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{0,62}$`)

// QuickConfig is the first guarded RouterOS write surface exposed by the web
// console. These fields are useful on every gateway class (physical RouterBOARD
// or CHR) and do not assume that a wireless radio exists.
type QuickConfig struct {
	Identity       string   `json:"identity"`
	DNSServers     []string `json:"dns_servers"`
	AllowRemoteDNS bool     `json:"allow_remote_dns"`
}

type QuickConfigResult struct {
	DeviceID   string      `json:"device_id"`
	Backup     string      `json:"backup"`
	Before     QuickConfig `json:"before"`
	After      QuickConfig `json:"after"`
	Verified   bool        `json:"verified"`
	RolledBack bool        `json:"rolled_back"`
}

// ValidateQuickConfig exposes the same normalization used by the applier so an
// HTTP handler can reject invalid input before opening a router connection.
func ValidateQuickConfig(cfg QuickConfig) (QuickConfig, error) { return normalizeQuickConfig(cfg) }

func normalizeQuickConfig(cfg QuickConfig) (QuickConfig, error) {
	cfg.Identity = strings.TrimSpace(cfg.Identity)
	if !routerIdentityPattern.MatchString(cfg.Identity) {
		return QuickConfig{}, fmt.Errorf("router identity must be 1-63 letters, numbers, spaces, dots, underscores or dashes")
	}
	seen := map[string]struct{}{}
	servers := make([]string, 0, len(cfg.DNSServers))
	for _, raw := range cfg.DNSServers {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		ip := net.ParseIP(value)
		if ip == nil {
			return QuickConfig{}, fmt.Errorf("invalid DNS server %q", value)
		}
		canonical := ip.String()
		if _, ok := seen[canonical]; ok {
			continue
		}
		seen[canonical] = struct{}{}
		servers = append(servers, canonical)
	}
	if len(servers) > 8 {
		return QuickConfig{}, fmt.Errorf("at most 8 DNS servers are allowed")
	}
	cfg.DNSServers = servers
	return cfg, nil
}

func routerOSQuoted(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	value = strings.ReplaceAll(value, `$`, `\$`)
	return `"` + value + `"`
}

func quickConfigFromRunner(ctx context.Context, runner CommandRunner) (QuickConfig, error) {
	// Both menus are RouterOS singleton menus. They reject the value-list
	// modifier on real RouterOS 7 devices, even though list menus accept it.
	identityOut, err := runner.Run(ctx, "/system identity print")
	if err != nil {
		return QuickConfig{}, fmt.Errorf("read router identity: %w", err)
	}
	dnsOut, err := runner.Run(ctx, "/ip dns print")
	if err != nil {
		return QuickConfig{}, fmt.Errorf("read DNS configuration: %w", err)
	}
	identity := parseValueList(identityOut)
	dns := parseValueList(dnsOut)
	servers := []string{}
	for _, value := range strings.Split(dns["servers"], ",") {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			servers = append(servers, trimmed)
		}
	}
	return normalizeQuickConfig(QuickConfig{
		Identity:       identity["name"],
		DNSServers:     servers,
		AllowRemoteDNS: strings.EqualFold(dns["allow-remote-requests"], "yes") || strings.EqualFold(dns["allow-remote-requests"], "true"),
	})
}

func quickConfigEqual(a, b QuickConfig) bool {
	if a.Identity != b.Identity || a.AllowRemoteDNS != b.AllowRemoteDNS || len(a.DNSServers) != len(b.DNSServers) {
		return false
	}
	ac, bc := append([]string(nil), a.DNSServers...), append([]string(nil), b.DNSServers...)
	sort.Strings(ac)
	sort.Strings(bc)
	for i := range ac {
		if ac[i] != bc[i] {
			return false
		}
	}
	return true
}

func applyQuickConfigCommands(ctx context.Context, runner CommandRunner, cfg QuickConfig) error {
	remote := "no"
	if cfg.AllowRemoteDNS {
		remote = "yes"
	}
	if _, err := runner.Run(ctx, "/system identity set name="+routerOSQuoted(cfg.Identity)); err != nil {
		return fmt.Errorf("set router identity: %w", err)
	}
	servers := strings.Join(cfg.DNSServers, ",")
	if _, err := runner.Run(ctx, "/ip dns set servers="+routerOSQuoted(servers)+" allow-remote-requests="+remote); err != nil {
		return fmt.Errorf("set DNS configuration: %w", err)
	}
	if err := guardRemoteDNS(ctx, runner, remote == "yes"); err != nil {
		return err
	}
	return nil
}

// guardRemoteDNS drops inbound DNS from untrusted interfaces whenever the router
// answers remote queries. RouterOS ships with a default firewall that does this,
// but this applier writes its own filter rules and a custom input rule could
// leave port 53 open to the internet - an open resolver. The rules are
// identified by their comment so they can be re-applied without duplicating.
func guardRemoteDNS(ctx context.Context, runner CommandRunner, remote bool) error {
	if !remote {
		return nil
	}
	const guardComment = "mm-guard-drop-remote-dns"
	for _, protocol := range []string{"udp", "tcp"} {
		selector := "where comment=" + routerOSQuoted(guardComment) + " and protocol=" + protocol
		if _, err := runner.Run(ctx, "/ip/firewall/filter remove [find "+selector+"]"); err != nil {
			return fmt.Errorf("clear remote DNS guard: %w", err)
		}
		command := "/ip/firewall/filter add chain=input in-interface-list=WAN protocol=" + protocol +
			" dst-port=53 action=drop comment=" + routerOSQuoted(guardComment)
		if _, err := runner.Run(ctx, command); err != nil {
			return fmt.Errorf("remote DNS guard failed for %s; router may be an open resolver", protocol)
		}
		count, err := networkCount(ctx, runner, "/ip/firewall/filter print count-only "+selector)
		if err != nil || count != 1 {
			return fmt.Errorf("remote DNS guard not verified for %s", protocol)
		}
	}
	return nil
}

// ReadQuickConfig reads the current values from the exact tenant/device target.
func (a *DeviceApplier) ReadQuickConfig(ctx context.Context, businessID, peerID string) (QuickConfig, error) {
	runner, err := runnerForDevice(ctx, a.src, a.factory, a.timeout, businessID, peerID)
	if err != nil {
		return QuickConfig{}, err
	}
	return quickConfigFromRunner(ctx, runner)
}

// ApplyQuickConfig backs up, applies, reads back and verifies. A failed write or
// mismatch restores the two targeted settings immediately; the on-device export
// and binary backup remain as a further recovery point.
func (a *DeviceApplier) ApplyQuickConfig(ctx context.Context, businessID, peerID string, requested QuickConfig) (QuickConfigResult, error) {
	want, err := normalizeQuickConfig(requested)
	if err != nil {
		return QuickConfigResult{}, err
	}
	runner, err := runnerForDevice(ctx, a.src, a.factory, a.timeout, businessID, peerID)
	if err != nil {
		return QuickConfigResult{}, err
	}
	before, err := quickConfigFromRunner(ctx, runner)
	if err != nil {
		return QuickConfigResult{}, err
	}
	name := snapshotName(peerID+"-quick-config", time.Now().UTC())
	if _, err := runner.Run(ctx, fmt.Sprintf("/export file=%s", name)); err != nil {
		return QuickConfigResult{}, fmt.Errorf("quick config export backup: %w", err)
	}
	if _, err := runner.Run(ctx, fmt.Sprintf("/system backup save name=%s", name)); err != nil {
		return QuickConfigResult{}, fmt.Errorf("quick config binary backup: %w", err)
	}
	result := QuickConfigResult{DeviceID: peerID, Backup: name, Before: before}
	rollback := func() {
		if rollbackErr := applyQuickConfigCommands(ctx, runner, before); rollbackErr == nil {
			result.RolledBack = true
		}
	}
	if err := applyQuickConfigCommands(ctx, runner, want); err != nil {
		rollback()
		return result, err
	}
	after, err := quickConfigFromRunner(ctx, runner)
	if err != nil {
		rollback()
		return result, fmt.Errorf("verify quick config: %w", err)
	}
	result.After = after
	result.Verified = quickConfigEqual(want, after)
	if !result.Verified {
		rollback()
		return result, fmt.Errorf("quick config verification mismatch; previous settings restored")
	}
	return result, nil
}
