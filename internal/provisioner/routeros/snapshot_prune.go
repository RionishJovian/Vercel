package routeros

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Every guarded change saves a full export and backup on the router named
// mm-snap-<device>-<kind>-<timestamp>.{rsc,backup}. They are the owner's way back,
// so recent ones are kept, but they otherwise fill the router's flash forever.
const (
	snapshotKeepNewest = 10
	snapshotMinAge     = 7 * 24 * time.Hour
	snapshotMaxDeletes = 20
)

var snapshotFile = regexp.MustCompile(`^(mm-snap-[A-Za-z0-9_-]{1,70}-(\d{8}-\d{6}))\.(rsc|backup)$`)

// runPrune runs cleanup off the request path so a slow router never delays or
// fails the change that just succeeded. Tests replace it with a synchronous call.
var runPrune = func(f func()) { go f() }

// pruneSnapshotsLater removes the app's own old snapshot files after a verified
// change. It is best effort and silent: a failure only leaves files behind.
func pruneSnapshotsLater(r CommandRunner) {
	runPrune(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		pruneSnapshots(ctx, r, time.Now().UTC())
	})
}

// pruneSnapshots keeps the newest snapshotKeepNewest snapshots, and anything younger
// than snapshotMinAge, and removes the rest, oldest first. It only ever touches
// files whose whole name matches what snapshotName produces, and removes each by its
// exact name. It reports how many files it removed.
func pruneSnapshots(ctx context.Context, r CommandRunner, now time.Time) int {
	out, err := r.Run(ctx, `:foreach f in=[/file find where name~"^mm-snap-"] do={ :put [/file get $f name] }`)
	if err != nil {
		return 0
	}
	type snap struct {
		base  string
		taken time.Time
		files []string
	}
	bases := map[string]*snap{}
	for _, line := range strings.Split(out, "\n") {
		name := strings.TrimSpace(line)
		m := snapshotFile.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		taken, e := time.Parse("20060102-150405", m[2])
		if e != nil {
			continue
		}
		s := bases[m[1]]
		if s == nil {
			s = &snap{base: m[1], taken: taken}
			bases[m[1]] = s
		}
		s.files = append(s.files, name)
	}
	ordered := make([]*snap, 0, len(bases))
	for _, s := range bases {
		ordered = append(ordered, s)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if !ordered[i].taken.Equal(ordered[j].taken) {
			return ordered[i].taken.After(ordered[j].taken)
		}
		return ordered[i].base > ordered[j].base
	})
	removed := 0
	for i := len(ordered) - 1; i >= snapshotKeepNewest && removed < snapshotMaxDeletes; i-- {
		if now.Sub(ordered[i].taken) < snapshotMinAge {
			continue
		}
		for _, name := range ordered[i].files {
			if removed >= snapshotMaxDeletes {
				break
			}
			if _, e := r.Run(ctx, `/file remove [find where name=`+routerOSQuoted(name)+`]`); e != nil {
				return removed
			}
			removed++
		}
	}
	return removed
}
