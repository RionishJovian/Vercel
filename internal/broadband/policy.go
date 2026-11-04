// Package broadband contains the shared paid PPPoE lifecycle. Router observations,
// provisioning retries and payments have separate identities; a login is not a sale.
package broadband

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var UUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var ErrState = errors.New("invalid broadband state or clock; reload and review")
var ErrRejected = errors.New("subscriber account was not installed; choose another username or plan")

const MaxPeriod = 366 * 86400
const MaxPrepaid = 10 * 366 * 86400

type State struct {
	PeriodSeconds int64 `json:"period_seconds"`
	FirstUse      int64 `json:"first_use"`
	ExpiresAt     int64 `json:"expires_at"`
	Suspended     bool  `json:"suspended"`
	LastClock     int64 `json:"last_clock"`
}

func ValidPeriod(n int64) bool { return n >= 86400 && n <= MaxPeriod && n%86400 == 0 }
func (s State) Valid() bool {
	return s.PeriodSeconds >= 86400 && s.PeriodSeconds <= MaxPrepaid && s.PeriodSeconds%86400 == 0 &&
		((s.FirstUse == 0 && s.ExpiresAt == 0) || (validClock(s.FirstUse) && s.ExpiresAt > s.FirstUse && s.ExpiresAt <= 4102444800)) &&
		(s.LastClock == 0 || validClock(s.LastClock)) && (s.FirstUse == 0 || s.LastClock >= s.FirstUse)
}
func validClock(n int64) bool { return n >= 1000000000 && n <= 4102444800 }
func (s *State) clock(now int64) error {
	if !s.Valid() || !validClock(now) || now < s.LastClock {
		return ErrState
	}
	return nil
}
func (s *State) Activate(now int64) error {
	if err := s.clock(now); err != nil {
		return err
	}
	if s.FirstUse == 0 {
		s.FirstUse = now
		s.ExpiresAt = now + s.PeriodSeconds
	}
	s.LastClock = now
	return nil
}
func (s *State) Renew(now, seconds int64) error {
	if err := s.clock(now); err != nil {
		return err
	}
	if !ValidPeriod(seconds) {
		return ErrState
	}
	if s.FirstUse == 0 {
		if s.PeriodSeconds+seconds > MaxPrepaid {
			return ErrState
		}
		s.PeriodSeconds += seconds
	} else {
		start := s.ExpiresAt
		if start < now {
			start = now
		}
		if start+seconds > now+MaxPrepaid || start+seconds > 4102444800 {
			return ErrState
		}
		s.ExpiresAt = start + seconds
	}
	s.LastClock = now
	return nil
}
func (s State) Status(now int64) string {
	if s.clock(now) != nil {
		return "clock_unverified"
	}
	if s.Suspended {
		return "suspended"
	}
	if s.FirstUse == 0 {
		return "unused"
	}
	if now >= s.ExpiresAt {
		return "expired"
	}
	return "active"
}
func (s State) Encode(account, operation string) string {
	suspended := 0
	if s.Suspended {
		suspended = 1
	}
	return fmt.Sprintf("mm-bb-v1,%s,%s,%d,%d,%d,%d,%d", account, operation, s.PeriodSeconds, s.FirstUse, s.ExpiresAt, suspended, s.LastClock)
}
func Decode(raw, account string) (State, string, error) {
	p := strings.Split(raw, ",")
	if len(p) != 8 || p[0] != "mm-bb-v1" || !UUID.MatchString(account) || p[1] != account || !UUID.MatchString(p[2]) {
		return State{}, "", ErrState
	}
	var n [5]int64
	for i := range n {
		v, err := strconv.ParseInt(p[i+3], 10, 64)
		if err != nil || strconv.FormatInt(v, 10) != p[i+3] {
			return State{}, "", ErrState
		}
		n[i] = v
	}
	s := State{n[0], n[1], n[2], n[3] == 1, n[4]}
	if (n[3] != 0 && n[3] != 1) || !s.Valid() {
		return State{}, "", ErrState
	}
	return s, p[2], nil
}

// ResolveAnchor merges the hook's immutable profile anchor with the independently
// updated operation state. A first login and an unused renewal commute: neither
// can overwrite the other's record.
func (s State) ResolveAnchor(first int64) (State, error) {
	if !s.Valid() {
		return State{}, ErrState
	}
	if first == 0 {
		if s.FirstUse != 0 {
			return State{}, ErrState
		}
		return s, nil
	}
	if !validClock(first) || (s.FirstUse != 0 && s.FirstUse != first) {
		return State{}, ErrState
	}
	if s.FirstUse == 0 {
		s.FirstUse = first
		s.ExpiresAt = first + s.PeriodSeconds
		if s.LastClock < first {
			s.LastClock = first
		}
	}
	if !s.Valid() {
		return State{}, ErrState
	}
	return s, nil
}
