package hub

import (
	"errors"
	"regexp"
	"strconv"
)

// ErrWireGuardUnsupported is returned for routers that cannot run a WireGuard
// tunnel. WireGuard arrived in RouterOS 7.0; RouterOS 6 has no /interface/wireguard.
var ErrWireGuardUnsupported = errors.New("WireGuard needs RouterOS 7 or newer")

// A RouterOS version looks like "7.16.2 (stable)", "7.17rc1 (testing)" or
// "6.49.13 (long-term)". Only the leading major.minor is trusted; the tail is
// length- and character-bounded so it can never carry script syntax.
var routerOSVersion = regexp.MustCompile(`^([0-9]{1,2})\.([0-9]{1,3})[A-Za-z0-9 ._()+-]{0,40}$`)

// ParseRouterOSVersion returns the major and minor numbers of a RouterOS version string.
func ParseRouterOSVersion(raw string) (major, minor int, ok bool) {
	m := routerOSVersion.FindStringSubmatch(raw)
	if m == nil {
		return 0, 0, false
	}
	major, _ = strconv.Atoi(m[1])
	minor, _ = strconv.Atoi(m[2])
	return major, minor, true
}

// RequireWireGuard fails closed: an unreadable version is refused, not assumed
// to be new enough. The Hotspot, PPPoE and walled-garden commands are written in
// the space-separated form both RouterOS 6 and 7 accept, so only the tunnel needs
// this gate.
func RequireWireGuard(version string) error {
	major, _, ok := ParseRouterOSVersion(version)
	if !ok || major < 7 {
		return ErrWireGuardUnsupported
	}
	return nil
}
