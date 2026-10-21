package routeros

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// The deployed login page is served BY the RouterBOARD and validates the
// voucher code against its own user list. It must not redirect anywhere.
func TestPortalLoginHTMLIsRouterLocal(t *testing.T) {
	page := portalLoginHTML()
	for _, want := range []string{
		`action="$(link-login-only)"`, // posts to the router itself
		`name="username"`,
		`name="password"`,
		`src="$(link-login-only)?only-target=iframe&amp;target=%2Fmd5.js"`, // RouterOS CHAP helper
		`'$(chap-id)'`,
		`'$(chap-challenge)'`,
		`$(if error)`, // login failure surfaced by the router
		`$(link-orig)`,
		`id="voucher"`,
		`doLogin`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("router-local login page missing %q", want)
		}
	}
	for _, banned := range []string{"megamagic.tech", "location.replace", "deviceId=", "10.5.50.", "portalOrigin"} {
		if strings.Contains(page, banned) {
			t.Errorf("router-local login page must not reference %q", banned)
		}
	}
	if abs := len(page); abs > 24000 {
		t.Errorf("login page is %d bytes; the deploy preflight only stages files up to 24 KB", abs)
	}
}

// The preflight refuses a profile that cannot validate a voucher form login
// locally (no HTTP CHAP or PAP): the deployed page would never authenticate.
func TestPortalPreflightRequiresFormLogin(t *testing.T) {
	runs := 0
	var loginBy string
	r := stubRunner{run: func(ctx context.Context, command string) (string, error) {
		runs++
		switch {
		case strings.HasPrefix(command, "/ip hotspot profile print count-only"):
			return "1", nil
		case strings.HasPrefix(command, "/ip hotspot profile print value-list"):
			return "name: hsprof\nlogin-by: " + loginBy + "\nhtml-directory: hotspot\n", nil
		case strings.HasPrefix(command, "/file print count-only where name=\"flash\""):
			return "0", nil
		case strings.Contains(command, `name="hotspot/login.html"`):
			return "1", nil
		case strings.HasPrefix(command, "/file print count-only where name="):
			return "0", nil
		case strings.HasPrefix(command, "/ip hotspot print count-only where profile="):
			return "1", nil
		case strings.HasPrefix(command, "/ip hotspot print value-list"):
			return "name: hs1\n", nil
		case strings.Contains(command, "/file get [find name="):
			return "<html>original</html>", nil
		case strings.HasPrefix(command, "/ip hotspot walled-garden ip print count-only"):
			return "0", nil
		}
		return "", errors.New("unexpected command: " + command)
	}}
	base := func() portalState {
		_, err := portalPreflight(context.Background(), r, "biz", "dev", PortalDeployment{Profile: "hsprof", Version: "0123456789abcdef"})
		if err == nil {
			t.Fatal("expected preflight error")
		}
		return portalState{}
	}
	loginBy = "cookie,mac"
	_ = base()
	loginBy = "mac,http-chap"
	if s, err := portalPreflight(context.Background(), r, "biz", "dev", PortalDeployment{Profile: "hsprof", Version: "0123456789abcdef"}); err != nil {
		t.Fatalf("http-chap profile must pass the form-login gate: %v", err)
	} else if !strings.Contains(strings.Join(s.plan.Steps, " "), "router-served voucher login page") {
		t.Errorf("plan steps must describe the router-local page, got: %v", s.plan.Steps)
	}
	loginBy = "mac,http-pap"
	if _, err := portalPreflight(context.Background(), r, "biz", "dev", PortalDeployment{Profile: "hsprof", Version: "0123456789abcdef"}); err != nil {
		t.Fatalf("http-pap profile must pass the form-login gate: %v", err)
	}
	if runs == 0 {
		t.Fatal("stub runner was never used")
	}
}

type stubRunner struct {
	run func(ctx context.Context, command string) (string, error)
}

func (s stubRunner) Run(ctx context.Context, command string) (string, error) {
	return s.run(ctx, command)
}
