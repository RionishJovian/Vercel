package routeros

import (
	"strings"
	"testing"
)

func TestBroadbandFirstLoginCannotOverwriteRenewalMarker(t *testing.T) {
	hook := bbOnUp("home001", "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	if strings.Contains(hook, "/ppp secret set $u comment=") {
		t.Fatal("first login can overwrite a concurrent renewal or suspension")
	}
}

func TestBroadbandGuardRecoversClockPauseWithoutOpeningDeniedAccounts(t *testing.T) {
	if !strings.Contains(bbEnforceOneSource, `} else={ /ppp secret set [find where name=$name comment=$raw profile=$p service=pppoe] disabled=no; }`) {
		t.Fatal("a valid account stays disabled after startup clock synchronization")
	}
	if strings.Contains(bbEnforceOneSource, `set $u disabled=no`) {
		t.Fatal("a stale guard snapshot can reopen a subsequently suspended account")
	}
	for _, condition := range []string{`$clockOK = false`, `($a->6) != "0"`, `$now >= $expiry`, `$now < $seen`} {
		if !strings.Contains(bbEnforceOneSource, condition) {
			t.Fatal("automatic recovery lost a denial condition", condition)
		}
	}
	if !strings.Contains(bbGuardSource, `on-error={ /ppp secret set $u disabled=yes;`) {
		t.Fatal("one damaged account can stop enforcement of the remaining accounts")
	}
}
