package broadband

import (
	"testing"
	"time"
)

func TestLifecycle(t *testing.T) {
	now := time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC).Unix()
	s := State{PeriodSeconds: 30 * 86400}
	if err := s.Activate(now); err != nil {
		t.Fatal(err)
	}
	first, expiry := s.FirstUse, s.ExpiresAt
	if err := s.Activate(now + 100); err != nil || s.FirstUse != first || s.ExpiresAt != expiry {
		t.Fatal("reconnect restarted validity", s, err)
	}
	if err := s.Renew(now+100, 10*86400); err != nil || s.ExpiresAt != expiry+10*86400 {
		t.Fatal("early renewal", s, err)
	}
	late := s.ExpiresAt + 100
	if err := s.Renew(late, 86400); err != nil || s.ExpiresAt != late+86400 || s.FirstUse != first {
		t.Fatal("late renewal", s, err)
	}
	s.Suspended = true
	if err := s.Renew(late+1, 86400); err != nil || !s.Suspended {
		t.Fatal("renewal lifted suspension")
	}
}

func TestUnusedRenewalAndClock(t *testing.T) {
	s := State{PeriodSeconds: 86400}
	now := int64(1793750400)
	if err := s.Renew(now, 86400); err != nil || s.FirstUse != 0 || s.ExpiresAt != 0 || s.PeriodSeconds != 172800 {
		t.Fatal(s, err)
	}
	if err := s.Activate(now); err != nil {
		t.Fatal(err)
	}
	if err := s.Activate(now - 100); err == nil {
		t.Fatal("accepted clock regression")
	}
	if err := s.Renew(now, 0); err == nil {
		t.Fatal("accepted empty period")
	}
	if err := s.Renew(now, 367*86400); err == nil {
		t.Fatal("accepted excessive purchase period")
	}
	if s.Status(now+172800) != "expired" {
		t.Fatal("expiry boundary")
	}
}

func TestRouterStateEncodingRejectsForeignAndMalformedEvidence(t *testing.T) {
	a := "11111111-1111-4111-8111-111111111111"
	op := "22222222-2222-4222-8222-222222222222"
	s := State{PeriodSeconds: 86400}
	encoded := s.Encode(a, op)
	got, last, err := Decode(encoded, a)
	if err != nil || got != s || last != op {
		t.Fatal(got, last, err)
	}
	for _, bad := range []string{encoded + ",extra", "mm-bb-v1," + a + "," + op + ",-1,0,0,0,0", encoded[:len(encoded)-1] + "x"} {
		if _, _, e := Decode(bad, a); e == nil {
			t.Fatal("accepted malformed evidence", bad)
		}
	}
	if _, _, e := Decode(encoded, op); e == nil {
		t.Fatal("accepted another account")
	}
}

func TestFirstUseAndUnusedRenewalCommute(t *testing.T) {
	now := int64(1793750400)
	s := State{PeriodSeconds: 86400}
	if e := s.Renew(now, 86400); e != nil {
		t.Fatal(e)
	}
	after, e := s.ResolveAnchor(now - 1)
	if e != nil || after.FirstUse != now-1 || after.ExpiresAt != now-1+172800 {
		t.Fatal(after, e)
	}
	s2 := State{PeriodSeconds: 86400}
	if e = s2.Activate(now - 1); e != nil {
		t.Fatal(e)
	}
	if e = s2.Renew(now, 86400); e != nil {
		t.Fatal(e)
	}
	if after.FirstUse != s2.FirstUse || after.ExpiresAt != s2.ExpiresAt {
		t.Fatal("concurrent activation lost prepaid time", after, s2)
	}
	if _, e = after.ResolveAnchor(0); e == nil {
		t.Fatal("missing immutable anchor was ignored")
	}
}
