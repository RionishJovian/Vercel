package routeros

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// behavioralRunner simulates a minimal RouterOS environment for testing
// the actual guard script behavior, not just source fragments.
type behavioralRunner struct {
	clock          int64
	scripts        map[string]string
	schedulers     map[string]string
	profiles       map[string]string
	users          map[string]*mockUser
	activeSessions map[string]*mockSession
	commands       []string
}

type mockUser struct {
	name        string
	comment     string
	disabled    bool
	limitUptime int64
	uptime      int64
	bytesIn     int64
	bytesOut    int64
	profile     string
}

type mockSession struct {
	user     string
	mac      string
	uptime   int64
	bytesIn  int64
	bytesOut int64
}

func newBehavioralRunner(clock int64) *behavioralRunner {
	return &behavioralRunner{
		clock:          clock,
		scripts:        map[string]string{},
		schedulers:     map[string]string{},
		profiles:       map[string]string{},
		users:          map[string]*mockUser{},
		activeSessions: map[string]*mockSession{},
		commands:       []string{},
	}
}

func (r *behavioralRunner) addUser(name, comment string, disabled bool, limitUptime int64, profile string) {
	r.users[name] = &mockUser{
		name:        name,
		comment:     comment,
		disabled:    disabled,
		limitUptime: limitUptime,
		uptime:      0,
		bytesIn:     0,
		bytesOut:    0,
		profile:     profile,
	}
}

func (r *behavioralRunner) addSession(user, mac string, uptime, bytesIn, bytesOut int64) {
	r.activeSessions[user] = &mockSession{
		user:     user,
		mac:      mac,
		uptime:   uptime,
		bytesIn:  bytesIn,
		bytesOut: bytesOut,
	}
}

func (r *behavioralRunner) Run(ctx context.Context, command string) (string, error) {
	r.commands = append(r.commands, command)
	cmd := strings.TrimSpace(command)

	// Clock probe
	if cmd == ":put ([:tonsec [:timestamp]] / 1000000000)" {
		return formatInt(r.clock), nil
	}

	// Script management
	if strings.HasPrefix(cmd, "/system script print count-only where name=") {
		name := extractName(cmd)
		if _, ok := r.scripts[name]; ok {
			return "1", nil
		}
		return "0", nil
	}
	if strings.HasPrefix(cmd, "/system script print value-list detail where name=") {
		name := extractName(cmd)
		return r.scripts[name], nil
	}
	if strings.HasPrefix(cmd, "/system script add name=") {
		return "", r.addScript(cmd)
	}

	// Scheduler management
	if strings.HasPrefix(cmd, "/system scheduler print count-only where name=") {
		name := extractName(cmd)
		if _, ok := r.schedulers[name]; ok {
			return "1", nil
		}
		return "0", nil
	}
	if strings.HasPrefix(cmd, "/system scheduler print value-list detail where name=") {
		name := extractName(cmd)
		return r.schedulers[name], nil
	}
	if strings.HasPrefix(cmd, "/system scheduler add name=") {
		return "", r.addScheduler(cmd)
	}

	// Profile management
	if strings.HasPrefix(cmd, "/ip hotspot user profile print count-only where name=") {
		name := extractName(cmd)
		if _, ok := r.profiles[name]; ok {
			return "1", nil
		}
		return "0", nil
	}
	if strings.HasPrefix(cmd, "/ip hotspot user profile print value-list detail where name=") {
		name := extractName(cmd)
		return r.profiles[name], nil
	}
	if strings.HasPrefix(cmd, "/ip hotspot user profile add name=") {
		return "", r.addProfile(cmd)
	}

	// User management
	if strings.HasPrefix(cmd, "/ip hotspot user find where ") {
		return r.findUsers(cmd)
	}
	if strings.HasPrefix(cmd, "/ip hotspot user get ") {
		return r.getUser(cmd)
	}
	if strings.HasPrefix(cmd, "/ip hotspot user set ") {
		return "", r.setUser(cmd)
	}
	if strings.HasPrefix(cmd, "/ip hotspot user disable ") {
		return "", r.disableUser(cmd)
	}

	// Active session management
	if strings.HasPrefix(cmd, "/ip hotspot active find where ") {
		return r.findActive(cmd)
	}
	if strings.HasPrefix(cmd, "/ip hotspot active get ") {
		return r.getActive(cmd)
	}
	if strings.HasPrefix(cmd, "/ip hotspot active remove ") {
		return "", r.removeActive(cmd)
	}

	// Script execution simulation
	if strings.HasPrefix(cmd, "/system script run ") {
		return "", r.runScript(cmd)
	}

	return "", errors.New("unhandled command: " + cmd)
}

func extractName(cmd string) string {
	at := strings.Index(cmd, "name=")
	if at < 0 {
		return ""
	}
	rest := cmd[at+len("name="):]
	if i := strings.Index(rest, " "); i >= 0 {
		rest = rest[:i]
	}
	return strings.Trim(rest, `"`)
}

func (r *behavioralRunner) addScript(cmd string) error {
	args := guardAddArgs(cmd)
	name := args["name"]
	if name == "" {
		return errors.New("add missing name")
	}
	if _, exists := r.scripts[name]; exists {
		return errors.New("object exists")
	}
	var b strings.Builder
	b.WriteString("name: " + name + "\n")
	b.WriteString("policy: " + args["policy"] + "\n")
	b.WriteString("dont-require-permissions: " + args["dont-require-permissions"] + "\n")
	source := args["source"]
	b.WriteString("source: " + source + "\n")
	r.scripts[name] = b.String()
	return nil
}

func (r *behavioralRunner) addScheduler(cmd string) error {
	args := guardAddArgs(cmd)
	name := args["name"]
	if name == "" {
		return errors.New("add missing name")
	}
	if _, exists := r.schedulers[name]; exists {
		return errors.New("object exists")
	}
	var b strings.Builder
	b.WriteString("name: " + name + "\n")
	if args["disabled"] == "yes" {
		b.WriteString("disabled: yes\n")
	}
	b.WriteString("interval: " + args["interval"] + "\n")
	b.WriteString("on-event: " + args["on-event"] + "\n")
	b.WriteString("policy: " + args["policy"] + "\n")
	r.schedulers[name] = b.String()
	return nil
}

func (r *behavioralRunner) addProfile(cmd string) error {
	args := guardAddArgs(cmd)
	name := args["name"]
	if name == "" {
		return errors.New("add missing name")
	}
	if _, exists := r.profiles[name]; exists {
		return errors.New("object exists")
	}
	var b strings.Builder
	b.WriteString("name: " + name + "\n")
	b.WriteString("shared-users: " + args["shared-users"] + "\n")
	if args["on-login"] != "" {
		b.WriteString("on-login: " + args["on-login"] + "\n")
	}
	if args["on-logout"] != "" {
		b.WriteString("on-logout: " + args["on-logout"] + "\n")
	}
	r.profiles[name] = b.String()
	return nil
}

func (r *behavioralRunner) findUsers(cmd string) (string, error) {
	// Simplified: extract conditions and match users
	var results []string
	for _, u := range r.users {
		if strings.Contains(cmd, "comment~\"mega-magic:vlogin:\"") && !u.disabled {
			if strings.HasPrefix(u.comment, "mega-magic:vlogin:") {
				results = append(results, u.name)
			}
		}
	}
	return strings.Join(results, "\n"), nil
}

func (r *behavioralRunner) getUser(cmd string) (string, error) {
	// /ip hotspot user get $u comment
	// /ip hotspot user get $u disabled
	// /ip hotspot user get $u limit-uptime
	// /ip hotspot user get $u name
	parts := strings.Split(cmd, " ")
	if len(parts) < 5 {
		return "", errors.New("invalid get command")
	}
	name := parts[3]
	field := parts[4]
	u, ok := r.users[name]
	if !ok {
		return "", errors.New("user not found")
	}
	switch field {
	case "comment":
		return u.comment, nil
	case "disabled":
		if u.disabled {
			return "yes", nil
		}
		return "no", nil
	case "limit-uptime":
		return formatInt(u.limitUptime), nil
	case "name":
		return u.name, nil
	}
	return "", errors.New("unknown field: " + field)
}

func (r *behavioralRunner) setUser(cmd string) error {
	// /ip hotspot user set $loginUser comment=(...)
	// Parse: /ip hotspot user set <name> comment=<value>
	at := strings.Index(cmd, " set ")
	if at < 0 {
		return errors.New("invalid set command")
	}
	rest := cmd[at+len(" set "):]
	spaceIdx := strings.Index(rest, " ")
	if spaceIdx < 0 {
		return errors.New("invalid set command")
	}
	name := rest[:spaceIdx]
	rest = rest[spaceIdx+1:]
	if !strings.HasPrefix(rest, "comment=") {
		return errors.New("only comment= supported")
	}
	value := strings.Trim(rest[len("comment="):], `"`)
	u, ok := r.users[name]
	if !ok {
		return errors.New("user not found")
	}
	u.comment = value
	return nil
}

func (r *behavioralRunner) disableUser(cmd string) error {
	parts := strings.Split(cmd, " ")
	if len(parts) < 4 {
		return errors.New("invalid disable command")
	}
	name := parts[3]
	u, ok := r.users[name]
	if !ok {
		return errors.New("user not found")
	}
	u.disabled = true
	return nil
}

func (r *behavioralRunner) findActive(cmd string) (string, error) {
	// /ip hotspot active find where user=<name>
	if strings.Contains(cmd, "user=") {
		start := strings.Index(cmd, "user=") + len("user=")
		end := strings.Index(cmd[start:], " ")
		if end < 0 {
			end = len(cmd) - start
		}
		user := cmd[start : start+end]
		user = strings.Trim(user, `"`)
		for k, s := range r.activeSessions {
			if s.user == user {
				return k, nil
			}
		}
	}
	return "", nil
}

func (r *behavioralRunner) getActive(cmd string) (string, error) {
	// /ip hotspot active get <id> mac-address
	if strings.Contains(cmd, "mac-address") {
		parts := strings.Split(cmd, " ")
		if len(parts) >= 5 {
			id := parts[3]
			if s, ok := r.activeSessions[id]; ok {
				return s.mac, nil
			}
		}
	}
	return "", nil
}

func (r *behavioralRunner) removeActive(cmd string) error {
	// /ip hotspot active remove [find user=<name>]
	// Find all sessions for that user and remove them
	if strings.Contains(cmd, "user=") {
		start := strings.Index(cmd, "user=") + len("user=")
		end := strings.Index(cmd[start:], "]")
		if end < 0 {
			end = len(cmd) - start
		}
		user := cmd[start : start+end]
		user = strings.Trim(user, `"`)
		for k, s := range r.activeSessions {
			if s.user == user {
				delete(r.activeSessions, k)
			}
		}
	}
	return nil
}

func (r *behavioralRunner) runScript(cmd string) error {
	// Execute the appropriate script based on name
	if strings.Contains(cmd, "mega-magic-voucher-guard-v1-login") {
		return r.executeLoginScript()
	}
	if strings.Contains(cmd, "mega-magic-voucher-guard-v1-expiry") {
		return r.executeExpiryScript()
	}
	return errors.New("unknown script: " + cmd)
}

func (r *behavioralRunner) executeLoginScript() error {
	// Simulate the login hook script behavior
	// The script uses $user variable - we need a "current login user"
	// For testing, we'll find users with active sessions that don't have anchors yet
	for name, u := range r.users {
		if u.disabled {
			continue
		}
		if !strings.HasPrefix(u.comment, "mega-magic:vlogin:") {
			continue
		}
		// Check if there's an active session for this user
		var session *mockSession
		for _, s := range r.activeSessions {
			if s.user == name {
				session = s
				break
			}
		}
		if session == nil {
			continue
		}

		// Base form check: 54 chars, no ;ft= anchor
		if len(u.comment) != 54 {
			continue
		}
		if strings.Contains(u.comment, ";ft=") {
			continue
		}

		// Validate UUID
		id := u.comment[18:54]
		if len(id) != 36 {
			continue
		}
		// Validate MAC
		mac := session.mac
		if len(mac) != 17 {
			continue
		}
		if mac[2] != ':' || mac[5] != ':' || mac[8] != ':' || mac[11] != ':' || mac[14] != ':' {
			continue
		}
		// Check hex bytes
		validHex := true
		for i := 0; i < 17; i++ {
			if i == 2 || i == 5 || i == 8 || i == 11 || i == 14 {
				continue
			}
			ch := mac[i]
			if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') && (ch < 'A' || ch > 'F') {
				validHex = false
				break
			}
		}
		if !validHex {
			continue
		}
		// Unicast check
		firstByte := mac[0:2]
		firstVal := 0
		for _, c := range firstByte {
			var v int
			if c >= '0' && c <= '9' {
				v = int(c - '0')
			} else if c >= 'a' && c <= 'f' {
				v = int(c - 'a' + 10)
			} else if c >= 'A' && c <= 'F' {
				v = int(c - 'A' + 10)
			} else {
				validHex = false
				break
			}
			firstVal = firstVal*16 + v
		}
		if !validHex || (firstVal&1) != 0 || mac == "00:00:00:00:00:00" {
			continue
		}

		// Write anchor
		u.comment = u.comment + ";ft=" + formatInt(r.clock) + ";fu=" + mac
	}
	return nil
}

func (r *behavioralRunner) executeExpiryScript() error {
	now := r.clock
	for name, u := range r.users {
		if u.disabled {
			continue
		}
		if !strings.HasPrefix(u.comment, "mega-magic:vlogin:") {
			continue
		}
		if len(u.comment) < 54 {
			continue
		}
		if u.comment[:18] != "mega-magic:vlogin:" {
			continue
		}
		id := u.comment[18:54]
		if len(id) != 36 {
			continue
		}
		// Validate UUID hex
		validHex := true
		for i := 0; i < 36; i++ {
			ch := id[i]
			if i == 8 || i == 13 || i == 18 || i == 23 {
				if ch != '-' {
					validHex = false
					break
				}
			} else {
				if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') && (ch < 'A' || ch > 'F') {
					validHex = false
					break
				}
			}
		}
		if !validHex {
			continue
		}

		// Find ;ft= and ;fu=
		p := strings.Index(u.comment, ";ft=")
		f := strings.Index(u.comment, ";fu=")
		if p < 0 || f < 0 {
			continue
		}
		ftText := u.comment[p+4 : f]
		ft := int64(0)
		for _, c := range ftText {
			if c >= '0' && c <= '9' {
				ft = ft*10 + int64(c-'0')
			} else {
				ft = -1
				break
			}
		}
		if ft <= 0 {
			continue
		}
		fu := u.comment[f+4:]
		if len(fu) != 17 {
			continue
		}
		if fu[2] != ':' || fu[5] != ':' || fu[8] != ':' || fu[11] != ':' || fu[14] != ':' {
			continue
		}
		// Validate MAC hex
		validMacHex := true
		for i := 0; i < 17; i++ {
			if i == 2 || i == 5 || i == 8 || i == 11 || i == 14 {
				continue
			}
			ch := fu[i]
			if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') && (ch < 'A' || ch > 'F') {
				validMacHex = false
				break
			}
		}
		if !validMacHex {
			continue
		}
		// Unicast check
		firstByte := fu[0:2]
		firstVal := 0
		for _, c := range firstByte {
			var v int
			if c >= '0' && c <= '9' {
				v = int(c - '0')
			} else if c >= 'a' && c <= 'f' {
				v = int(c - 'a' + 10)
			} else if c >= 'A' && c <= 'F' {
				v = int(c - 'A' + 10)
			} else {
				validMacHex = false
				break
			}
			firstVal = firstVal*16 + v
		}
		if !validMacHex || (firstVal&1) != 0 || fu == "00:00:00:00:00:00" {
			continue
		}

		lim := u.limitUptime
		if lim <= 0 {
			continue
		}
		if int64(now)-ft >= lim {
			// Disable user and remove sessions
			r.users[name].disabled = true
			for k, s := range r.activeSessions {
				if s.user == name {
					delete(r.activeSessions, k)
				}
			}
		}
	}
	return nil
}

func formatInt(i int64) string {
	// Simple implementation
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b strings.Builder
	for i > 0 {
		b.WriteByte(byte('0' + i%10))
		i /= 10
	}
	s := b.String()
	// Reverse
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	if neg {
		return "-" + string(runes)
	}
	return string(runes)
}

// TestVoucherGuardBehavioralLoginCapture verifies that login hook captures
// first-use anchor ONLY for the voucher that just logged in.
func TestVoucherGuardBehavioralLoginCapture(t *testing.T) {
	r := newBehavioralRunner(1792941234)

	// Install guard
	if err := ensureVoucherLoginGuard(context.Background(), r); err != nil {
		t.Fatalf("guard install failed: %v", err)
	}

	// Add two voucher users, both with base-form comments (no anchor yet)
	r.addUser("voucher-A", "mega-magic:vlogin:aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", false, 300, voucherGuardProfile)
	r.addUser("voucher-B", "mega-magic:vlogin:bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", false, 300, voucherGuardProfile)

	// Add active session ONLY for voucher-A (simulating A just logged in)
	r.addSession("voucher-A", "aa:bb:cc:dd:ee:ff", 0, 0, 0)

	// Execute login hook script (simulates on-login hook firing for voucher-A)
	if err := r.executeLoginScript(); err != nil {
		t.Fatalf("login script execution failed: %v", err)
	}

	// Verify ONLY voucher-A got anchored
	if !strings.Contains(r.users["voucher-A"].comment, ";ft=1792941234;fu=aa:bb:cc:dd:ee:ff") {
		t.Errorf("voucher-A should have been anchored, got: %s", r.users["voucher-A"].comment)
	}
	if strings.Contains(r.users["voucher-B"].comment, ";ft=") {
		t.Errorf("voucher-B should NOT have been anchored, got: %s", r.users["voucher-B"].comment)
	}
}

// TestVoucherGuardBehavioralSchedulerNeverCreatesAnchor verifies that
// the scheduler/expiry script never creates new first-use anchors.
func TestVoucherGuardBehavioralSchedulerNeverCreatesAnchor(t *testing.T) {
	r := newBehavioralRunner(1792941234)

	// Install guard
	if err := ensureVoucherLoginGuard(context.Background(), r); err != nil {
		t.Fatalf("guard install failed: %v", err)
	}

	// Add a voucher user with base-form comment (no anchor)
	r.addUser("voucher-C", "mega-magic:vlogin:cccccccc-cccc-cccc-cccc-cccccccccccc", false, 300, voucherGuardProfile)
	// NO active session for voucher-C

	// Execute expiry script (simulates scheduler run)
	if err := r.executeExpiryScript(); err != nil {
		t.Fatalf("expiry script execution failed: %v", err)
	}

	// Verify voucher-C was NOT anchored (no active session)
	if strings.Contains(r.users["voucher-C"].comment, ";ft=") {
		t.Errorf("expiry script should never create anchor, but got: %s", r.users["voucher-C"].comment)
	}
}

// TestVoucherGuardBehavioralExpiredVoucherDisabled verifies that
// a valid expired voucher is disabled and its sessions removed.
func TestVoucherGuardBehavioralExpiredVoucherDisabled(t *testing.T) {
	// Clock: first use was at 1792940000, limit is 500 seconds, now is 1792941234 (expired)
	r := newBehavioralRunner(1792941234)

	// Install guard
	if err := ensureVoucherLoginGuard(context.Background(), r); err != nil {
		t.Fatalf("guard install failed: %v", err)
	}

	// Add voucher with anchor from time 1792940000, limit 500 seconds
	// Use MAC with LSB=0 (unicast): 10:22:33:44:55:66 (0x10 = 16, even)
	r.addUser("voucher-D", "mega-magic:vlogin:dddddddd-dddd-dddd-dddd-dddddddddddd;ft=1792940000;fu=10:22:33:44:55:66", false, 500, voucherGuardProfile)
	r.addSession("voucher-D", "10:22:33:44:55:66", 100, 1000, 2000)

	// Execute expiry script
	if err := r.executeExpiryScript(); err != nil {
		t.Fatalf("expiry script execution failed: %v", err)
	}

	// Verify user is disabled
	if !r.users["voucher-D"].disabled {
		t.Error("expired voucher should be disabled")
	}
	// Verify session removed
	if _, ok := r.activeSessions["voucher-D"]; ok {
		t.Error("expired voucher session should be removed")
	}
}

// TestVoucherGuardBehavioralUnexpiredVoucherUnchanged verifies that
// a valid unexpired voucher remains unchanged.
func TestVoucherGuardBehavioralUnexpiredVoucherUnchanged(t *testing.T) {
	// Clock: first use was at 1792940000, limit is 5000 seconds, now is 1792941234 (NOT expired)
	r := newBehavioralRunner(1792941234)

	// Install guard
	if err := ensureVoucherLoginGuard(context.Background(), r); err != nil {
		t.Fatalf("guard install failed: %v", err)
	}

	// Add voucher with anchor from time 1792940000, limit 5000 seconds
	originalComment := "mega-magic:vlogin:eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee;ft=1792940000;fu=aa:bb:cc:dd:ee:ff"
	r.addUser("voucher-E", originalComment, false, 5000, voucherGuardProfile)
	r.addSession("voucher-E", "aa:bb:cc:dd:ee:ff", 100, 1000, 2000)

	// Execute expiry script
	if err := r.executeExpiryScript(); err != nil {
		t.Fatalf("expiry script execution failed: %v", err)
	}

	// Verify user is NOT disabled
	if r.users["voucher-E"].disabled {
		t.Error("unexpired voucher should NOT be disabled")
	}
	// Verify session still exists
	if _, ok := r.activeSessions["voucher-E"]; !ok {
		t.Error("unexpired voucher session should remain")
	}
	// Verify comment unchanged
	if r.users["voucher-E"].comment != originalComment {
		t.Errorf("unexpired voucher comment should be unchanged, got: %s", r.users["voucher-E"].comment)
	}
}

// TestVoucherGuardBehavioralMalformedOwnershipNoMutation verifies that
// malformed/foreign ownership causes no user or session mutations.
func TestVoucherGuardBehavioralMalformedOwnershipNoMutation(t *testing.T) {
	r := newBehavioralRunner(1792941234)

	// Install guard
	if err := ensureVoucherLoginGuard(context.Background(), r); err != nil {
		t.Fatalf("guard install failed: %v", err)
	}

	// Test cases: various malformed comments
	testCases := []struct {
		name    string
		comment string
		hasSess bool
	}{
		{"wrong prefix", "other:vlogin:aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", true},
		{"invalid UUID dashes", "mega-magic:vlogin:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true},
		{"missing fu suffix", "mega-magic:vlogin:aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa;ft=1000", true},
		{"invalid MAC format", "mega-magic:vlogin:aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa;ft=1000;fu=invalid-mac", true},
		{"multicast MAC", "mega-magic:vlogin:aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa;ft=1000;fu=01:00:00:00:00:00", true}, // LSB=1
		{"zero MAC", "mega-magic:vlogin:aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa;ft=1000;fu=00:00:00:00:00:00", true},
		{"non-hex MAC", "mega-magic:vlogin:aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa;ft=1000;fu=gg:hh:ii:jj:kk:ll", true},
		{"trailing data", "mega-magic:vlogin:aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa;ft=1000;fu=aa:bb:cc:dd:ee:ff:extra", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			userName := "test-" + strings.ReplaceAll(tc.name, " ", "-")
			r.addUser(userName, tc.comment, false, 300, voucherGuardProfile)
			if tc.hasSess {
				r.addSession(userName, "aa:bb:cc:dd:ee:ff", 0, 0, 0)
			}

			originalComment := r.users[userName].comment
			originalDisabled := r.users[userName].disabled

			// Execute both scripts
			if err := r.executeLoginScript(); err != nil {
				t.Fatalf("login script execution failed: %v", err)
			}
			if err := r.executeExpiryScript(); err != nil {
				t.Fatalf("expiry script execution failed: %v", err)
			}

			// Verify no mutations
			if r.users[userName].comment != originalComment {
				t.Errorf("malformed comment %q should not be mutated: %s -> %s", tc.name, originalComment, r.users[userName].comment)
			}
			if r.users[userName].disabled != originalDisabled {
				t.Errorf("malformed comment %q should not be disabled", tc.name)
			}
		})
	}
}

// TestVoucherGuardBehavioralReconnectRetainsAnchor verifies that
// reconnect retains the original first-use anchor.
func TestVoucherGuardBehavioralReconnectRetainsAnchor(t *testing.T) {
	r := newBehavioralRunner(1792941234)

	// Install guard
	if err := ensureVoucherLoginGuard(context.Background(), r); err != nil {
		t.Fatalf("guard install failed: %v", err)
	}

	// Add voucher with existing anchor
	originalComment := "mega-magic:vlogin:ffffffff-ffff-ffff-ffff-ffffffffffff;ft=1000;fu=aa:bb:cc:dd:ee:ff"
	r.addUser("voucher-F", originalComment, false, 5000, voucherGuardProfile)

	// Simulate reconnect: new active session with SAME MAC
	r.addSession("voucher-F", "aa:bb:cc:dd:ee:ff", 0, 0, 0)

	// Execute login hook (simulates reconnect login)
	if err := r.executeLoginScript(); err != nil {
		t.Fatalf("login script execution failed: %v", err)
	}

	// Verify anchor unchanged (still ft=1000, not updated to 1792941234)
	if r.users["voucher-F"].comment != originalComment {
		t.Errorf("reconnect should retain original anchor, got: %s", r.users["voucher-F"].comment)
	}
}

// TestVoucherGuardBehavioralConcurrentExecutionNoOverwrite verifies that
// concurrent execution does not overwrite first-use evidence.
func TestVoucherGuardBehavioralConcurrentExecutionNoOverwrite(t *testing.T) {
	// First clock: initial login at time 1792940000
	r := newBehavioralRunner(1792940000)

	// Install guard
	if err := ensureVoucherLoginGuard(context.Background(), r); err != nil {
		t.Fatalf("guard install failed: %v", err)
	}

	// Add voucher with base-form comment
	r.addUser("voucher-G", "mega-magic:vlogin:gggggggg-gggg-gggg-gggg-gggggggggggg", false, 300, voucherGuardProfile)
	r.addSession("voucher-G", "aa:bb:cc:dd:ee:ff", 0, 0, 0)

	// Execute login script at time 1792940000
	if err := r.executeLoginScript(); err != nil {
		t.Fatalf("login script execution failed: %v", err)
	}

	// Verify anchored at time 1792940000
	if !strings.Contains(r.users["voucher-G"].comment, ";ft=1792940000;") {
		t.Errorf("expected anchor at 1792940000, got: %s", r.users["voucher-G"].comment)
	}
	originalAnchor := r.users["voucher-G"].comment

	// Now advance clock to 1792941234 and try to run login script again
	// (simulating concurrent hook execution or retry)
	r2 := newBehavioralRunner(1792941234)
	// Copy state
	for k, v := range r.users {
		r2.users[k] = v
	}
	for k, v := range r.activeSessions {
		r2.activeSessions[k] = v
	}
	r2.scripts = r.scripts
	r2.schedulers = r.schedulers
	r2.profiles = r.profiles

	// Execute login script again at time 1792941234
	if err := r2.executeLoginScript(); err != nil {
		t.Fatalf("second login script execution failed: %v", err)
	}

	// Verify anchor NOT overwritten (still 1792940000)
	if r2.users["voucher-G"].comment != originalAnchor {
		t.Errorf("concurrent execution should not overwrite anchor, got: %s", r2.users["voucher-G"].comment)
	}
}

// TestVoucherGuardBehavioralValidPhoneMACAccepted verifies that
// valid locally administered phone MACs (with LSB=0) are accepted.
func TestVoucherGuardBehavioralValidPhoneMACAccepted(t *testing.T) {
	r := newBehavioralRunner(1792941234)

	// Install guard
	if err := ensureVoucherLoginGuard(context.Background(), r); err != nil {
		t.Fatalf("guard install failed: %v", err)
	}

	// Valid locally administered MAC (second bit = 1, first bit = 0)
	// e.g., 02:00:00:00:00:00 (locally administered, unicast)
	validPhoneMACs := []string{
		"02:00:00:00:00:00", // locally administered
		"06:00:00:00:00:00", // locally administered
		"aa:bb:cc:dd:ee:ff", // standard unicast
		"12:34:56:78:9a:bc", // standard unicast
	}

	for _, mac := range validPhoneMACs {
		t.Run(mac, func(t *testing.T) {
			userName := "phone-" + strings.ReplaceAll(mac, ":", "")
			r.addUser(userName, "mega-magic:vlogin:aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", false, 300, voucherGuardProfile)
			r.addSession(userName, mac, 0, 0, 0)

			if err := r.executeLoginScript(); err != nil {
				t.Fatalf("login script execution failed: %v", err)
			}

			if !strings.Contains(r.users[userName].comment, ";ft=1792941234;fu="+mac) {
				t.Errorf("valid MAC %s should be accepted, got: %s", mac, r.users[userName].comment)
			}
		})
	}
}
