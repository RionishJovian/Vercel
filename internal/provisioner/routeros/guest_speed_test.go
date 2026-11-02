package routeros

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
)

func TestGuestSpeedRateIsUploadFirstAndNamesAreExact(t *testing.T) {
	// RouterOS reads rx/tx from the router's side: rx is what the client uploads.
	if got := guestSpeedRate(20, 5); got != "5M/20M" {
		t.Fatalf("a 20 Mbps down, 5 Mbps up plan must be 5M/20M, got %s", got)
	}
	if guestSpeedProfileName(20, 5) != "mm-speed-u5-d20" || guestSpeedProfileName(5, 20) == guestSpeedProfileName(20, 5) {
		t.Fatal("upload and download must not share a profile name")
	}
}

// speedRouter simulates the Hotspot profile and user menus, keeping state so order and read-back are real.
type speedRouter struct {
	profiles map[string]map[string]string
	user     string
	log      []string
	badProf  bool
	sched    bool
}

var kv = regexp.MustCompile(`([a-z-]+)=("[^"]*"|\S+)`)

func (r *speedRouter) Run(_ context.Context, cmd string) (string, error) {
	r.log = append(r.log, cmd)
	switch {
	case cmd == "/system clock print":
		return "date: 2026-12-31\ntime: 23:59:30", nil
	case strings.HasPrefix(cmd, "/ip hotspot user profile print count-only where name="):
		name := strings.Trim(strings.TrimPrefix(cmd, "/ip hotspot user profile print count-only where name="), `"`)
		if _, ok := r.profiles[name]; ok {
			return "1", nil
		}
		return "0", nil
	case strings.HasPrefix(cmd, "/ip hotspot user profile add "):
		f := map[string]string{"on-login": "", "on-logout": ""}
		for _, m := range kv.FindAllStringSubmatch(cmd, -1) {
			f[m[1]] = strings.Trim(m[2], `"`)
		}
		r.profiles[f["name"]] = f
		return "", nil
	case strings.HasPrefix(cmd, "/ip hotspot user profile print value-list detail where name="):
		name := strings.Trim(strings.TrimPrefix(cmd, "/ip hotspot user profile print value-list detail where name="), `"`)
		var b strings.Builder
		for k, v := range r.profiles[name] {
			fmt.Fprintf(&b, "%s: %s\n", k, v)
		}
		return b.String(), nil
	case strings.Contains(cmd, " user print "):
		return r.user, nil
	case strings.Contains(cmd, " user set "):
		p := "default"
		if m := regexp.MustCompile(`profile="([^"]+)"`).FindStringSubmatch(cmd); m != nil {
			p = m[1]
		}
		if r.badProf {
			p = "default"
		}
		r.user = fmt.Sprintf("name: AA:BB:CC:DD:EE:FE\ncomment: mega-magic:paid:%s\nprofile: %s\nlimit-uptime: 1h30s\nlimit-bytes-total: 1024003000", graceTestID, p)
		return "", nil
	case strings.Contains(cmd, " scheduler print count-only "):
		if r.sched {
			return "1", nil
		}
		return "0", nil
	case strings.Contains(cmd, " scheduler add "):
		r.sched = true
		return "", nil
	case strings.Contains(cmd, " scheduler remove "), strings.Contains(cmd, " active remove "):
		return "", nil
	}
	return "", fmt.Errorf("unexpected command %s", cmd)
}
func newSpeedRouter() *speedRouter {
	return &speedRouter{profiles: map[string]map[string]string{}, user: fmt.Sprintf("name: AA:BB:CC:DD:EE:FE\ncomment: mega-magic:grace:%s\nprofile: default\nuptime: 30s\nbytes-in: 1000\nbytes-out: 2000", graceTestID)}
}
func (r *speedRouter) index(sub string) int {
	for i, c := range r.log {
		if strings.Contains(c, sub) {
			return i
		}
	}
	return -1
}

func approve(r *speedRouter, down, up int) error {
	return quickConfigApplier(r).DecideGuestAccess(context.Background(), "biz-1", "dev-1", graceTestID, "AA:BB:CC:DD:EE:FE", true, 60, 1024, down, up)
}

func TestApprovedPlanWithSpeedCreatesTheProfileBeforeSwitchingTheUser(t *testing.T) {
	r := newSpeedRouter()
	if err := approve(r, 20, 5); err != nil {
		t.Fatal(err)
	}
	add, set := r.index("/ip hotspot user profile add "), r.index(" user set ")
	if add < 0 || set < 0 || add > set {
		t.Fatalf("the profile must exist before the user is switched to it: add=%d set=%d", add, set)
	}
	p := r.profiles["mm-speed-u5-d20"]
	if p == nil || p["rate-limit"] != "5M/20M" || p["shared-users"] != "1" || p["comment"] != guestSpeedMarker {
		t.Fatalf("profile: %v", p)
	}
	if !strings.Contains(r.log[set], `profile="mm-speed-u5-d20"`) || !strings.Contains(r.user, "profile: mm-speed-u5-d20") {
		t.Fatalf("the guest was not switched to the speed profile: %s", r.log[set])
	}
}

func TestApprovedPlanWithoutSpeedTouchesNoProfile(t *testing.T) {
	r := newSpeedRouter()
	if err := approve(r, 0, 0); err != nil {
		t.Fatal(err)
	}
	for _, c := range r.log {
		if strings.Contains(c, "user profile") || strings.Contains(c, "profile=") {
			t.Fatalf("a plan without a speed changed profiles: %s", c)
		}
	}
	if !strings.Contains(r.user, "profile: default") {
		t.Fatal("the guest left the default profile")
	}
}

func TestExistingProfileIsAdoptedOnlyIfMegaMagicMadeItWithTheSameLimits(t *testing.T) {
	ours := map[string]string{"name": "mm-speed-u5-d20", "rate-limit": "5000k/20000k", "shared-users": "1", "comment": guestSpeedMarker, "on-login": "", "on-logout": ""}
	r := newSpeedRouter()
	r.profiles["mm-speed-u5-d20"] = ours
	if err := approve(r, 20, 5); err != nil {
		t.Fatalf("an equal profile in different units must be adopted: %v", err)
	}
	if r.index("/ip hotspot user profile add ") >= 0 {
		t.Fatal("an existing profile was created again")
	}
	for name, mutate := range map[string]func(map[string]string){
		"made by someone else": func(m map[string]string) { m["comment"] = "" },
		"different limit":      func(m map[string]string) { m["rate-limit"] = "5M/50M" },
		"swapped direction":    func(m map[string]string) { m["rate-limit"] = "20M/5M" },
		"has a login script":   func(m map[string]string) { m["on-login"] = ":log info x" },
		"shares users":         func(m map[string]string) { m["shared-users"] = "5" },
	} {
		f := map[string]string{}
		for k, v := range ours {
			f[k] = v
		}
		mutate(f)
		r := newSpeedRouter()
		r.profiles["mm-speed-u5-d20"] = f
		err := approve(r, 20, 5)
		if err == nil || !strings.Contains(err.Error(), "nothing was changed") {
			t.Errorf("%s: a foreign profile was adopted (err=%v)", name, err)
		}
		if r.index(" user set ") >= 0 {
			t.Errorf("%s: the guest was switched although the profile was refused", name)
		}
		if got := r.profiles["mm-speed-u5-d20"]; got["rate-limit"] != f["rate-limit"] || got["comment"] != f["comment"] {
			t.Errorf("%s: the foreign profile was modified", name)
		}
	}
}

func TestSpeedIsVerifiedByReadBackAndInvalidSpeedsNeverReachTheRouter(t *testing.T) {
	r := newSpeedRouter()
	r.badProf = true
	if err := approve(r, 20, 5); err == nil {
		t.Fatal("a guest that did not end up on the speed profile was reported as approved")
	}
	for _, pair := range [][2]int{{20, 0}, {0, 5}, {-1, 5}, {10001, 5}, {20, 10001}} {
		r := newSpeedRouter()
		if err := approve(r, pair[0], pair[1]); err == nil {
			t.Errorf("%v accepted", pair)
		}
		if len(r.log) != 0 {
			t.Errorf("%v reached the router: %v", pair, r.log)
		}
	}
}
