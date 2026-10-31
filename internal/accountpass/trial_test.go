package accountpass

import (
	"testing"
	"time"
)

func TestApplyTrial(t *testing.T) {
	now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	future, past := now.Add(time.Hour), now.Add(-time.Hour)
	needs := Evaluate(now, true, false, nil, nil, nil, nil)
	if got := ApplyTrial(needs, now, &future); !got.Access || got.State != "trial" || got.TrialExpiresAt == nil {
		t.Fatalf("running trial should grant access: %+v", got)
	}
	if got := ApplyTrial(needs, now, &past); got.Access || got.State != "pass_required" {
		t.Fatalf("ended trial must not grant access: %+v", got)
	}
	if got := ApplyTrial(needs, now, &now); got.Access {
		t.Fatal("trial ends exactly at its expiry")
	}
	if got := ApplyTrial(needs, now, nil); got.Access {
		t.Fatal("no trial, no access")
	}
	revoked := Evaluate(now, true, false, &past, &future, &now, nil)
	if got := ApplyTrial(revoked, now, &future); got.Access || got.State != "revoked" {
		t.Fatalf("a revoked pass is not overridden: %+v", got)
	}
	notAuthorized := needs
	notAuthorized.Authorized = false
	if got := ApplyTrial(notAuthorized, now, &future); got.Access {
		t.Fatal("only the authorized login can use a trial")
	}
	legacy := Evaluate(now, false, false, nil, nil, nil, nil)
	legacy.Access = false
	if got := ApplyTrial(legacy, now, &future); got.Access {
		t.Fatal("trial does nothing when passes are not enforced")
	}
}
