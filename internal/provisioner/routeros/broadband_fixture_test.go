package routeros

import (
	"context"
	"fmt"
	"mega-magic/internal/broadband"
	"strings"
	"testing"
	"time"
)

type bbReadFixture struct {
	t                                              *testing.T
	id, op                                         string
	state                                          broadband.State
	commands                                       []string
	active, disabled, failRemove, missingScheduler bool
}

func (r *bbReadFixture) Run(_ context.Context, cmd string) (string, error) {
	r.commands = append(r.commands, cmd)
	switch {
	case strings.HasPrefix(cmd, "/system script print"):
		return "name: " + bbGuardName + "\ncomment: " + bbMarker + "\npolicy: read,write\ndont-require-permissions: yes\nsource: " + bbGuardSource + "\n", nil
	case strings.HasPrefix(cmd, "/system scheduler print"):
		if r.missingScheduler {
			return "", nil
		}
		return "name: " + bbGuardName + "\ncomment: " + bbMarker + "\ninterval: 15s\ndisabled: no\npolicy: read,write\non-event: /system script run " + bbGuardName + "\n", nil
	case strings.HasPrefix(cmd, "/ppp profile print"):
		comment := bbMarker
		if r.state.FirstUse != 0 {
			comment += fmt.Sprintf(",%d", r.state.FirstUse)
		}
		return "name: " + bbProfile(r.id) + "\ncomment: " + comment + "\nonly-one: yes\non-up: " + bbOnUp("home001", r.id) + "\non-down: \n", nil
	case strings.Contains(cmd, "get") && strings.Contains(cmd, "comment"):
		return r.state.Encode(r.id, r.op), nil
	case strings.Contains(cmd, ":timestamp"):
		return fmt.Sprint(time.Now().Unix()), nil
	case strings.Contains(cmd, "active print count-only"):
		if r.active {
			return "1", nil
		}
		return "0", nil
	case strings.Contains(cmd, "BBSESSION"):
		return "none", nil
	case strings.HasPrefix(cmd, "/ppp secret set"):
		r.disabled = strings.HasSuffix(cmd, "disabled=yes")
		return "", nil
	case strings.HasPrefix(cmd, "/ppp active remove"):
		if r.failRemove {
			r.failRemove = false
			return "", fmt.Errorf("lost response")
		}
		r.active = false
		return "", nil
	case strings.Contains(cmd, "get") && strings.Contains(cmd, "disabled"):
		if r.disabled {
			return "yes", nil
		}
		return "no", nil
	}
	r.t.Fatalf("unexpected router operation: %s", cmd)
	return "", fmt.Errorf("unknown command")
}

func TestBroadbandRetryFinishesDisconnectionAndRequiresEnforcement(t *testing.T) {
	id, op := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	now := time.Now().Unix()
	s := broadband.State{PeriodSeconds: 86400, FirstUse: now - 100, ExpiresAt: now + 86300, LastClock: now - 100, Suspended: true}
	r := &bbReadFixture{t: t, id: id, op: op, state: s, active: true, disabled: true, failRemove: true}
	if _, e := readBroadband(context.Background(), r, id, "home001"); e == nil {
		t.Fatal("background read accepted an unfinished disconnection")
	}
	spec := broadband.Spec{AccountID: id, OperationID: op, ExpectedOperation: op, Action: "suspend", Username: "home001", Profile: "default"}
	if _, e := applyBroadband(context.Background(), r, spec); e == nil {
		t.Fatal("lost disconnect was called successful")
	}
	if _, e := applyBroadband(context.Background(), r, spec); e != nil || r.active {
		t.Fatal("retry did not finish disconnect", e)
	}
	r.missingScheduler = true
	if _, e := readBroadband(context.Background(), r, id, "home001"); e == nil {
		t.Fatal("missing expiry scheduler was reported as verified")
	}
}
