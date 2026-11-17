package server

import (
	"testing"
	"time"
)

func TestSyncBackoffWaitsLongerAfterEachFailureAndResetsOnSuccess(t *testing.T) {
	b, now := newSyncBackoff(), time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	if !b.allow("a", now) {
		t.Fatal("a new business must be tried")
	}
	n, logIt := b.fail("a", now)
	if n != 1 || !logIt {
		t.Fatalf("first failure: n=%d log=%v, want 1 true", n, logIt)
	}
	if b.allow("a", now.Add(30*time.Second)) || !b.allow("a", now.Add(time.Minute)) {
		t.Fatal("first wait must be one minute")
	}
	_, logIt = b.fail("a", now.Add(time.Minute))
	if logIt || b.allow("a", now.Add(2*time.Minute)) || !b.allow("a", now.Add(3*time.Minute)) {
		t.Fatal("second failure: not logged, two minute wait")
	}
	for i := 0; i < 20; i++ {
		b.fail("a", now)
	}
	if b.allow("a", now.Add(14*time.Minute)) || !b.allow("a", now.Add(15*time.Minute)) {
		t.Fatal("wait must stop growing at 15 minutes")
	}
	if got := b.ok("a"); got != 22 {
		t.Fatalf("ok returned %d failures, want 22", got)
	}
	if !b.allow("a", now) || b.ok("a") != 0 {
		t.Fatal("a success must clear the business")
	}
}
