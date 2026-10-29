package routeros

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"
)

type portalSource struct{}

func (portalSource) TargetForDevice(ctx context.Context, businessID, deviceID string) (*DeviceTarget, error) {
	if businessID != "biz" {
		return nil, ErrNoTarget
	}
	target, err := (capabilitySource{}).TargetForDevice(ctx, "biz-1", "dev-1")
	if target != nil {
		target.DeviceID = deviceID
	}
	return target, err
}
func portalTestApplier(r CommandRunner) *DeviceApplier {
	return NewDeviceApplier(portalSource{}, time.Second, func(SSHConfig) CommandRunner { return r }, DefaultInterface, 0, 0)
}

type portalRunner struct {
	directory       string
	files           map[string]string
	garden          bool
	commands        []string
	failSwitch      bool
	badRule         bool
	host            string
	mediaGarden     bool
	failLogin       bool
	failMediaAdd    bool
	failMediaRemove bool
}

var portalQuoted = regexp.MustCompile(`"((?:\\.|[^"\\])*)"`)

func (r *portalRunner) Run(_ context.Context, cmd string) (string, error) {
	r.commands = append(r.commands, cmd)
	q := portalQuoted.FindAllStringSubmatch(cmd, -1)
	switch {
	case cmd == portalProfilesCommand:
		return "name: default\nname: guest\n", nil
	case strings.HasPrefix(cmd, "/ip hotspot profile print count-only"):
		return "1\n", nil
	case cmd == portalProfileCommand(`name="guest"`):
		return "name: guest\nlogin-by: http-chap,mac\nhtml-directory: " + r.directory + "\n", nil
	case strings.HasPrefix(cmd, "/ip hotspot profile set"):
		if r.failSwitch {
			r.failSwitch = false
			r.directory = q[1][1]
			return "", errors.New("lost response after switch")
		}
		r.directory = q[1][1]
		return "", nil
	case strings.HasPrefix(cmd, "/ip hotspot print count-only"):
		return "1", nil
	case cmd == `:put ("name: " . [/ip hotspot get [find where profile="guest" disabled=no] name])`:
		return "name: guest-server\nprofile: guest\n", nil
	case cmd == "/file print count-only where name=\"flash\"":
		return "1", nil
	case strings.HasPrefix(cmd, "/file print count-only"):
		if _, ok := r.files[q[0][1]]; ok {
			return "1", nil
		}
		return "0", nil
	case strings.HasPrefix(cmd, ":put [/file get"):
		v, ok := r.files[q[0][1]]
		if !ok {
			return "", errors.New("missing file")
		}
		return v + "\r\n", nil
	case strings.HasPrefix(cmd, "/file copy"):
		if strings.HasSuffix(q[0][1], ".html") {
			r.files[q[1][1]] = r.files[q[0][1]]
			return "", nil
		}
		r.files[q[1][1]] = "directory"
		for name, contents := range r.files {
			if strings.HasPrefix(name, q[0][1]+"/") {
				r.files[q[1][1]+strings.TrimPrefix(name, q[0][1])] = contents
			}
		}
		return "", nil
	case strings.HasPrefix(cmd, "/file set"):
		if r.failLogin {
			return "", errors.New("login write failed")
		}
		v := decodeRouterOSStringForTest(q[1][1])
		r.files[q[0][1]] = v
		return "", nil
	case strings.HasPrefix(cmd, "/ip hotspot walled-garden ip print count-only"):
		if strings.Contains(cmd, `"mm-media-guest"`) {
			if r.mediaGarden {
				return "1", nil
			}
			return "0", nil
		}
		if r.garden {
			return "1", nil
		}
		return "0", nil
	case strings.HasPrefix(cmd, ":local id [/ip hotspot walled-garden ip find where "):
		if strings.Contains(cmd, `"mm-media-guest"`) {
			return "dst-address: 192.168.88.10\ndst-port: 8096\nprotocol: tcp\naction: accept\n", nil
		}
		host := r.host
		if r.badRule {
			host = "bad.example"
		}
		return "dst-host: " + host + "\ndst-port: 443\nprotocol: tcp\nserver: guest-server\naction: accept\ndisabled: no\n", nil
	case strings.HasPrefix(cmd, "/ip hotspot walled-garden ip add"):
		if strings.Contains(cmd, `"mm-media-guest"`) {
			r.mediaGarden = true
			if r.failMediaAdd {
				return "", errors.New("lost response after media rule write")
			}
			return "", nil
		}
		r.garden = true
		return "", nil
	case strings.HasPrefix(cmd, "/ip hotspot walled-garden ip remove"):
		if strings.Contains(cmd, `"mm-media-guest"`) {
			if r.failMediaRemove {
				return "", errors.New("media cleanup failed")
			}
			r.mediaGarden = false
			return "", nil
		}
		r.garden = false
		return "", nil
	}
	return "", errors.New("unexpected command")
}
func newPortalRunner() *portalRunner {
	return &portalRunner{directory: "hotspot", host: portalHost, files: map[string]string{"hotspot/login.html": "<html>Original</html>", "hotspot/md5.js": "function hexMD5(input) { /* RouterOS helper */ }"}}
}

func TestPortalDeploymentBindsMediaTargetAndCleansUpFailures(t *testing.T) {
	for _, mode := range []string{"changed-target", "login-failure", "lost-add-response", "cleanup-failure"} {
		t.Run(mode, func(t *testing.T) {
			r := newPortalRunner()
			r.garden, r.host = true, portalHost
			a := portalTestApplier(r)
			request := PortalDeployment{Profile: "guest", MediaHost: "192.168.88.10", MediaPort: "8096"}
			plan, err := a.PreviewPortal(context.Background(), "biz", "device", request)
			if err != nil || plan.MediaHost != request.MediaHost || plan.MediaPort != request.MediaPort {
				t.Fatalf("review lost the media target: %+v %v", plan, err)
			}
			request.Version, request.Confirmation = plan.Version, plan.Confirmation
			r.commands = nil
			switch mode {
			case "changed-target":
				request.MediaHost = "192.168.88.11"
			case "lost-add-response":
				r.failMediaAdd = true
			default:
				r.failLogin = true
				r.failMediaRemove = mode == "cleanup-failure"
			}
			result, err := a.ApplyPortal(context.Background(), "biz", "device", request)
			if err == nil || result.Verified || r.directory != "hotspot" || !r.garden {
				t.Fatalf("failed deployment not contained: %+v %v", result, err)
			}
			if mode == "changed-target" {
				if portalHasWrites(r.commands) {
					t.Fatal("changed review target caused router writes")
				}
			} else if mode == "cleanup-failure" {
				if result.RolledBack || !r.mediaGarden {
					t.Fatal("reported rollback despite unremoved media rule")
				}
			} else if !result.RolledBack || r.mediaGarden {
				t.Fatalf("new media rule not removed: %+v", result)
			}
		})
	}
}
func portalHasWrites(commands []string) bool {
	for _, c := range commands {
		if strings.Contains(c, " set ") || strings.Contains(c, " add ") || strings.Contains(c, " copy ") || strings.Contains(c, " remove ") {
			return true
		}
	}
	return false
}
func TestMediaTargetValidation(t *testing.T) {
	cases := []struct {
		name string
		in   PortalDeployment
		ok   bool
	}{
		{"absent means no media", PortalDeployment{}, true},
		{"private host", PortalDeployment{MediaHost: "192.168.88.10"}, true},
		{"port override", PortalDeployment{MediaHost: "192.168.88.10", MediaPort: "8097"}, true},
		{"public address refused", PortalDeployment{MediaHost: "8.8.8.8"}, false},
		{"loopback refused", PortalDeployment{MediaHost: "127.0.0.1"}, false},
		{"unspecified refused", PortalDeployment{MediaHost: "0.0.0.0"}, false},
		{"hostname refused", PortalDeployment{MediaHost: "jellyfin.local"}, false},
		{"bad port", PortalDeployment{MediaHost: "192.168.88.10", MediaPort: "99999"}, false},
		{"port not a number", PortalDeployment{MediaHost: "192.168.88.10", MediaPort: "abc"}, false},
	}
	for _, tc := range cases {
		host, port, err := mediaTarget(tc.in)
		if tc.ok && err != nil {
			t.Fatalf("%s: expected accept, got %v", tc.name, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("%s: expected refusal, accepted host=%q port=%q", tc.name, host, port)
		}
		if tc.ok && tc.in.MediaHost != "" && (host == "" || port == "") {
			t.Fatalf("%s: accepted but produced empty target", tc.name)
		}
		if tc.ok && tc.in.MediaHost != "" && tc.in.MediaPort == "" && port != defaultMediaPort {
			t.Fatalf("%s: default port should be %s, got %s", tc.name, defaultMediaPort, port)
		}
	}
}

func TestPortalDeploymentPreviewAndApply(t *testing.T) {
	r := newPortalRunner()
	a := portalTestApplier(r)
	ctx := context.Background()
	profiles, err := a.PortalProfiles(ctx, "biz", "device")
	if err != nil || len(profiles) != 2 {
		t.Fatalf("profiles: %v %v", profiles, err)
	}
	plan, err := a.PreviewPortal(ctx, "biz", "device", PortalDeployment{Profile: "guest"})
	if err != nil {
		t.Fatal(err)
	}
	if portalHasWrites(r.commands) {
		t.Fatal("preview wrote to router")
	}
	result, err := a.ApplyPortal(ctx, "biz", "device", PortalDeployment{Profile: "guest", Version: plan.Version, Confirmation: plan.Confirmation})
	if err != nil || !result.Verified || !strings.HasPrefix(result.Backup, "flash/mm-portal-") || r.files[result.Backup] != "<html>Original</html>" {
		t.Fatalf("%+v %v", result, err)
	}
	expected, renderErr := RenderPortalLogin(nil, false)
	if renderErr != nil {
		t.Fatal(renderErr)
	}
	if !strings.HasPrefix(r.directory, "flash/mm-portal-") || r.files["hotspot/login.html"] != "<html>Original</html>" || r.files[result.Plan.File] != expected {
		t.Fatal("isolation or persistent staging failed")
	}
}
func TestPortalDeploymentRejectsStaleAndCrossDevicePreview(t *testing.T) {
	for _, mode := range []string{"changed", "other-device", "empty"} {
		t.Run(mode, func(t *testing.T) {
			r := newPortalRunner()
			a := portalTestApplier(r)
			ctx := context.Background()
			plan, err := a.PreviewPortal(ctx, "biz", "device", PortalDeployment{Profile: "guest"})
			if err != nil {
				t.Fatal(err)
			}
			id := "device"
			if mode == "changed" {
				r.files["hotspot/login.html"] = "changed"
			}
			if mode == "other-device" {
				id = "other"
			}
			if mode == "empty" {
				plan.Confirmation = ""
			}
			r.commands = nil
			_, err = a.ApplyPortal(ctx, "biz", id, PortalDeployment{Profile: "guest", Version: plan.Version, Confirmation: plan.Confirmation})
			if err == nil || portalHasWrites(r.commands) {
				t.Fatalf("accepted stale confirmation or wrote: %v", err)
			}
		})
	}
}
func TestPortalDeploymentRollsBackAmbiguousSwitchFailure(t *testing.T) {
	r := newPortalRunner()
	a := portalTestApplier(r)
	ctx := context.Background()
	plan, err := a.PreviewPortal(ctx, "biz", "device", PortalDeployment{Profile: "guest"})
	if err != nil {
		t.Fatal(err)
	}
	r.failSwitch = true
	result, err := a.ApplyPortal(ctx, "biz", "device", PortalDeployment{Profile: "guest", Version: plan.Version, Confirmation: plan.Confirmation})
	if err == nil || !result.RolledBack || r.directory != "hotspot" || r.garden {
		t.Fatalf("rollback: %+v %v", result, err)
	}
}
func TestPortalDeploymentRejectsInjectionAndConflictingRule(t *testing.T) {
	r := newPortalRunner()
	a := portalTestApplier(r)
	if _, err := a.PreviewPortal(context.Background(), "biz", "device", PortalDeployment{Profile: `guest"; /system reboot`}); err == nil {
		t.Fatal("accepted injection")
	}
	r.garden = true
	r.badRule = true
	if _, err := a.PreviewPortal(context.Background(), "biz", "device", PortalDeployment{Profile: "guest"}); err == nil {
		t.Fatal("accepted conflicting rule")
	}
	if portalHasWrites(r.commands) {
		t.Fatal("wrote during rejected preview")
	}
}

func TestPortalProfilesUsesExplicitReadOnlyGetters(t *testing.T) {
	r := newPortalRunner()
	profiles, err := portalTestApplier(r).PortalProfiles(context.Background(), "biz", "device")
	if err != nil || len(profiles) != 2 || len(r.commands) != 1 || r.commands[0] != portalProfilesCommand {
		t.Fatalf("profiles=%v err=%v commands=%v", profiles, err, r.commands)
	}
	if portalHasWrites(r.commands) || strings.Contains(r.commands[0], "value-list") {
		t.Fatal("profile discovery must use fixed read-only getter output")
	}
}

func TestPortalReviewAcceptsRouterOSListDelimiter(t *testing.T) {
	if !strings.Contains(portalProfileCommand(`name="guest"`), `[:tostr [/ip hotspot profile get $id login-by]]`) {
		t.Fatal("RouterOS arrays must be scalar before adding the output label")
	}
	r := newPortalRunner()
	runner := commandRunnerFunc(func(ctx context.Context, cmd string) (string, error) {
		out, err := r.Run(ctx, cmd)
		if cmd == portalProfileCommand(`name="guest"`) {
			out = strings.ReplaceAll(out, "http-chap,mac", "http-chap;mac")
		}
		return out, err
	})
	if _, err := portalTestApplier(runner).PreviewPortal(context.Background(), "biz", "device", PortalDeployment{Profile: "guest"}); err != nil {
		t.Fatal(err)
	}
	if portalHasWrites(r.commands) {
		t.Fatal("review modified router")
	}
}

func TestPortalRuleVerificationUsesScalarGetters(t *testing.T) {
	for _, media := range []bool{false, true} {
		command := portalGardenCommand(`comment="mm-portal-guest"`, media)
		if strings.Contains(command, "print value-list") || !strings.Contains(command, "[:len $id]!=1") {
			t.Fatal("rule read must be unique and independent of print formatting")
		}
		for _, field := range []string{"dst-port", "protocol", "server", "action", "disabled"} {
			if !strings.Contains(command, `get $id `+field+`]]`) {
				t.Fatalf("missing scalar %s", field)
			}
		}
	}
}

func TestPortalIPRuleSelectorDoesNotFilterUnsupportedDynamicFlag(t *testing.T) {
	r := newPortalRunner()
	a := portalTestApplier(r)
	plan, e := a.PreviewPortal(context.Background(), "biz", "device", PortalDeployment{Profile: "guest"})
	if e != nil {
		t.Fatal(e)
	}
	result, e := a.ApplyPortal(context.Background(), "biz", "device", PortalDeployment{Profile: "guest", Version: plan.Version, Confirmation: plan.Confirmation})
	if e != nil || !result.Verified {
		t.Fatal(e)
	}
	for _, cmd := range r.commands {
		if strings.Contains(cmd, "walled-garden ip") && strings.Contains(cmd, "dynamic=") {
			t.Fatal("IP walled-garden has no dynamic selector; it hides owned rules and breaks cleanup")
		}
	}
}

func TestPortalRejectsDuplicateOwnedRulesBeforeWrites(t *testing.T) {
	r := newPortalRunner()
	wrapped := commandRunnerFunc(func(ctx context.Context, cmd string) (string, error) {
		if strings.HasPrefix(cmd, `/ip hotspot walled-garden ip print count-only where comment="mm-portal-guest"`) {
			r.commands = append(r.commands, cmd)
			return "5", nil
		}
		return r.Run(ctx, cmd)
	})
	_, e := portalTestApplier(wrapped).PreviewPortal(context.Background(), "biz", "device", PortalDeployment{Profile: "guest"})
	if e == nil || !strings.Contains(e.Error(), "ambiguous") {
		t.Fatalf("duplicate rules must block deployment: %v", e)
	}
	if portalHasWrites(r.commands) {
		t.Fatal("ambiguous rules caused writes")
	}
}
