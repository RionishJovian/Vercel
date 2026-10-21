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
	directory  string
	files      map[string]string
	garden     bool
	commands   []string
	failSwitch bool
	badRule    bool
}

var portalQuoted = regexp.MustCompile(`"((?:\\.|[^"\\])*)"`)

func (r *portalRunner) Run(_ context.Context, cmd string) (string, error) {
	r.commands = append(r.commands, cmd)
	q := portalQuoted.FindAllStringSubmatch(cmd, -1)
	switch {
	case cmd == "/ip hotspot profile print value-list detail":
		return "name: default\nname: guest\n", nil
	case strings.HasPrefix(cmd, "/ip hotspot profile print count-only"):
		return "1\n", nil
	case strings.HasPrefix(cmd, "/ip hotspot profile print value-list"):
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
	case strings.HasPrefix(cmd, "/ip hotspot print value-list"):
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
		r.files[q[1][1]+"/login.html"] = r.files[q[0][1]+"/login.html"]
		return "", nil
	case strings.HasPrefix(cmd, "/file set"):
		v := q[1][1]
		v = strings.ReplaceAll(v, `\"`, `"`)
		v = strings.ReplaceAll(v, `\$`, `$`)
		v = strings.ReplaceAll(v, `\\`, `\`)
		r.files[q[0][1]] = v
		return "", nil
	case strings.HasPrefix(cmd, "/ip hotspot walled-garden ip print count-only"):
		if r.garden {
			return "1", nil
		}
		return "0", nil
	case strings.HasPrefix(cmd, "/ip hotspot walled-garden ip print value-list"):
		host := portalHost
		if r.badRule {
			host = "bad.example"
		}
		return "dst-host: " + host + "\ndst-port: 443\nprotocol: tcp\nserver: guest-server\naction: accept\ndisabled: no\n", nil
	case strings.HasPrefix(cmd, "/ip hotspot walled-garden ip add"):
		r.garden = true
		return "", nil
	case strings.HasPrefix(cmd, "/ip hotspot walled-garden ip remove"):
		r.garden = false
		return "", nil
	}
	return "", errors.New("unexpected command")
}
func newPortalRunner() *portalRunner {
	return &portalRunner{directory: "hotspot", files: map[string]string{"hotspot/login.html": "<html>Original</html>"}}
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
	if !strings.HasPrefix(r.directory, "flash/mm-portal-") || r.files["hotspot/login.html"] != "<html>Original</html>" || r.files[result.Plan.File] != portalLoginHTML() {
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
