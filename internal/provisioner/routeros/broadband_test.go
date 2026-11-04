package routeros

import (
	"context"
	"mega-magic/internal/broadband"
	"strings"
	"testing"
	"time"
)

func TestBroadbandRejectsInvalidRequestBeforeIO(t *testing.T) {
	r := runnerFunc(func(context.Context, string) (string, error) {
		t.Fatal("invalid request reached router")
		return "", nil
	})
	if _, e := applyBroadband(context.Background(), r, broadband.Spec{Action: "renew"}); e == nil {
		t.Fatal("invalid request accepted")
	}
}

func TestBroadbandAcceptsExistingProfileLabels(t *testing.T) {
	s := broadband.Spec{AccountID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", OperationID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", Action: "create", Username: "home001", Password: "test-password-only", PeriodSeconds: 86400}
	for _, profile := range []string{"default", "Home 20M", "Plan (30 days)", "မြန်မာ Plan", `Home "A" $plan`} {
		s.Profile = profile
		if !validBBSpec(s) {
			t.Errorf("existing profile label was rejected: %q", profile)
		}
	}
}

func TestBroadbandProfileLookupKeepsQuotedLabelLiteral(t *testing.T) {
	profile := `Home "A" $plan`
	r := runnerFunc(func(_ context.Context, command string) (string, error) {
		if command != `/ppp profile print value-list detail where name="Home \"A\" \$plan"` {
			t.Fatal("profile label escaped its literal selector", command)
		}
		return "name: " + profile + "\non-up: \non-down: \n", nil
	})
	fields, e := readNamedValueList(context.Background(), r, "/ppp profile", profile)
	if e != nil || fields["name"] != profile {
		t.Fatal(fields, e)
	}
}

func TestBroadbandRejectsUnsafeSourceProfilesBeforeIO(t *testing.T) {
	s := broadband.Spec{AccountID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", OperationID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", Action: "create", Username: "home001", Password: "test-password-only", PeriodSeconds: 86400}
	for _, profile := range []string{"", " Home", "Home ", "Home\nPlan", "Home\tPlan", "Home\x00Plan", strings.Repeat("p", 65)} {
		s.Profile = profile
		if validBBSpec(s) {
			t.Errorf("unsafe profile accepted: %q", profile)
		}
	}
}
func TestBroadbandRetryDoesNotApplyRenewalTwice(t *testing.T) {
	id, op := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	now := time.Now().Unix()
	state := broadband.State{PeriodSeconds: 86400, FirstUse: now - 100, ExpiresAt: now + 86300, LastClock: now - 100}
	r := &bbReadFixture{t: t, id: id, op: op, state: state}
	got, e := applyBroadband(context.Background(), r, broadband.Spec{AccountID: id, OperationID: op, ExpectedOperation: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", Action: "renew", Username: "home001", Profile: "default", PeriodSeconds: 86400})
	if e != nil || got.State.ExpiresAt != state.ExpiresAt {
		t.Fatal(got, e)
	}
	for _, cmd := range r.commands {
		if strings.Contains(cmd, "set $u comment=") {
			t.Fatal("retry rewrote paid period")
		}
	}
}

func TestBroadbandRejectsForeignAccount(t *testing.T) {
	r := runnerFunc(func(_ context.Context, cmd string) (string, error) {
		if strings.Contains(cmd, "get") {
			return "owned-by-somebody-else", nil
		}
		t.Fatal("foreign record caused a write", cmd)
		return "", nil
	})
	_, e := applyBroadband(context.Background(), r, broadband.Spec{AccountID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", OperationID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", ExpectedOperation: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", Action: "renew", Username: "home001", Profile: "default", PeriodSeconds: 86400})
	if e == nil {
		t.Fatal("foreign account adopted")
	}
}

func TestBroadbandSessionCounters(t *testing.T) {
	got, e := parseBroadbandSession("BBSESSION=80000001|1h2m3s|123456|654321\n")
	if e != nil || got == nil || got.UptimeSeconds != 3723 || got.DownloadBytes != 123456 || got.UploadBytes != 654321 {
		t.Fatal(got, e)
	}
	if s, e := parseBroadbandSession("none\n"); e != nil || s != nil {
		t.Fatal(s, e)
	}
	for _, raw := range []string{"BBSESSION=1|1m|-1|3", "BBSESSION=1|1m|5|6\nBBSESSION=2|2m|7|8", "random output", "BBSESSION=1|nonsense|2|3"} {
		if _, e := parseBroadbandSession(raw); e == nil {
			t.Fatal("invalid session accepted", raw)
		}
	}
}

func TestGenericSubscriberEditorCannotChangePaidAccount(t *testing.T) {
	r := networkFixture()
	r.objects["subscribers"]["*3"]["comment"] = "mm-bb-v1,owned-account"
	_, _, e := buildNetworkPlan(context.Background(), r, NetworkRequest{Kind: "subscriber", ID: "*3", Values: map[string]string{"name": "alice", "password": "replacement", "profile": "business", "disabled": "no"}})
	if e == nil {
		t.Fatal("generic editor bypassed paid-account lifecycle")
	}
}

func TestGenericSubscriberCannotBorrowPaidProfile(t *testing.T) {
	r := networkFixture()
	_, _, e := buildNetworkPlan(context.Background(), r, NetworkRequest{Kind: "subscriber", ID: "*3", Values: map[string]string{"name": "alice", "password": "replacement", "profile": "mm-bb-aaaaaaaaaaaa4aaa8aaaaaaaaaaaaaaa", "disabled": "no"}})
	if e == nil {
		t.Fatal("generic subscriber borrowed a paid account's first-use hook")
	}
}
