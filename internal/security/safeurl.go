package security

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// ErrBlockedAddress is returned when a tenant-supplied URL would reach a
// loopback, private, link-local or otherwise internal address.
var ErrBlockedAddress = errors.New("destination address is not allowed")

// blockedPrefixes are ranges a tenant-supplied URL must never reach. The
// helper methods on netip.Addr cover most of them; the list also catches the
// documentation, carrier-grade NAT and benchmarking ranges they do not.
var blockedPrefixes = func() []netip.Prefix {
	raw := []string{
		"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16",
		"172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24", "192.168.0.0/16", "198.18.0.0/15",
		"198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4",
		"::/128", "::1/128", "fc00::/7", "fe80::/10", "ff00::/8", "2001:db8::/32", "64:ff9b::/96",
	}
	out := make([]netip.Prefix, 0, len(raw))
	for _, r := range raw {
		out = append(out, netip.MustParsePrefix(r))
	}
	return out
}()

// IsBlockedIP reports whether ip is an address a webhook must never reach.
// An invalid address is treated as blocked (fail closed).
func IsBlockedIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsMulticast() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	for _, p := range blockedPrefixes {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

// ValidateWebhookURL checks a tenant-supplied webhook URL at save time. It is
// an early, friendly check only: the dial-time guard in SafeHTTPClient is the
// real protection, because DNS can change after a URL is saved.
func ValidateWebhookURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return errors.New("url is not valid")
	}
	if u.Scheme != "https" {
		return errors.New("url must start with https://")
	}
	if u.User != nil {
		return errors.New("url must not contain a username or password")
	}
	host := u.Hostname()
	if host == "" {
		return errors.New("url must include a host name")
	}
	if ip, perr := netip.ParseAddr(host); perr == nil {
		if IsBlockedIP(ip) {
			return fmt.Errorf("url host: %w", ErrBlockedAddress)
		}
		return nil
	}
	lower := strings.ToLower(strings.TrimSuffix(host, "."))
	// Single-label names (db, redis, api) are internal service names; so are
	// localhost and the usual private suffixes.
	if !strings.Contains(lower, ".") || lower == "localhost" || strings.HasSuffix(lower, ".localhost") ||
		strings.HasSuffix(lower, ".local") || strings.HasSuffix(lower, ".internal") ||
		strings.HasSuffix(lower, ".localdomain") || strings.HasSuffix(lower, ".lan") {
		return fmt.Errorf("url host: %w", ErrBlockedAddress)
	}
	return nil
}

// SafeHTTPClient returns an HTTP client for calls to tenant-supplied URLs. It
// refuses to connect to internal addresses (checked on the resolved IP at dial
// time, which also defeats DNS rebinding), never follows redirects, and ignores
// proxy environment variables.
func SafeHTTPClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{
		Timeout: 5 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return ErrBlockedAddress
			}
			ip, err := netip.ParseAddr(host)
			if err != nil || IsBlockedIP(ip) {
				return ErrBlockedAddress
			}
			return nil
		},
	}
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: timeout,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
