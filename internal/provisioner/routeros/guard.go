// Package routeros implements the REAL SSH + RouterOS transport that fills the
// injectable seams (Applier, Prober, CommandRunner) of the provisioner apply
// path. It is intentionally isolated from the pure planning core in
// internal/provisioner.
//
// Safety: nothing here is constructed unless the live applier is explicitly
// enabled, and GuardTarget fails closed if the SSH target looks like the
// production hub/CCR2004. The transport must only ever be pointed at a
// disposable lab/sandbox router.
package routeros

import (
	"fmt"
	"net"
	"net/netip"
	"strings"
)

// Production denylist values. These appear ONLY as guard inputs/checks — never
// as a target, endpoint, key, or default. This is the one place the live
// addresses are named, so the guard can refuse them.
const (
	ProdHubHost = "hub.mikromagic.com"
	ProdHubPort = 51820
	prodCCRCIDR = "10.77.0.0/24" // live CCR2004 tunnel range 10.77.0.4/24
)

var prodCCR = func() *net.IPNet {
	_, n, err := net.ParseCIDR(prodCCRCIDR)
	if err != nil {
		panic("routeros: bad prod CIDR constant: " + err.Error())
	}
	return n
}()

var disposableNetworkParents = []netip.Prefix{
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("::1/128"),
}

var prodCCRPrefix = netip.MustParsePrefix(prodCCRCIDR)

// ValidateDisposableCIDR accepts only canonical networks wholly contained in
// private, loopback, or IPv6 ULA space and refuses any overlap with the known
// production router range. Certification uses this before wrapping every DB
// lookup in CIDRTargetSource, so a concurrent row change cannot escape from a
// disposable network.
func ValidateDisposableCIDR(raw string) error {
	raw = strings.TrimSpace(raw)
	prefix, err := netip.ParsePrefix(raw)
	if err != nil {
		return fmt.Errorf("routeros: invalid disposable CIDR %q: %w", raw, err)
	}
	if prefix != prefix.Masked() {
		return fmt.Errorf("routeros: disposable CIDR %q is not a canonical network", raw)
	}
	if prefix.Overlaps(prodCCRPrefix) {
		return fmt.Errorf("routeros: disposable CIDR %q overlaps production range %s", raw, prodCCRCIDR)
	}
	for _, parent := range disposableNetworkParents {
		if prefix.Addr().BitLen() == parent.Addr().BitLen() &&
			prefix.Bits() >= parent.Bits() && parent.Contains(prefix.Addr()) {
			return nil
		}
	}
	return fmt.Errorf("routeros: disposable CIDR %q is not wholly private, loopback, or IPv6 ULA", raw)
}

// GuardTarget fails closed when an SSH target looks like production.
//
// Denylisted:
//   - empty host
//   - the live hub WireGuard port (51820)
//   - the live hub domain (hub.mikromagic.com and any *.mikromagic.com)
//   - the live CCR2004 range (10.77.0.0/24)
//
// It returns nil only for a target that is not obviously production; it is a
// safety backstop, not an allowlist.
func GuardTarget(host string, port int) error {
	h := strings.ToLower(strings.TrimSpace(host))
	if h == "" {
		return fmt.Errorf("routeros: refusing to build a live transport with an empty host")
	}
	if port == ProdHubPort {
		return fmt.Errorf("routeros: port %d is the production WireGuard hub port; refusing", port)
	}
	if h == ProdHubHost || h == "mikromagic.com" ||
		strings.HasSuffix(h, "."+ProdHubHost) || strings.HasSuffix(h, ".mikromagic.com") {
		return fmt.Errorf("routeros: %q is a production host; refusing", host)
	}
	if ip := net.ParseIP(h); ip != nil {
		if ip4 := ip.To4(); ip4 != nil && prodCCR.Contains(ip4) {
			return fmt.Errorf("routeros: %s is inside the production CCR2004 range %s; refusing", h, prodCCRCIDR)
		}
	}
	return nil
}
