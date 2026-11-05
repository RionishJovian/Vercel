package server

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"mega-magic/internal/hub"
)

// Structural checks for the scripts an operator pastes into a WinBox terminal.
// They cannot replace running the script on a router, but they catch the
// classes of mistake that silently break a paste: unbalanced blocks, undeclared
// variables, text RouterOS would substitute, and operators RouterOS 7.25 rejects.

var (
	quotedString = regexp.MustCompile(`"[^"\n]*"`)
	variableRef  = regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)`)
	declaration  = regexp.MustCompile(`:(?:local|global)\s+([A-Za-z_][A-Za-z0-9_]*)|:foreach\s+([A-Za-z_][A-Za-z0-9_]*)\s+in=`)
	commandPath  = regexp.MustCompile(`(?:^|[\[;{]\s*|do=\{\s*|else=\{\s*)(/[a-z][a-z0-9/-]*)`)
	knownPaths   = map[string]bool{
		"/interface/wireguard": true, "/interface/wireguard/peers": true, "/ip/address": true, "/ip/route": true,
		"/ip/service": true, "/ip/firewall/filter": true, "/ip/firewall/nat": true, "/user": true, "/user/group": true, "/user/ssh-keys": true, "/file": true,
	}
)

type reporter interface {
	Errorf(format string, args ...any)
}

func (p *failureProbe) Errorf(format string, args ...any) {
	p.failed = true
	p.messages = append(p.messages, fmt.Sprintf(format, args...))
}

func checkRouterOSBlock(t *testing.T, name, script string) {
	t.Helper()
	checkRouterOSBlockWith(t, name, script)
}

func checkRouterOSBlockWith(t reporter, name, script string) {
	fail := func(format string, args ...any) { t.Errorf(name+": "+format, args...) }
	// One final line break is allowed: it is what makes a paste run by itself.
	if body := strings.TrimSuffix(script, "\n"); !strings.HasPrefix(body, "{\n") || !strings.HasSuffix(body, "\n}") {
		fail("not a single braced block")
	}
	for _, bad := range []string{"%!", "%d", "%s", "<nil>", "(MISSING)", "(EXTRA", "\t", "\r"} {
		if strings.Contains(script, bad) {
			fail("contains %q", bad)
		}
	}
	for i, r := range script {
		if r > 126 || (r < 32 && r != '\n') {
			fail("non-ASCII or control character %q at offset %d (WinBox paste safe?)", r, i)
			break
		}
	}
	depth := map[rune]int{}
	pairs := map[rune]rune{')': '(', ']': '[', '}': '{'}
	topLevelCloses := 0
	maxLine := 0
	declared := map[string]bool{}
	for n, line := range strings.Split(script, "\n") {
		if len(line) > maxLine {
			maxLine = len(line)
		}
		if strings.Count(strings.ReplaceAll(line, `\"`, ""), `"`)%2 != 0 {
			fail("line %d has an unbalanced quote: %.80s", n+1, line)
		}
		code := line
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		// RouterOS substitutes $ and backslash sequences inside double quotes.
		for _, q := range quotedString.FindAllString(code, -1) {
			if strings.ContainsAny(q, `$\`) {
				fail("line %d: quoted text contains $ or backslash that RouterOS would interpret: %.80s", n+1, q)
			}
		}
		code = quotedString.ReplaceAllString(code, `""`)
		if regexp.MustCompile(`[<>]=?\s*""|""\s*[<>]=?`).MatchString(code) {
			fail("line %d orders a string with < or >; RouterOS 7.25 rejects that: %.80s", n+1, line)
		}
		for _, d := range declaration.FindAllStringSubmatch(code, -1) {
			declared[d[1]+d[2]] = true
		}
		for _, v := range variableRef.FindAllStringSubmatch(code, -1) {
			if !declared[v[1]] {
				fail("line %d uses $%s before it is declared", n+1, v[1])
			}
		}
		for _, p := range commandPath.FindAllStringSubmatch(code, -1) {
			if !knownPaths[p[1]] {
				fail("line %d uses unexpected command path %s", n+1, p[1])
			}
		}
		for _, r := range code {
			switch r {
			case '(', '[', '{':
				depth[r]++
			case ')', ']', '}':
				depth[pairs[r]]--
				if depth[pairs[r]] < 0 {
					fail("line %d closes %q that was never opened", n+1, r)
					depth[pairs[r]] = 0
				}
				if r == '}' && depth['{'] == 0 {
					topLevelCloses++
				}
			}
		}
	}
	for open, d := range depth {
		if d != 0 {
			fail("unbalanced %q (%d left open)", open, d)
		}
	}
	if topLevelCloses != 1 {
		fail("the block closes %d times at top level, want exactly 1", topLevelCloses)
	}
	if maxLine > 1500 {
		fail("longest line is %d characters; long pastes can be truncated by the terminal", maxLine)
	}
	if tt, ok := t.(*testing.T); ok {
		tt.Logf("%s: %d lines, longest %d chars", name, strings.Count(script, "\n")+1, maxLine)
	}
}

func TestOnePasteScriptsAreWellFormedRouterOS(t *testing.T) {
	private, public, err := hub.GenerateRouterKeypair()
	if err != nil {
		t.Fatal(err)
	}
	_, hubKey, err := hub.GenerateRouterKeypair()
	if err != nil {
		t.Fatal(err)
	}
	peer := hub.Peer{RouterID: "683a4cee-97fb-4a88-a4b4-f142fdbdc78c", RouterPublicKey: public, HubPublicKey: hubKey,
		Endpoint: "156.67.216.128:51820", AllowedIPs: []string{"10.77.0.1/32", "10.77.0.9/32"},
		EnrollmentVersion: 1, State: "active", TunnelAddress: "10.77.0.8/32"}
	scripts, err := hub.BuildRouterScripts(peer.RouterID, peer)
	if err != nil {
		t.Fatal(err)
	}
	quick, err := hub.BuildQuickConnectScript(peer.RouterID, peer, private)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := newPendingManagement("00000000-0000-0000-0000-000000000001", peer.RouterID, public, quick, "10.77.0.9/32")
	if err != nil {
		t.Fatal(err)
	}
	combined, err := automaticEnrollmentScript(peer.RouterID, pending)
	if err != nil {
		t.Fatal(err)
	}
	fullDisconnect, err := hub.BuildDisconnectScript(peer.RouterID, peer, pending.user)
	if err != nil {
		t.Fatal(err)
	}
	all := map[string]string{
		"step 1 bootstrap": scripts.Bootstrap, "step 2 setup": scripts.Setup, "disconnect (tunnel only)": scripts.Disconnect,
		"one-paste tunnel": quick, "one-paste tunnel + management": combined, "full disconnect": fullDisconnect,
	}
	for name, script := range all {
		checkRouterOSBlock(t, name, script)
	}
	// A script must not claim it leaves SSH and users alone while it changes them.
	for name, script := range all {
		changes := strings.Contains(script, "/user add") || strings.Contains(script, "/ip/service set") || strings.Contains(script, "/ip/firewall/filter add")
		if changes && strings.Contains(script, "Does not enable SSH") {
			t.Errorf("%s changes users/SSH/firewall but its header says it does not", name)
		}
	}
	if !strings.Contains(scripts.Disconnect, "NOT removed") || !strings.Contains(scripts.Disconnect, "were not removed") {
		t.Error("disconnect must say the management user, key and rule stay on the router")
	}
	// Full disconnect: setup and disconnect must agree on the account, and it must
	// remove only what setup created, in a safe order, and only when owned.
	if !strings.Contains(combined, `/user add name="`+pending.user+`"`) || !strings.Contains(fullDisconnect, `name="`+pending.user+`"`) {
		t.Error("setup and disconnect disagree about the management account name")
	}
	ordered := []string{`/user/ssh-keys remove`, `/user remove`, `/user/group remove`, `/interface/wireguard/peers remove`}
	last := -1
	for _, step := range ordered {
		i := strings.Index(fullDisconnect, step)
		if i < 0 || i < last {
			t.Errorf("full disconnect step %q missing or out of order", step)
		}
		last = i
	}
	if !strings.Contains(fullDisconnect, `comment="mega-magic:`+pending.user+`"`) || !strings.Contains(fullDisconnect, `] = "mega-magic:`+pending.user+`:"`) {
		t.Error("full disconnect does not check ownership before removing the rule and user")
	}
	for _, forbidden := range []string{"/ip/service", "private-key", "/system", "/ip/firewall/filter add"} {
		if strings.Contains(fullDisconnect, forbidden) {
			t.Errorf("full disconnect must not contain %q", forbidden)
		}
	}
	// The private WireGuard key may appear only in the one-paste scripts.
	for _, name := range []string{"step 1 bootstrap", "step 2 setup", "disconnect (tunnel only)", "full disconnect"} {
		if strings.Contains(all[name], private) {
			t.Errorf("%s leaks the router private key", name)
		}
	}
	if strings.Contains(combined, pending.private) {
		t.Error("management paste leaks the app's SSH private key")
	}
	if dir := os.Getenv("MM_DUMP_SCRIPTS"); dir != "" {
		for name, script := range all {
			_ = os.WriteFile(dir+"/"+strings.ReplaceAll(name, " ", "_")+".rsc.txt", []byte(script), 0o600)
		}
	}
}

// The checker must actually fail on the mistakes it exists to catch.
func TestRouterOSBlockCheckerCatchesDefects(t *testing.T) {
	good := "{\n:local a 1\n:if ($a = 1) do={ :put \"ok\" }\n}"
	cases := map[string]string{
		"undeclared variable":  "{\n:if ($missing = 1) do={ :put \"x\" }\n}",
		"unbalanced block":     "{\n:local a 1\n:if ($a = 1) do={ :put \"x\"\n}",
		"unbalanced quote":     "{\n:put \"x\n}",
		"dollar inside string": "{\n:put \"pa$word\"\n}",
		"string ordering":      "{\n:local c \"a\"\n:if ($c > \"9\") do={ :put \"x\" }\n}",
		"second top-level":     "{\n:put \"a\"\n}\n{\n:put \"b\"\n}",
		"unknown command path": "{\n/system/reset-configuration\n}",
		"non-ascii":            "{\n:put \"မှေ\"\n}",
		"format leftover":      "{\n:put \"%!s(MISSING)\"\n}",
	}
	for name, script := range cases {
		sub := &failureProbe{}
		runChecker(sub, name, script)
		if !sub.failed {
			t.Errorf("checker did not flag: %s", name)
		}
	}
	probe := &failureProbe{}
	runChecker(probe, "good", good)
	if probe.failed {
		t.Errorf("checker rejected a valid block: %v", probe.messages)
	}
}

type failureProbe struct {
	failed   bool
	messages []string
}

func runChecker(p *failureProbe, name, script string) {
	defer func() { _ = recover() }()
	checkRouterOSBlockWith(p, name, script)
}
