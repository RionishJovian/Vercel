// Package webfig opens a customer's own RouterOS WebFig through Magic Hub.
//
// The browser talks to a separate hostname served by Proxy, the proxy talks to
// the router over the tunnel, and the router itself checks its own password.
// WebFig encrypts the login in the browser, so the server carries it unread.
package webfig

import (
	"os"
	"strings"

	"mega-magic/internal/model"
)

// Mode is the one switch for this feature, read from MM_WEBFIG_ACCESS.
type Mode string

const (
	// ModeOff hides the feature and refuses every request. It is the default,
	// and anything unrecognised is treated as off.
	ModeOff Mode = "off"
	// ModeOpen lets every authorized ADMIN or above use it, with no charge.
	ModeOpen Mode = "open"
	// ModeLocked shows the feature as locked. See Entitled.
	ModeLocked Mode = "locked"
)

// ModeFromEnv reads the switch. Fail closed: only the two exact words turn it on.
func ModeFromEnv() Mode {
	switch strings.TrimSpace(os.Getenv("MM_WEBFIG_ACCESS")) {
	case string(ModeOpen):
		return ModeOpen
	case string(ModeLocked):
		return ModeLocked
	}
	return ModeOff
}

// Entitled is the single place that decides whether a role may use WebFig under
// a mode. When this becomes a paid service, the check for a paid key or pass
// goes here, in the ModeLocked branch. The platform administrator stays exempt
// so the locked state can be tested.
func Entitled(mode Mode, role string) bool {
	switch mode {
	case ModeOpen:
		return true
	case ModeLocked:
		return model.UserRole(role) == model.RoleDeveloper
	}
	return false
}

// HostFromEnv is the proxy's own domain. Each session opens on a random subdomain
// of it, so it needs a wildcard certificate. Empty means it is not set up.
func HostFromEnv() string {
	return SafeHost(os.Getenv("MM_WEBFIG_HOST"))
}

// appDomain is the console's domain. Routers decide what the proxy serves, and a
// page on a subdomain counts as the same site as its parent: it could set cookies
// on the console. So WebFig must live on a separate domain.
const appDomain = "megamagic.tech"

// SafeHost returns the cleaned domain, or empty when it can't be used safely.
func SafeHost(raw string) string {
	host := strings.ToLower(strings.TrimSpace(raw))
	if !strings.Contains(host, ".") || strings.ContainsAny(host, "/:*@ ") || host == appDomain || strings.HasSuffix(host, "."+appDomain) {
		return ""
	}
	return host
}
