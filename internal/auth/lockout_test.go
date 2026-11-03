package auth

import (
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
