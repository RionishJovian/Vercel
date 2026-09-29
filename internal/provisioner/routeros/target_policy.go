package routeros

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
)

// ErrTargetNotAllowed means a database-resolved SSH address is outside the
// operator's explicit management-network allowlist. It is distinct from
// ErrNoTarget: the device is configured, but policy refuses to dial it.
var ErrTargetNotAllowed = errors.New("routeros: ssh target is outside allowed management networks")

// CIDRTargetSource adds an outbound network boundary to another TargetSource.
// Host-key pinning authenticates the host after dialing; this policy decides
// whether the backend may dial that address at all.
type CIDRTargetSource struct {
	next    TargetSource
	allowed []*net.IPNet
}

// NewCIDRTargetSource parses an explicit allowlist. Empty lists and default
// routes are refused because either would turn the control plane into a general
// purpose SSH client if a device row were compromised.
func NewCIDRTargetSource(next TargetSource, cidrs []string) (*CIDRTargetSource, error) {
	if next == nil {
		return nil, fmt.Errorf("routeros: target policy needs a target source")
	}
	if len(cidrs) == 0 {
		return nil, fmt.Errorf("routeros: allowed target CIDRs are required")
	}
	policy := &CIDRTargetSource{next: next}
	for _, raw := range cidrs {
		raw = strings.TrimSpace(raw)
		ip, network, err := net.ParseCIDR(raw)
		if err != nil {
			return nil, fmt.Errorf("routeros: invalid allowed target CIDR %q: %w", raw, err)
		}
		ones, _ := network.Mask.Size()
		if ones == 0 {
			return nil, fmt.Errorf("routeros: default-route target CIDR %q is forbidden", raw)
		}
		// ParseCIDR returns both the supplied address and its normalized network.
		// Require canonical notation so a typo like 10.77.0.4/24 is visible in
		// review rather than silently becoming 10.77.0.0/24.
		if !ip.Equal(network.IP) {
			return nil, fmt.Errorf("routeros: allowed target CIDR %q is not a canonical network", raw)
		}
		policy.allowed = append(policy.allowed, network)
	}
	return policy, nil
}

func (s *CIDRTargetSource) TargetForDevice(ctx context.Context, businessID, deviceID string) (*DeviceTarget, error) {
	target, err := s.next.TargetForDevice(ctx, businessID, deviceID)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, ErrNoTarget
	}
	host, _, err := net.SplitHostPort(target.Addr)
	if err != nil {
		return nil, fmt.Errorf("%w: device %s has invalid address", ErrTargetNotAllowed, deviceID)
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	if ip == nil {
		return nil, fmt.Errorf("%w: device %s address is not an IP", ErrTargetNotAllowed, deviceID)
	}
	for _, network := range s.allowed {
		if network.Contains(ip) {
			return target, nil
		}
	}
	return nil, fmt.Errorf("%w: device %s address %s", ErrTargetNotAllowed, deviceID, ip)
}

var _ TargetSource = (*CIDRTargetSource)(nil)
