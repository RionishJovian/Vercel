package server

import "time"

// syncBackoff keeps one unreachable router from logging a warning and dialing out every
// scan. Each consecutive failure of a business doubles its wait (1, 2, 4... up to 15
// minutes); a success clears it. Used from the single worker goroutine, so no lock.
type syncBackoff struct {
	state map[string]*syncFailure
}

type syncFailure struct {
	count int
	next  time.Time
}

const (
	syncBackoffBase = time.Minute
	syncBackoffMax  = 15 * time.Minute
)

func newSyncBackoff() *syncBackoff { return &syncBackoff{state: map[string]*syncFailure{}} }

// allow reports whether the business may be tried now.
func (b *syncBackoff) allow(biz string, now time.Time) bool {
	f := b.state[biz]
	return f == nil || !now.Before(f.next)
}

// fail records a failure and says whether to log it: the first one, then every tenth.
func (b *syncBackoff) fail(biz string, now time.Time) (count int, logIt bool) {
	f := b.state[biz]
	if f == nil {
		f = &syncFailure{}
		b.state[biz] = f
	}
	f.count++
	wait := syncBackoffBase << min(f.count-1, 4)
	if wait > syncBackoffMax {
		wait = syncBackoffMax
	}
	f.next = now.Add(wait)
	return f.count, f.count == 1 || f.count%10 == 0
}

// ok clears the business and returns how many failures it had.
func (b *syncBackoff) ok(biz string) int {
	f := b.state[biz]
	delete(b.state, biz)
	if f == nil {
		return 0
	}
	return f.count
}
