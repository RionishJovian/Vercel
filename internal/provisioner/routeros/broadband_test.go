package routeros

import (
	"context"
	"mega-magic/internal/broadband"
	"regexp"
	"slices"
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

// Properties of /ppp secret, /ppp active and /ppp profile (RouterOS CLI reference).
// Inside `where`, a variable with one of these names reads the record's own property,
// so `where name=$name` matches every record and would disconnect every subscriber.
var bbReservedNames = []string{"name", "service", "caller-id", "address", "uptime", "encoding", "session-id", "password", "profile", "comment", "disabled", "routes", "local-address", "remote-address", "only-one", "rate-limit", "on-up", "on-down", "id", "bytes", "packets", "type"}

func bbRouterSources() map[string]string {
	sources := map[string]string{"guard": bbGuardSource, "hook": bbOnUp("home001", "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")}
	for _, action := range []string{"renew", "suspend", "resume"} {
		sources[action] = bbMutationCommand(broadband.Spec{AccountID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", OperationID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", ExpectedOperation: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", Action: action, Username: "home001", PeriodSeconds: 86400}, "mm-bb-x")
	}
	return sources
}

func TestBroadbandScriptsDoNotReadRecordPropertiesByAccident(t *testing.T) {
	local := regexp.MustCompile(`:local ([A-Za-z0-9-]+)`)
	where := regexp.MustCompile(`where [^\]]*`)
	variable := regexp.MustCompile(`=\$"?([A-Za-z0-9-]+)`)
	for source, text := range bbRouterSources() {
		for _, m := range local.FindAllStringSubmatch(text, -1) {
			if slices.Contains(bbReservedNames, m[1]) {
				t.Errorf("%s declares :local %s, a RouterOS property name", source, m[1])
			}
		}
		for _, clause := range where.FindAllString(text, -1) {
			for _, m := range variable.FindAllStringSubmatch(clause, -1) {
				if slices.Contains(bbReservedNames, m[1]) {
					t.Errorf("%s filters with $%s, which reads each record's own property: %s", source, m[1], clause)
				}
			}
		}
	}
}

func TestBroadbandSSHCommandsStayOnOneLine(t *testing.T) {
	if strings.Contains(bbClockSource, "\n") {
		t.Error("the clock check is sent over SSH, so it must stay on one line")
	}
	for source, text := range bbRouterSources() {
		if source != "guard" && source != "hook" && strings.ContainsAny(text, "\r\n") {
			t.Errorf("%s is sent over SSH with a line break: it fails before RouterOS 7.25", source)
		}
	}
	// Scripts stored on the router may span lines; they travel quoted.
	if quoted := routerOSQuoted(bbGuardSource); strings.ContainsAny(quoted, "\r\n") {
		t.Error("stored guard source reached the SSH command unescaped")
	}
}

func TestBroadbandScriptsDoNotDependOnArrayElementTypes(t *testing.T) {
	for source, text := range bbRouterSources() {
		for _, risky := range []string{`($a->1) !=`, `($a->2) !=`, `($a->6) !=`, `($anchor->0) =`, ":return"} {
			if strings.Contains(text, risky) {
				t.Errorf("%s relies on undocumented behaviour: %s", source, risky)
			}
		}
	}
}

func TestBroadbandGuardWritesFlashOnlyOnChange(t *testing.T) {
	for _, want := range []string{`:if ($off != "true" and $off != "yes") do={ /ppp secret set $u disabled=yes; };`, `:if ($off = "true" or $off = "yes") do={ /ppp secret set [find where name=$bbUser`} {
		if !strings.Contains(bbEnforceOneSource, want) {
			t.Error("the 15-second guard rewrites the secret without checking it first:", want)
		}
	}
}
