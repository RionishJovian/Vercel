package recovery

import (
	"errors"
	"strings"
	"testing"

	"mega-magic/internal/provisioner"
)

func TestActionOutcomeTextIsFixed(t *testing.T) {
	verified := &ActionOutcome{Approval: provisioner.ApprovalOutcome{Allowed: true}, Result: &provisioner.ApplyResult{Verified: true}}
	sent := &ActionOutcome{Approval: provisioner.ApprovalOutcome{Allowed: true}, Result: &provisioner.ApplyResult{}}
	refused := &ActionOutcome{}
	cases := []struct {
		out  *ActionOutcome
		err  error
		want string
	}{
		{verified, nil, "router.reboot.applied"},
		{sent, nil, "router.reboot.unverified"},
		{refused, nil, "router.reboot.denied"},
		{nil, errors.New("dial 10.77.0.4:22 secret detail"), "router.reboot.failed"},
	}
	for _, c := range cases {
		action, reason := actionOutcome("reboot", c.out, c.err)
		if action != c.want || reason == "" || strings.Contains(reason, "10.77") {
			t.Errorf("%s: got %s %q", c.want, action, reason)
		}
	}
}
