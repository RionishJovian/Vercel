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
