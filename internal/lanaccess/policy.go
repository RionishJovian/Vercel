// Package lanaccess contains fail-closed policy for future Magic Hub LAN access.
// It does not establish tunnels, listen on ports, or persist access grants.
package lanaccess

import (
	"errors"
	"net/netip"
	"time"
)

var ErrDenied = errors.New("LAN access denied")

const MaxDuration = time.Hour

// Scope must be resolved by the authenticated server, never from request JSON.
// LANs are discovered for this gateway; Protected includes hub/control networks
// and platform services, even when they overlap an otherwise permitted LAN.
type Scope struct {
	BusinessID string
	GatewayID  string
	UserID     string
	Role       string
	Verified   bool
	LANs       []netip.Prefix
	Protected  []netip.Prefix
}

type Request struct {
	TargetIP string
	Port     int
	Duration time.Duration
}

// Grant is server-side state, not a bearer credential. A future repository must
// reload its current revoked/expiry state on every connection and close existing
// streams when revoked or expired. Do not serialize it as an access capability.
type Grant struct {
	BusinessID string
	GatewayID  string
	UserID     string
	Target     netip.Addr
	Port       uint16
	CreatedAt  time.Time
	ExpiresAt  time.Time
	RevokedAt  time.Time
}

func canManage(role string) bool {
	return role == "OWNER" || role == "ADMIN" || role == "DEVELOPER"
}

func validScope(s Scope) bool {
	return s.BusinessID != "" && s.GatewayID != "" && s.UserID != "" && s.Verified && canManage(s.Role) && len(s.Protected) > 0
}

func permitted(s Scope, target netip.Addr) bool {
	// First release permits private IPv4 unicast only. Reject names/URLs, IPv6,
	// link-local metadata addresses, unspecified, loopback and multicast targets.
	if !target.Is4() || target.Zone() != "" || !target.IsPrivate() || !target.IsGlobalUnicast() {
		return false
	}
	for _, network := range s.Protected {
		if !network.IsValid() || network.Contains(target) {
			return false
		}
	}
	allowed := false
	for _, network := range s.LANs {
		// Invalid inventory fails closed rather than silently granting partial access.
		if !network.IsValid() {
			return false
		}
		if !network.Contains(target) {
			continue
		}
		if target.Is4() && network.Bits() < 31 {
			p := network.Masked()
			bytes := target.As4()
			base := p.Addr().As4()
			ip := uint32(bytes[0])<<24 | uint32(bytes[1])<<16 | uint32(bytes[2])<<8 | uint32(bytes[3])
			first := uint32(base[0])<<24 | uint32(base[1])<<16 | uint32(base[2])<<8 | uint32(base[3])
			last := first | ^uint32(0)>>p.Bits()
			if ip == first || ip == last {
				return false
			}
		}
		allowed = true
	}
	return allowed
}

func Create(s Scope, r Request, now time.Time) (Grant, error) {
	target, err := netip.ParseAddr(r.TargetIP)
	if err != nil || !validScope(s) || now.IsZero() || r.Port < 1 || r.Port > 65535 || r.Duration <= 0 || r.Duration > MaxDuration || !permitted(s, target) {
		return Grant{}, ErrDenied
	}
	return Grant{BusinessID: s.BusinessID, GatewayID: s.GatewayID, UserID: s.UserID,
		Target: target, Port: uint16(r.Port), CreatedAt: now, ExpiresAt: now.Add(r.Duration)}, nil
}

// Authorize must run again for every connection with current server-resolved
// scope and grant state. Exact destination matching prevents redirect/proxy
// requests from expanding a grant to another host or port.
func (g Grant) Authorize(s Scope, target netip.Addr, port uint16, now time.Time) error {
	if !validScope(s) || g.BusinessID != s.BusinessID || g.GatewayID != s.GatewayID || g.UserID != s.UserID ||
		!g.RevokedAt.IsZero() || g.CreatedAt.IsZero() || !g.ExpiresAt.After(g.CreatedAt) ||
		g.ExpiresAt.Sub(g.CreatedAt) > MaxDuration || now.Before(g.CreatedAt) || !now.Before(g.ExpiresAt) ||
		g.Target != target || g.Port == 0 || g.Port != port || !permitted(s, target) {
		return ErrDenied
	}
	return nil
}
