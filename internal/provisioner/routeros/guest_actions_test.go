package routeros

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// scriptedRouter answers in order per matching rule, so a read after a write can differ.
type scriptedRouter struct {
	calls []string
	rules []*scriptRule
}
type scriptRule struct {
	contains string
	replies  []string
	fail     error
}

func (r *scriptedRouter) on(contains string, replies ...string) *scriptedRouter {
	r.rules = append(r.rules, &scriptRule{contains: contains, replies: replies})
	return r
}
func (r *scriptedRouter) failOn(contains string) *scriptedRouter {
	r.rules = append(r.rules, &scriptRule{contains: contains, fail: errors.New("router refused")})
	return r
}
func (r *scriptedRouter) Run(_ context.Context, cmd string) (string, error) {
	r.calls = append(r.calls, cmd)
	for _, rule := range r.rules {
		if !strings.Contains(cmd, rule.contains) {
			continue
		}
		if rule.fail != nil {
			return "", rule.fail
		}
		if len(rule.replies) == 0 {
			return "", nil
		}
		out := rule.replies[0]
		if len(rule.replies) > 1 {
			rule.replies = rule.replies[1:]
		}
		return out, nil
	}
	return "", nil
}
func (r *scriptedRouter) ran(contains string) bool {
	for _, c := range r.calls {
		if strings.Contains(c, contains) {
			return true
		}
	}
	return false
}

const (
	guestTestMAC = "AA:BB:CC:DD:EE:01"
	sessionRead  = "user: ABCDEFGH\naddress: 10.5.50.20\nmac-address: " + guestTestMAC + "\nuptime: 00:10:00\n"
)

var guestTestSession = GuestSession{ID: "*1A", MAC: guestTestMAC, Address: "10.5.50.20", User: "ABCDEFGH"}

func TestDisconnectGuestChecksIdentityThenConfirmsItEnded(t *testing.T) {
	r := (&scriptedRouter{}).on(":do { :put", sessionRead, guestGoneMarker)
	if err := disconnectGuest(context.Background(), r, guestTestSession); err != nil {
		t.Fatal(err)
	}
	if !r.ran("/ip hotspot active remove *1A") || !r.ran("/ip hotspot cookie remove [find where mac-address="+guestTestMAC+"]") {
		t.Fatalf("expected removal of the session and its cookie: %q", r.calls)
	}
}

// RouterOS 7.24.6 really answers this way for a removed session: empty values, no error.
const removedSessionRead = "user: \naddress: \nmac-address: \n"

func TestDisconnectGuestUnderstandsRouterOSEmptyReadOfARemovedSession(t *testing.T) {
	r := (&scriptedRouter{}).on(":do { :put", sessionRead, removedSessionRead)
	if err := disconnectGuest(context.Background(), r, guestTestSession); err != nil {
		t.Fatal(err)
	}
	r = (&scriptedRouter{}).on(":do { :put", removedSessionRead)
	if err := disconnectGuest(context.Background(), r, guestTestSession); !errors.Is(err, ErrGuestSessionChanged) {
		t.Fatalf("second kick: %v", err)
	}
	if r.ran("remove") {
		t.Fatalf("removed something: %q", r.calls)
	}
}

func TestDisconnectGuestAcceptsAGuestWhoLoggedStraightBackIn(t *testing.T) {
	fresh := "user: ABCDEFGH\naddress: 10.5.50.20\nmac-address: " + guestTestMAC + "\nuptime: 00:00:01\n"
	r := (&scriptedRouter{}).on(":do { :put", sessionRead, fresh)
	if err := disconnectGuest(context.Background(), r, guestTestSession); err != nil {
		t.Fatal(err)
	}
}

func TestDisconnectGuestDoesNotMistakeGarbageForARemovedSession(t *testing.T) {
	r := (&scriptedRouter{}).on(":do { :put", "syntax error (line 1)")
	if err := disconnectGuest(context.Background(), r, guestTestSession); err == nil || errors.Is(err, ErrGuestSessionChanged) {
		t.Fatalf("err = %v", err)
	}
}

func TestDisconnectGuestLeavesAChangedSessionAlone(t *testing.T) {
	for name, read := range map[string]string{
		"gone":          guestGoneMarker,
		"other user":    "user: OTHER\naddress: 10.5.50.20\nmac-address: " + guestTestMAC + "\n",
		"other device":  "user: ABCDEFGH\naddress: 10.5.50.20\nmac-address: AA:BB:CC:DD:EE:02\n",
		"other address": "user: ABCDEFGH\naddress: 10.5.50.99\nmac-address: " + guestTestMAC + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			r := (&scriptedRouter{}).on(":do { :put", read)
			if err := disconnectGuest(context.Background(), r, guestTestSession); !errors.Is(err, ErrGuestSessionChanged) {
				t.Fatalf("err = %v", err)
			}
			if r.ran("remove") {
				t.Fatalf("removed something: %q", r.calls)
			}
		})
	}
}

func TestDisconnectGuestReportsFailureWhenTheRouterStillListsIt(t *testing.T) {
	r := (&scriptedRouter{}).on(":do { :put", sessionRead, sessionRead)
	if err := disconnectGuest(context.Background(), r, guestTestSession); err == nil {
		t.Fatal("claimed success")
	}
}

func TestGuestInputsAreRefusedBeforeAnyCommandCouldCarryThem(t *testing.T) {
	for _, s := range []GuestSession{
		{ID: "*1; /system reset", MAC: guestTestMAC, Address: "10.0.0.1", User: "A"},
		{ID: "*1", MAC: "AA:BB:CC:DD:EE:01]; /x", Address: "10.0.0.1", User: "A"},
		{ID: "*1", MAC: guestTestMAC, Address: "not-ip", User: "A"},
		{ID: "*1", MAC: guestTestMAC, Address: "10.0.0.1", User: ""},
	} {
		if s.valid() {
			t.Errorf("accepted %+v", s)
		}
	}
	for _, mac := range []string{"FF:FF:FF:FF:FF:FF", "01:00:5E:00:00:01", "00:00:00:00:00:00", "AA-BB-CC-DD-EE-01", "*"} {
		if validGuestMAC(mac) {
			t.Errorf("accepted MAC %q", mac)
		}
	}
}

func binding(id, mac, kind, comment string) string {
	return fmt.Sprintf("id: %s\nmac-address: %s\naddress: \nserver: all\ntype: %s\ncomment: %s\ndisabled: false\n", id, mac, kind, comment)
}

func TestBlockGuestMACAddsConfirmsThenEndsSessions(t *testing.T) {
	r := (&scriptedRouter{}).on("/ip hotspot ip-binding find", "", binding("*5", guestTestMAC, "blocked", guestBanComment))
	if err := blockGuestMAC(context.Background(), r, guestTestMAC); err != nil {
		t.Fatal(err)
	}
	if !r.ran("/ip hotspot ip-binding add mac-address="+guestTestMAC+" type=blocked comment=\""+guestBanComment+"\"") || r.ran("place-before") {
		t.Fatalf("add command: %q", r.calls)
	}
	if !r.ran("/ip hotspot active remove [find where mac-address="+guestTestMAC+"]") || !r.ran("/ip hotspot cookie remove") {
		t.Fatalf("session not ended: %q", r.calls)
	}
}

func TestBlockGuestMACGoesFirstSoNoEarlierBindingWins(t *testing.T) {
	other := binding("*2", "AA:BB:CC:DD:EE:99", "bypassed", "printer")
	r := (&scriptedRouter{}).on("/ip hotspot ip-binding find", other, other+binding("*5", guestTestMAC, "blocked", guestBanComment))
	if err := blockGuestMAC(context.Background(), r, guestTestMAC); err != nil {
		t.Fatal(err)
	}
	if !r.ran("place-before=0") {
		t.Fatalf("not placed first: %q", r.calls)
	}
}

func TestBlockGuestMACNeverTakesOverAnOwnersBinding(t *testing.T) {
	r := (&scriptedRouter{}).on("/ip hotspot ip-binding find", binding("*2", guestTestMAC, "bypassed", "owner phone"))
	if err := blockGuestMAC(context.Background(), r, guestTestMAC); !errors.Is(err, ErrGuestBindingForeign) {
		t.Fatalf("err = %v", err)
	}
	if r.ran("ip-binding add") || r.ran("active remove") {
		t.Fatalf("changed the router: %q", r.calls)
	}
}

func TestBlockGuestMACDoesNotEndSessionsUnlessTheBlockIsConfirmed(t *testing.T) {
	r := (&scriptedRouter{}).on("/ip hotspot ip-binding find", "", "")
	if err := blockGuestMAC(context.Background(), r, guestTestMAC); err == nil {
		t.Fatal("claimed success")
	}
	if r.ran("active remove") {
		t.Fatalf("ended a session with no block: %q", r.calls)
	}
}

func TestUnblockGuestMACRemovesOnlyTheAppsOwnBlock(t *testing.T) {
	r := (&scriptedRouter{}).on("/ip hotspot ip-binding find", binding("*5", guestTestMAC, "blocked", guestBanComment), "")
	if err := unblockGuestMAC(context.Background(), r, guestTestMAC); err != nil {
		t.Fatal(err)
	}
	if !r.ran("/ip hotspot ip-binding remove *5") {
		t.Fatalf("calls: %q", r.calls)
	}
	r = (&scriptedRouter{}).on("/ip hotspot ip-binding find", binding("*5", guestTestMAC, "blocked", "set by hand"))
	if err := unblockGuestMAC(context.Background(), r, guestTestMAC); !errors.Is(err, ErrGuestBindingForeign) {
		t.Fatalf("err = %v", err)
	}
	if r.ran("ip-binding remove") {
		t.Fatalf("removed an owner's binding: %q", r.calls)
	}
}

const retireID = "11111111-1111-4111-8111-111111111111"

var retireLogin = VoucherLogin{VoucherID: retireID, Code: "ABCDEFGH"}

func voucherRead(uptime, in, mac, comment string) string {
	return fmt.Sprintf("%s%s__start\ncomment=%s\ndisabled=false\nuptime=%s\nbytes-in=%s\nbytes-out=0\nmac=%s\n%s%s__end\n", voucherSyncMarker, retireID, comment, uptime, in, mac, voucherSyncMarker, retireID)
}

var voucherAbsent = voucherSyncMarker + retireID + "__start\nabsent\n" + voucherSyncMarker + retireID + "__end\n"

func TestRemoveVoucherLoginRemovesAnUnusedLoginAndConfirms(t *testing.T) {
	r := (&scriptedRouter{}).on(voucherSyncMarker, voucherRead("0s", "0", "none", voucherLoginPrefix+retireID), voucherAbsent)
	if err := removeVoucherLogin(context.Background(), r, retireLogin); err != nil {
		t.Fatal(err)
	}
	if !r.ran(`/ip hotspot user remove [find where name="ABCDEFGH"]`) {
		t.Fatalf("calls: %q", r.calls)
	}
}

func TestRemoveVoucherLoginTreatsAnAbsentLoginAsDone(t *testing.T) {
	r := (&scriptedRouter{}).on(voucherSyncMarker, voucherAbsent)
	if err := removeVoucherLogin(context.Background(), r, retireLogin); err != nil {
		t.Fatal(err)
	}
	if r.ran("user remove") {
		t.Fatalf("calls: %q", r.calls)
	}
}

func TestRemoveVoucherLoginRefusesAUsedCode(t *testing.T) {
	for name, read := range map[string]string{
		"uptime": voucherRead("5m", "0", "none", voucherLoginPrefix+retireID),
		"bytes":  voucherRead("0s", "10", "none", voucherLoginPrefix+retireID),
		"online": voucherRead("0s", "0", guestTestMAC, voucherLoginPrefix+retireID),
	} {
		t.Run(name, func(t *testing.T) {
			r := (&scriptedRouter{}).on(voucherSyncMarker, read)
			if err := removeVoucherLogin(context.Background(), r, retireLogin); !errors.Is(err, ErrVoucherLoginInUse) {
				t.Fatalf("err = %v", err)
			}
			if r.ran("user remove") {
				t.Fatalf("removed a used code: %q", r.calls)
			}
		})
	}
}

func TestRemoveVoucherLoginReportsFailureWhenTheLoginRemains(t *testing.T) {
	read := voucherRead("0s", "0", "none", voucherLoginPrefix+retireID)
	r := (&scriptedRouter{}).on(voucherSyncMarker, read, read)
	if err := removeVoucherLogin(context.Background(), r, retireLogin); err == nil {
		t.Fatal("claimed success")
	}
}

func TestObservationExtrasFailWithoutLosingTheSessionList(t *testing.T) {
	r := (&scriptedRouter{}).failOn("session-time-left").failOn("/ip hotspot ip-binding").
		on("/ip hotspot active find", "id: *1\nuser: A\naddress: 10.0.0.2\nmac-address: "+guestTestMAC+"\nuptime: 1m\nbytes-in: 1\nbytes-out: 1\nserver: hs\n")
	inv, err := readObservations(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if !inv.Available["hotspot_sessions"] || len(inv.Objects["hotspot_sessions"]) != 1 {
		t.Fatalf("lost the session list: %+v", inv.Available)
	}
	if inv.Available["hotspot_session_times"] || inv.Available["hotspot_bindings"] {
		t.Fatalf("failed reads reported as available: %+v", inv.Available)
	}
}
