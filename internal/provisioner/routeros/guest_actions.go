package routeros

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
)

// Explicit owner actions on guests. Each one checks what is on the router first,
// changes one typed thing and reads the result back. The handler keeps them
// behind a switch until they have passed a router trial.
const (
	guestBanComment = "mega-magic:ban"
	guestGoneMarker = "__MM_GONE__"
)

var (
	// ErrGuestSessionChanged means the session is gone or now belongs to someone else.
	ErrGuestSessionChanged = errors.New("guest session changed on the router")
	// ErrGuestBindingForeign means the MAC has an IP binding this app did not create.
	ErrGuestBindingForeign = errors.New("this MAC already has an IP binding the app did not create")
	// ErrVoucherLoginInUse means the code has been used, so it is not removed.
	ErrVoucherLoginInUse = errors.New("voucher login has been used")
)

// GuestSession identifies one row of /ip hotspot active as the owner saw it.
type GuestSession struct{ ID, MAC, Address, User string }

func (s GuestSession) valid() bool {
	ip := net.ParseIP(s.Address)
	return networkIDPattern.MatchString(s.ID) && validGuestMAC(s.MAC) && ip != nil && ip.To4() != nil && len(s.User) >= 1 && len(s.User) <= 64
}

// validGuestMAC accepts one unicast MAC, so a ban can never match a whole group.
func validGuestMAC(mac string) bool {
	m, err := net.ParseMAC(mac)
	return err == nil && voucherMACPattern.MatchString(mac) && m[0]&1 == 0 && m.String() != "00:00:00:00:00:00"
}

// DisconnectGuest ends one session. The guest can log in again with valid access.
func (a *DeviceApplier) DisconnectGuest(ctx context.Context, businessID, deviceID string, s GuestSession) error {
	if !s.valid() {
		return fmt.Errorf("invalid guest session")
	}
	r, err := runnerForDevice(ctx, a.src, a.factory, a.timeout, businessID, deviceID)
	if err != nil {
		return err
	}
	return disconnectGuest(ctx, r, s)
}

func disconnectGuest(ctx context.Context, r CommandRunner, s GuestSession) error {
	// A session's id can come back on the next login, seen on RouterOS 7.24.6 with the
	// same id under another user, so the identity must still match.
	got, gone, err := readGuestSession(ctx, r, s.ID)
	if err != nil {
		return err
	}
	if gone || got["user"] != s.User || got["address"] != s.Address || !strings.EqualFold(got["mac-address"], s.MAC) {
		return ErrGuestSessionChanged
	}
	if _, err := r.Run(ctx, "/ip hotspot active remove "+s.ID); err != nil {
		return fmt.Errorf("cannot end the session: %w", err)
	}
	if err := removeGuestCookies(ctx, r, s.MAC); err != nil {
		return err
	}
	after, gone, err := readGuestSession(ctx, r, s.ID)
	if err != nil {
		return fmt.Errorf("cannot confirm the session ended: %w", err)
	}
	if gone || after["user"] != s.User || newerSession(got["uptime"], after["uptime"]) {
		return nil // Ended. A guest with valid access may already have logged in again.
	}
	return fmt.Errorf("the router still lists the session")
}

// newerSession reports a session younger than the one that was removed.
func newerSession(before, after string) bool {
	b, errB := parseVoucherSyncUptime(before)
	a, errA := parseVoucherSyncUptime(after)
	return errB == nil && errA == nil && a < b
}

func readGuestSession(ctx context.Context, r CommandRunner, id string) (map[string]string, bool, error) {
	out, err := r.Run(ctx, `:do { :put ("user: " . [:tostr [/ip hotspot active get `+id+` user]]); :put ("address: " . [:tostr [/ip hotspot active get `+id+` address]]); :put ("mac-address: " . [:tostr [/ip hotspot active get `+id+` mac-address]]); :put ("uptime: " . [:tostr [/ip hotspot active get `+id+` uptime]]) } on-error={ :put "`+guestGoneMarker+`" }`)
	if err != nil {
		return nil, false, fmt.Errorf("cannot read the session: %w", err)
	}
	if strings.Contains(out, guestGoneMarker) {
		return nil, true, nil
	}
	if !strings.Contains(out, "user: ") {
		return nil, false, fmt.Errorf("unrecognized session read")
	}
	fields := map[string]string{}
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		if k, v, ok := strings.Cut(line+" ", ": "); ok {
			fields[k] = strings.TrimSpace(v)
		}
	}
	// Seen on RouterOS 7.24.6: inside :tostr, get on a missing id gives an empty
	// value and no error, so on-error alone never reports a removed session.
	if fields["user"] == "" {
		return nil, true, nil
	}
	return fields, false, nil
}

// A MAC cookie left behind would log the browser straight back in.
func removeGuestCookies(ctx context.Context, r CommandRunner, mac string) error {
	if _, err := r.Run(ctx, "/ip hotspot cookie remove [find where mac-address="+strings.ToUpper(mac)+"]"); err != nil {
		return fmt.Errorf("cannot clear the guest's login cookie: %w", err)
	}
	return nil
}

// BlockGuestMAC adds a blocked IP binding for one MAC and ends its sessions.
func (a *DeviceApplier) BlockGuestMAC(ctx context.Context, businessID, deviceID, mac string) error {
	if !validGuestMAC(mac) {
		return fmt.Errorf("invalid MAC address")
	}
	r, err := runnerForDevice(ctx, a.src, a.factory, a.timeout, businessID, deviceID)
	if err != nil {
		return err
	}
	return blockGuestMAC(ctx, r, strings.ToUpper(mac))
}

func blockGuestMAC(ctx context.Context, r CommandRunner, mac string) error {
	bindings, err := readNetworkObjects(ctx, r, "bindings")
	if err != nil {
		return fmt.Errorf("cannot read IP bindings: %w", err)
	}
	mine := false
	for _, b := range bindings {
		if strings.EqualFold(b.Values["mac-address"], mac) {
			if b.Values["comment"] != guestBanComment {
				return ErrGuestBindingForeign
			}
			mine = true
		}
	}
	if !mine {
		cmd := "/ip hotspot ip-binding add mac-address=" + mac + " type=blocked comment=" + routerOSQuoted(guestBanComment)
		if len(bindings) > 0 {
			cmd += " place-before=0" // The first matching binding wins.
		}
		if _, err := r.Run(ctx, cmd); err != nil {
			return fmt.Errorf("cannot add the block: %w", err)
		}
	}
	found, err := guestBanEntries(ctx, r, mac)
	if err != nil {
		return fmt.Errorf("cannot confirm the block: %w", err)
	}
	if len(found) != 1 || found[0].Values["type"] != "blocked" || found[0].Values["disabled"] == "true" {
		return fmt.Errorf("the router does not show the block as active")
	}
	if _, err := r.Run(ctx, "/ip hotspot active remove [find where mac-address="+mac+"]"); err != nil {
		return fmt.Errorf("blocked, but cannot end the guest's session: %w", err)
	}
	return removeGuestCookies(ctx, r, mac)
}

// UnblockGuestMAC removes only a block this app added.
func (a *DeviceApplier) UnblockGuestMAC(ctx context.Context, businessID, deviceID, mac string) error {
	if !validGuestMAC(mac) {
		return fmt.Errorf("invalid MAC address")
	}
	r, err := runnerForDevice(ctx, a.src, a.factory, a.timeout, businessID, deviceID)
	if err != nil {
		return err
	}
	return unblockGuestMAC(ctx, r, strings.ToUpper(mac))
}

func unblockGuestMAC(ctx context.Context, r CommandRunner, mac string) error {
	bindings, err := readNetworkObjects(ctx, r, "bindings")
	if err != nil {
		return fmt.Errorf("cannot read IP bindings: %w", err)
	}
	removed := false
	for _, b := range bindings {
		if !strings.EqualFold(b.Values["mac-address"], mac) {
			continue
		}
		if b.Values["comment"] != guestBanComment {
			return ErrGuestBindingForeign
		}
		if !networkIDPattern.MatchString(b.ID) {
			return fmt.Errorf("invalid router record identity")
		}
		if _, err := r.Run(ctx, "/ip hotspot ip-binding remove "+b.ID); err != nil {
			return fmt.Errorf("cannot remove the block: %w", err)
		}
		removed = true
	}
	if !removed {
		return nil
	}
	left, err := guestBanEntries(ctx, r, mac)
	if err != nil {
		return fmt.Errorf("cannot confirm the block was removed: %w", err)
	}
	if len(left) != 0 {
		return fmt.Errorf("the router still shows the block")
	}
	return nil
}

func guestBanEntries(ctx context.Context, r CommandRunner, mac string) ([]NetworkObject, error) {
	bindings, err := readNetworkObjects(ctx, r, "bindings")
	if err != nil {
		return nil, err
	}
	found := []NetworkObject{}
	for _, b := range bindings {
		if strings.EqualFold(b.Values["mac-address"], mac) {
			found = append(found, b)
		}
	}
	return found, nil
}

// RemoveVoucherLogin deletes an unused voucher's login, so a printed code stops
// working. A login that is already gone counts as done, which makes a retry safe.
func (a *DeviceApplier) RemoveVoucherLogin(ctx context.Context, businessID, deviceID string, login VoucherLogin) error {
	if err := validateVoucherSyncRefs([]VoucherLogin{login}); err != nil {
		return err
	}
	r, err := runnerForDevice(ctx, a.src, a.factory, a.timeout, businessID, deviceID)
	if err != nil {
		return err
	}
	return removeVoucherLogin(ctx, r, login)
}

func removeVoucherLogin(ctx context.Context, r CommandRunner, login VoucherLogin) error {
	// This read also refuses a user whose comment shows it belongs to someone else.
	states, err := readVoucherLogins(ctx, r, []VoucherLogin{login})
	if err != nil {
		return err
	}
	if len(states) != 1 {
		return fmt.Errorf("unrecognized voucher login read")
	}
	s := states[0]
	if !s.Exists {
		return nil
	}
	if s.UptimeSeconds > 0 || s.BytesUsed > 0 || s.FirstUseEpoch > 0 || s.ActiveMAC != "" {
		return ErrVoucherLoginInUse
	}
	if _, err := r.Run(ctx, "/ip hotspot user remove [find where name="+routerOSQuoted(login.Code)+"]"); err != nil {
		return fmt.Errorf("cannot remove the voucher login: %w", err)
	}
	states, err = readVoucherLogins(ctx, r, []VoucherLogin{login})
	if err != nil {
		return fmt.Errorf("cannot confirm the voucher login was removed: %w", err)
	}
	if len(states) != 1 || states[0].Exists {
		return fmt.Errorf("the router still lists the voucher login")
	}
	return nil
}
