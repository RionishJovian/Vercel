package routeros

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type provisionContainRunner struct {
	commands             []string
	user, active, cookie string
	writeFails           bool
}

func (r *provisionContainRunner) Run(_ context.Context, cmd string) (string, error) {
	r.commands = append(r.commands, cmd)
	switch {
	case strings.HasPrefix(cmd, ":if"):
		if r.writeFails {
			return "", errors.New("timeout")
		}
		return "", nil
	case strings.HasPrefix(cmd, "/ip hotspot user print"):
		return r.user, nil
	case strings.HasPrefix(cmd, "/ip hotspot active print"):
		return r.active, nil
	case strings.HasPrefix(cmd, "/ip hotspot cookie print"):
		return r.cookie, nil
	default:
		return "", errors.New("unexpected command")
	}
}

func TestGuestProvisionContainmentVerifiesSessionsAndCookies(t *testing.T) {
	for _, tc := range []struct {
		name, user, active, cookie string
		writeFails, wantError      bool
	}{
		{"contained", "name: AA:BB:CC:DD:EE:FE\ndisabled: yes", "0", "0", false, false},
		{"missing owned user", "", "0", "0", false, false},
		{"still enabled", "name: AA:BB:CC:DD:EE:FE\ndisabled: no", "0", "0", false, true},
		{"active session survives", "name: AA:BB:CC:DD:EE:FE\ndisabled: yes", "1", "0", false, true},
		{"cookie survives", "name: AA:BB:CC:DD:EE:FE\ndisabled: yes", "0", "1", false, true},
		{"uncertain write", "", "0", "0", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &provisionContainRunner{user: tc.user, active: tc.active, cookie: tc.cookie, writeFails: tc.writeFails}
			err := containGuestProvision(context.Background(), r, graceTestID, "AA:BB:CC:DD:EE:FE")
			if (err != nil) != tc.wantError {
				t.Fatalf("%v", err)
			}
			cmd := r.commands[0]
			if !strings.Contains(cmd, "comment=\"mega-magic:voucher:") || !strings.Contains(cmd, "/ip hotspot cookie remove") || !strings.Contains(cmd, "/ip hotspot active remove") {
				t.Fatal("containment lacks ownership or revocation guards")
			}
		})
	}
}
