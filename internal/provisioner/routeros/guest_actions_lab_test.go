package routeros

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// Runs the guest actions against a disposable RouterOS CHR. It changes the router, so
// it only runs when MM_CHR_SSH names one. Needs the Hotspot and guest from
// TestCHRLabHotspotAndReads. See docs/CHR-LAB.md.
func TestCHRLabGuestActions(t *testing.T) {
	target := os.Getenv("MM_CHR_SSH")
	if target == "" {
		t.Skip("set MM_CHR_SSH to run against a disposable CHR")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	r := chrRunner{target}
	l := chrLab{t, r, ctx}
	a := NewDeviceApplier(capabilitySource{}, 60*time.Second, func(SSHConfig) CommandRunner { return r }, DefaultInterface, 0, 0)
	const biz, dev = "biz-1", "dev-1"

	guestMAC := strings.ToUpper(l.put(`:put [/interface get [find name=m4] mac-address]`))
	activeCount := func() string {
		return l.put(`:put [:len [/ip hotspot active find where mac-address=` + guestMAC + `]]`)
	}
	traffic := func() { r.Run(ctx, `/ping 1.1.1.1 vrf=guest count=2`) }
	waitActive := func(want bool, within time.Duration) bool {
		end := time.Now().Add(within)
		for time.Now().Before(end) {
			traffic()
			if (activeCount() != "0") == want {
				return true
			}
		}
		return false
	}
	// Logs the lab guest in directly, so this test does not depend on the lab's clock.
	bringOnline := func() {
		t.Helper()
		r.Run(ctx, `:if ([:len [/ip hotspot user find where name="labguest"]] = 0) do={ /ip hotspot user add name="labguest" password="labguest" comment="lab-owner" }`)
		end := time.Now().Add(45 * time.Second)
		for time.Now().Before(end) {
			traffic()
			if activeCount() != "0" {
				return
			}
			host := l.put(`:put [:len [/ip hotspot host find where mac-address=` + guestMAC + `]]`)
			if host != "0" {
				r.Run(ctx, `/ip hotspot active login user="labguest" password="labguest" mac-address=`+guestMAC+` ip=10.60.0.50`)
			}
		}
		if activeCount() == "0" {
			t.Fatal("guest did not get online")
		}
	}
	session := func() GuestSession {
		t.Helper()
		inv, e := readObservations(ctx, r)
		if e != nil || !inv.Available["hotspot_sessions"] {
			t.Fatalf("session read: %v %+v", e, inv.Available)
		}
		for _, o := range inv.Objects["hotspot_sessions"] {
			if strings.EqualFold(o.Values["mac-address"], guestMAC) {
				return GuestSession{ID: o.ID, MAC: guestMAC, Address: o.Values["address"], User: o.Values["user"]}
			}
		}
		t.Fatal("guest session not listed")
		return GuestSession{}
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		for _, cmd := range []string{
			`/ip hotspot ip-binding remove [find where comment=` + routerOSQuoted(guestBanComment) + `]`,
			`/ip hotspot ip-binding remove [find where comment="lab-owner"]`,
			`/ip hotspot active remove [find where mac-address=` + guestMAC + `]`,
			`/ip hotspot user remove [find where name="labguest"]`,
		} {
			r.Run(ctx, cmd)
		}
	})

	bringOnline()

	t.Run("new reads work on a live session", func(t *testing.T) {
		inv, e := readObservations(ctx, r)
		if e != nil {
			t.Fatal(e)
		}
		for _, key := range []string{"hotspot_session_times", "hotspot_bindings"} {
			if !inv.Available[key] {
				t.Errorf("%s not available", key)
			}
		}
		for _, o := range inv.Objects["hotspot_session_times"] {
			t.Logf("session %s: idle-time=%q session-time-left=%q", o.ID, o.Values["idle-time"], o.Values["session-time-left"])
			if _, ok := o.Values["idle-time"]; !ok {
				t.Error("no idle-time")
			}
		}
		if len(inv.Objects["hotspot_session_times"]) == 0 {
			t.Error("no session times for a live session")
		}
	})

	t.Run("kick ends the session and clears its cookie", func(t *testing.T) {
		s := session()
		if e := a.DisconnectGuest(ctx, biz, dev, s); e != nil {
			t.Fatal(e)
		}
		if activeCount() != "0" {
			// A MAC-login guest can return at once; it must be a new session.
			if strings.Contains(l.put(`:put [/ip hotspot active find where mac-address=`+guestMAC+`]`), s.ID) {
				t.Fatal("the same session is still listed")
			}
		}
		if n := l.put(`:put [:len [/ip hotspot cookie find where mac-address=` + guestMAC + `]]`); n != "0" {
			t.Errorf("%s cookies left for the guest", n)
		}
		if e := a.DisconnectGuest(ctx, biz, dev, s); !errors.Is(e, ErrGuestSessionChanged) {
			t.Fatalf("second kick of the same session: %v", e)
		}
	})

	t.Run("ban blocks traffic, and removing it restores access", func(t *testing.T) {
		bringOnline()
		if e := a.BlockGuestMAC(ctx, biz, dev, guestMAC); e != nil {
			t.Fatal(e)
		}
		r.Run(ctx, `/ip hotspot active login user="labguest" password="labguest" mac-address=`+guestMAC+` ip=10.60.0.50`)
		first := l.put(`:put [/ip hotspot ip-binding get [:pick [/ip hotspot ip-binding find] 0] comment]`)
		if first != guestBanComment {
			t.Errorf("first binding comment = %q", first)
		}
		if e := a.BlockGuestMAC(ctx, biz, dev, guestMAC); e != nil {
			t.Fatalf("banning twice must be safe: %v", e)
		}
		if n := l.put(`:put [:len [/ip hotspot ip-binding find where comment=` + routerOSQuoted(guestBanComment) + `]]`); n != "1" {
			t.Errorf("%s bans listed", n)
		}
		if !waitActive(false, 15*time.Second) {
			t.Fatal("banned guest still has a session")
		}
		time.Sleep(5 * time.Second)
		traffic()
		if activeCount() != "0" {
			t.Error("banned guest came back")
		}
		inv, e := readObservations(ctx, r)
		if e != nil || !inv.Available["hotspot_bindings"] {
			t.Fatalf("binding read: %v", e)
		}
		seen := false
		for _, b := range inv.Objects["hotspot_bindings"] {
			if b.Values["comment"] == guestBanComment && b.Values["type"] == "blocked" && strings.EqualFold(b.Values["mac-address"], guestMAC) {
				seen = true
			}
		}
		if !seen {
			t.Errorf("ban not in the read: %+v", inv.Objects["hotspot_bindings"])
		}
		if e := a.UnblockGuestMAC(ctx, biz, dev, guestMAC); e != nil {
			t.Fatal(e)
		}
		if n := l.put(`:put [:len [/ip hotspot ip-binding find where comment=` + routerOSQuoted(guestBanComment) + `]]`); n != "0" {
			t.Errorf("%s bans left after removal", n)
		}
		// RouterOS took about 45 s to list the host again after the block was removed.
		end := time.Now().Add(120 * time.Second)
		var last string
		for time.Now().Before(end) && activeCount() == "0" {
			traffic()
			out, err := r.Run(ctx, `/ip hotspot active login user="labguest" password="labguest" mac-address=`+guestMAC+` ip=10.60.0.50`)
			last = fmt.Sprintf("%q %v", out, err)
			time.Sleep(2 * time.Second)
		}
		if activeCount() == "0" {
			hosts, _ := r.Run(ctx, `/ip hotspot host print; /ip hotspot ip-binding print; /ip hotspot user print where name="labguest"`)
			t.Errorf("guest could not log in after the block was removed; last login %s\n%s", last, hosts)
		}
	})

	t.Run("an owner's own binding is never touched", func(t *testing.T) {
		const mac = "02:AA:BB:CC:DD:EE"
		l.put(`/ip hotspot ip-binding add mac-address=` + mac + ` type=bypassed comment="lab-owner"`)
		if e := a.BlockGuestMAC(ctx, biz, dev, mac); !errors.Is(e, ErrGuestBindingForeign) {
			t.Errorf("ban: %v", e)
		}
		if e := a.UnblockGuestMAC(ctx, biz, dev, mac); !errors.Is(e, ErrGuestBindingForeign) {
			t.Errorf("unban: %v", e)
		}
		if got := l.put(`:put [/ip hotspot ip-binding get [find where comment="lab-owner"] type]`); got != "bypassed" {
			t.Errorf("binding changed: %q", got)
		}
	})

	t.Run("delete removes an unused login and refuses a foreign one", func(t *testing.T) {
		now := time.Now().UnixNano() % 1e12
		code := fmt.Sprintf("LAB%010d", now%1e10)
		login := VoucherLogin{VoucherID: fmt.Sprintf("ffffffff-ffff-4fff-8fff-%012x", now), Code: code, Minutes: 60, DataMB: 500}
		// Added directly: provisioning also needs a synchronized clock, which this test does not.
		l.put(`/ip hotspot user add name=` + routerOSQuoted(code) + ` password=` + routerOSQuoted(code) + ` comment=` + routerOSQuoted(voucherLoginPrefix+login.VoucherID))
		if e := a.RemoveVoucherLogin(ctx, biz, dev, login); e != nil {
			t.Fatal(e)
		}
		if n := l.put(`:put [:len [/ip hotspot user find where name=` + routerOSQuoted(code) + `]]`); n != "0" {
			t.Errorf("login still listed: %s", n)
		}
		if e := a.RemoveVoucherLogin(ctx, biz, dev, login); e != nil {
			t.Errorf("a repeat must be safe: %v", e)
		}
		foreign := "LABOWNER" + fmt.Sprint(now%1000)
		l.put(`/ip hotspot user add name=` + routerOSQuoted(foreign) + ` password="x" comment="lab-owner"`)
		defer r.Run(context.Background(), `/ip hotspot user remove [find where name=`+routerOSQuoted(foreign)+`]`)
		if e := a.RemoveVoucherLogin(ctx, biz, dev, VoucherLogin{VoucherID: login.VoucherID, Code: foreign}); e == nil {
			t.Error("removed a login the app does not own")
		}
		if n := l.put(`:put [:len [/ip hotspot user find where name=` + routerOSQuoted(foreign) + `]]`); n != "1" {
			t.Error("the foreign login is gone")
		}
	})
}
