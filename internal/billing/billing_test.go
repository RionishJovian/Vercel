package billing

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestParseDecision(t *testing.T) {
	cases := []struct {
		in       string
		wantOK   bool
		wantAppr bool
	}{
		{"approve", true, true},
		{"YES", true, true},
		{"Approved", true, true},
		{"reject", true, false},
		{"no", true, false},
		{"deny", true, false},
		{"", false, false},               // blank => deny
		{"   ", false, false},            // blank => deny
		{"maybe", false, false},          // unknown => deny
		{"approve reject", false, false}, // ambiguous => deny
		{"please approve", false, false}, // malformed => deny
	}
	for _, tc := range cases {
		dec, ok := ParseDecision(tc.in)
		if ok != tc.wantOK {
			t.Fatalf("%q: ok = %v, want %v", tc.in, ok, tc.wantOK)
		}
		if ok && dec.Approve != tc.wantAppr {
			t.Fatalf("%q: approve = %v, want %v", tc.in, dec.Approve, tc.wantAppr)
		}
	}
}

func TestResolvePlan(t *testing.T) {
	if _, err := ResolvePlan("sovereign"); err != nil {
		t.Fatalf("sovereign must resolve: %v", err)
	}
	for _, bad := range []string{"", "plus", "basic", "unknown"} {
		if _, err := ResolvePlan(bad); !errors.Is(err, ErrUnknownPlan) {
			t.Fatalf("%q must fail closed with ErrUnknownPlan, got %v", bad, err)
		}
	}
}

func TestPlanDurationMinutes(t *testing.T) {
	d, err := PlanDurationMinutes("sovereign")
	if err != nil || d != 30*24*60 {
		t.Fatalf("sovereign duration = %d/%v, want 43200", d, err)
	}
	if _, err := PlanDurationMinutes("nope"); err == nil {
		t.Fatal("unknown plan must error")
	}
}

func TestNextStatus_terminalOnceDecided(t *testing.T) {
	// pending -> approved (changed), then terminal.
	next, changed, err := NextStatus(StatusPending, Decision{Approve: true})
	if err != nil || !changed || next != StatusApproved {
		t.Fatalf("pending+approve = %q/%v/%v", next, changed, err)
	}
	if _, changed, err := NextStatus(StatusApproved, Decision{Approve: true}); changed || !errors.Is(err, ErrAlreadyDecided) {
		t.Fatalf("approved is terminal: changed=%v err=%v", changed, err)
	}
	// pending -> rejected (changed), then terminal.
	next, changed, err = NextStatus(StatusPending, Decision{Approve: false})
	if err != nil || !changed || next != StatusRejected {
		t.Fatalf("pending+reject = %q/%v/%v", next, changed, err)
	}
	if _, changed, err := NextStatus(StatusRejected, Decision{Approve: true}); changed || !errors.Is(err, ErrAlreadyDecided) {
		t.Fatalf("rejected is terminal: changed=%v err=%v", changed, err)
	}
}

func TestSaveReceipt_notWorldReadable(t *testing.T) {
	root := t.TempDir()
	path, err := SaveReceipt(root, "biz-1", "req-1", []byte("photo-bytes"))
	if err != nil {
		t.Fatalf("SaveReceipt: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat receipt: %v", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("receipt mode = %o, must not be world/group readable", info.Mode().Perm())
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("receipt mode = %o, want 600", info.Mode().Perm())
	}

	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if dirInfo.Mode().Perm()&0o077 != 0 {
		t.Fatalf("receipt dir mode = %o, must not be world/group accessible", dirInfo.Mode().Perm())
	}

	// Tenant-scoped path + guards.
	if filepath.Base(filepath.Dir(path)) != "biz-1" {
		t.Fatalf("receipt not tenant-scoped: %s", path)
	}
	if _, err := SaveReceipt(root, "", "req-1", nil); err == nil {
		t.Fatal("empty businessID must error")
	}
	if _, err := SaveReceipt(root, "b", "../evil", nil); err == nil {
		t.Fatal("path-traversal id must error")
	}
	if _, err := SaveReceipt(root, "b", "..", nil); err == nil {
		t.Fatal("dot-dot id must error")
	}
	if _, err := SaveReceipt("", "b", "r", nil); err == nil {
		t.Fatal("empty root must error")
	}
}
