package auth

import (
	"fmt"
	"testing"
	"time"
)

func TestLoginLimiterLocksAfterRepeatedFailuresAndExpires(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	l := NewLoginLimiter(3, 10*time.Minute, 15*time.Minute)
	l.now = func() time.Time { return now }

	l.Fail("Owner@Example.com")
	l.Fail(" owner@example.com ")
	if locked, _ := l.Locked("owner@example.com"); locked {
		t.Fatal("locked after two failures")
	}
	l.Fail("OWNER@example.com")
	locked, left := l.Locked("owner@example.com")
	if !locked || left != 15*time.Minute {
		t.Fatalf("expected a 15 minute lock, got %v %v", locked, left)
	}
	if locked, _ := l.Locked("other@example.com"); locked {
		t.Fatal("another account was locked")
	}
	now = now.Add(16 * time.Minute)
	if locked, _ := l.Locked("owner@example.com"); locked {
		t.Fatal("lock did not expire")
	}
}

func TestLoginLimiterOldFailuresDoNotAccumulateAndSuccessClears(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	l := NewLoginLimiter(3, 10*time.Minute, 15*time.Minute)
	l.now = func() time.Time { return now }
	l.Fail("a@b.c")
	l.Fail("a@b.c")
	now = now.Add(11 * time.Minute) // both fall out of the window
	l.Fail("a@b.c")
	if locked, _ := l.Locked("a@b.c"); locked {
		t.Fatal("failures outside the window counted")
	}
	l.Fail("a@b.c")
	l.Succeed("a@b.c")
	l.Fail("a@b.c")
	l.Fail("a@b.c")
	if locked, _ := l.Locked("a@b.c"); locked {
		t.Fatal("a successful sign-in did not clear the count")
	}
}

// Per-account protection is keyed on the normalized email ONLY. It must not depend
// on the request source, a rate cookie, or whether the account exists.
func TestLoginLimiterIgnoresSourceAndNormalizesEmail(t *testing.T) {
	l := NewLoginLimiter(8, 10*time.Minute, 15*time.Minute)
	variants := []string{"Owner@Example.com", " owner@example.com", "OWNER@EXAMPLE.COM ", "owner@example.com"}
	for i := 0; i < 8; i++ {
		l.Fail(variants[i%len(variants)]) // different spelling, same account; no source information is even passed
	}
	for _, v := range variants {
		if locked, _ := l.Locked(v); !locked {
			t.Fatalf("%q not locked after 8 failures across spellings", v)
		}
	}
	if locked, _ := l.Locked("someone-else@example.com"); locked {
		t.Fatal("lock leaked to a different account")
	}
}

func TestLoginLimiterLocksUnknownAccountsToo(t *testing.T) {
	l := NewLoginLimiter(8, 10*time.Minute, 15*time.Minute)
	for i := 0; i < 8; i++ {
		l.Fail("does-not-exist@example.com")
	}
	if locked, _ := l.Locked("does-not-exist@example.com"); !locked {
		t.Fatal("unknown accounts must lock like real ones so a lock never reveals whether an account exists")
	}
}

func TestLoginLimiterStateStaysBoundedUnderFakeEmailFlood(t *testing.T) {
	l := NewLoginLimiter(8, 10*time.Minute, 15*time.Minute)
	for i := 0; i < 120000; i++ {
		l.Fail(fmt.Sprintf("flood-%d@example.com", i))
	}
	l.mu.Lock()
	n := len(l.entries)
	l.mu.Unlock()
	if n > loginEntryCapacity {
		t.Fatalf("login limiter grew to %d entries; capacity is %d", n, loginEntryCapacity)
	}
}

func TestLoginLimiterFloodNeverEvictsALockedAccount(t *testing.T) {
	l := NewLoginLimiter(8, 10*time.Minute, 15*time.Minute)
	for i := 0; i < 8; i++ {
		l.Fail("victim@example.com")
	}
	for i := 0; i < 120000; i++ {
		l.Fail(fmt.Sprintf("flood-%d@example.com", i))
	}
	if locked, _ := l.Locked("victim@example.com"); !locked {
		t.Fatal("an attacker flooding fake emails released a locked account")
	}
}
