package routeros

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func pair(device, kind string, taken time.Time) []string {
	base := fmt.Sprintf("mm-snap-%s-%s-%s", device, kind, taken.Format("20060102-150405"))
	return []string{base + ".rsc", base + ".backup"}
}

func TestOldSnapshotsArePrunedButRecentAndForeignFilesStay(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	r := networkFixture()
	// 25 old pairs (30..54 days) and 3 recent pairs; plus files that are not ours.
	for d := 54; d >= 30; d-- {
		r.files = append(r.files, pair("dev", "net", now.AddDate(0, 0, -d))...)
	}
	for _, h := range []int{1, 24, 48} {
		r.files = append(r.files, pair("dev", "net", now.Add(-time.Duration(h)*time.Hour))...)
	}
	foreign := []string{"mm-snap-notes.txt", "my-backup-20261101-120000.rsc", "mm-snap-dev-net-20260101-000000.rsc.bak", "flash/mm-snap-x-net-20200101-000000.rsc"}
	r.files = append(r.files, foreign...)

	removed := pruneSnapshots(context.Background(), r, now)
	if removed != snapshotMaxDeletes {
		t.Fatalf("expected the per-run cap of %d, got %d", snapshotMaxDeletes, removed)
	}
	have := strings.Join(r.files, "\n")
	for _, f := range foreign {
		if !strings.Contains(have, f) {
			t.Fatalf("%s is not ours and must stay", f)
		}
	}
	for _, h := range []int{1, 24, 48} {
		for _, f := range pair("dev", "net", now.Add(-time.Duration(h)*time.Hour)) {
			if !strings.Contains(have, f) {
				t.Fatalf("recent %s must stay", f)
			}
		}
	}
	// The oldest go first: the very oldest pair is gone, a pair near the keep line is not touched yet.
	for _, f := range pair("dev", "net", now.AddDate(0, 0, -54)) {
		if strings.Contains(have, f) {
			t.Fatalf("oldest %s should have been removed", f)
		}
	}
	// A second run finishes the job; a third finds nothing.
	pruneSnapshots(context.Background(), r, now)
	if n := pruneSnapshots(context.Background(), r, now); n != 0 {
		t.Fatalf("nothing should be left to remove, removed %d", n)
	}
	kept := 0
	for _, f := range r.files {
		if snapshotFile.MatchString(f) {
			kept++
		}
	}
	if kept != 2*snapshotKeepNewest {
		t.Fatalf("expected the newest %d pairs kept, found %d files", snapshotKeepNewest, kept)
	}
}

func TestSnapshotsUnderTheKeepCountOrYoungerThanAWeekAreNeverRemoved(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	r := networkFixture()
	for i := 0; i < 5; i++ { // few, but old
		r.files = append(r.files, pair("dev", "net", now.AddDate(0, 0, -100-i))...)
	}
	if n := pruneSnapshots(context.Background(), r, now); n != 0 {
		t.Fatalf("5 pairs is under the keep count, removed %d", n)
	}
	r = networkFixture()
	for i := 0; i < 30; i++ { // many, but all within a week
		r.files = append(r.files, pair("dev", "net", now.Add(-time.Duration(i)*time.Hour))...)
	}
	if n := pruneSnapshots(context.Background(), r, now); n != 0 {
		t.Fatalf("everything is younger than a week, removed %d", n)
	}
}

func TestPruningRunsAfterAVerifiedChangeAndNeverBlocksIt(t *testing.T) {
	r := addressFixture()
	now := time.Now().UTC()
	for d := 40; d >= 20; d-- {
		r.files = append(r.files, pair("dev", "net", now.AddDate(0, 0, -d))...)
	}
	applyOK(t, r, NetworkRequest{Kind: "vlan", ID: "*51", Values: map[string]string{"vlan-id": "21"}})
	if len(r.files) >= 42 {
		t.Fatalf("old snapshots should be pruned after a verified change, %d files remain", len(r.files))
	}
	// A failed listing must not turn a success into a failure and removes nothing.
	failing := &failingListRunner{networkRunner: addressFixture()}
	if n := pruneSnapshots(context.Background(), failing, now); n != 0 {
		t.Fatal("a failed listing must remove nothing")
	}
}

type failingListRunner struct{ *networkRunner }

func (f *failingListRunner) Run(ctx context.Context, cmd string) (string, error) {
	if strings.HasPrefix(cmd, ":foreach f in=[/file find") {
		return "", fmt.Errorf("router busy")
	}
	return f.networkRunner.Run(ctx, cmd)
}
