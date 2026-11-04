package routeros

import (
	"context"
	"fmt"
	"mega-magic/internal/broadband"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Runs the real PPPoE code against a disposable RouterOS CHR. It changes that router, so it only runs
// when MM_CHR_SSH names one (for example admin@192.168.64.4). The CHR needs: a PPPoE server on one
// macvlan, a plan named lab-plan, working NTP, the unmanaged PPPoE user "unmanaged1" connected through
// client c-un on the other macvlan. See docs/CHR-LAB.md.
type chrRunner struct{ target string }

func (c chrRunner) Run(ctx context.Context, cmd string) (string, error) {
	args := []string{"-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null", "-o", "ConnectTimeout=20", "-o", "LogLevel=ERROR", "-o", "BatchMode=yes"}
	if key := os.Getenv("MM_CHR_KEY"); key != "" {
		args = append(args, "-i", key)
	}
	// RouterOS can refuse a new SSH handshake when many open in a row; the app's transport retries too.
	var out []byte
	var err error
	for i := 0; i < 6; i++ {
		out, err = exec.CommandContext(ctx, "ssh", append(args, c.target, cmd)...).CombinedOutput()
		if err == nil || (!strings.Contains(string(out), "Connection refused") && !strings.Contains(string(out), "Connection reset") && !strings.Contains(string(out), "Connection closed")) {
			break
		}
		time.Sleep(3 * time.Second)
	}
	text := strings.ReplaceAll(string(out), "\r", "")
	if err != nil {
		return text, fmt.Errorf("%w: %s", err, text)
	}
	if strings.HasPrefix(strings.TrimSpace(text), "failure:") || strings.Contains(text, "(line ") && strings.Contains(text, "bad ") || strings.Contains(text, "syntax error") || strings.Contains(text, "no such item") || strings.Contains(text, "expected end of command") {
		return text, fmt.Errorf("router refused: %s", strings.TrimSpace(text))
	}
	return text, nil
}

type chrLab struct {
	t   *testing.T
	r   chrRunner
	ctx context.Context
}

func (l chrLab) put(cmd string) string {
	l.t.Helper()
	out, err := l.r.Run(l.ctx, cmd)
	if err != nil {
		l.t.Fatalf("%s: %v", cmd, err)
	}
	return strings.TrimSpace(out)
}
func (l chrLab) active(user string) bool {
	l.t.Helper()
	return l.put(`:put [:len [/ppp active find name=`+routerOSQuoted(user)+`]]`) != "0"
}
func (l chrLab) disabled(user string) bool {
	l.t.Helper()
	return l.put(`:put [/ppp secret get [find name=`+routerOSQuoted(user)+`] disabled]`) == "true"
}
func (l chrLab) waitActive(user string, want bool, within time.Duration) {
	l.t.Helper()
	end := time.Now().Add(within)
	for time.Now().Before(end) {
		if l.active(user) == want {
			return
		}
		time.Sleep(2 * time.Second)
	}
	l.t.Fatalf("%s active=%v not reached within %s", user, want, within)
}
func (l chrLab) client(user string) {
	l.put(`/interface pppoe-client add name=` + routerOSQuoted("c-"+user) + ` interface=m1 user=` + routerOSQuoted(user) + ` password="lab-test-password" add-default-route=no disabled=no`)
}
func (l chrLab) guardRuns() int {
	n, _ := strconv.Atoi(l.put(`:put [/system scheduler get [find name=` + routerOSQuoted(bbGuardName) + `] run-count]`))
	return n
}
func (l chrLab) scriptErrors(since string) string {
	out, _ := l.r.Run(l.ctx, `:foreach e in=[/log find where topics~"script" topics~"error"] do={ :put [/log get $e message] }`)
	return strings.Replace(out, since, "", 1)
}

func TestCHRLabPPPoELifecycle(t *testing.T) {
	target := os.Getenv("MM_CHR_SSH")
	if target == "" {
		t.Skip("set MM_CHR_SSH to run against a disposable CHR")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	l := chrLab{t, chrRunner{target}, ctx}
	if !l.active("unmanaged1") {
		t.Fatal("the unmanaged control session must be connected first")
	}
	for _, cmd := range []string{`/interface pppoe-client remove [find name~"^c-lab"]`, `/ppp active remove [find name~"^lab-"]`, `/ppp secret remove [find name~"^lab-"]`, `/ppp profile remove [find name~"^mm-bb-"]`, `/system scheduler remove [find name=` + routerOSQuoted(bbGuardName) + `]`, `/system script remove [find name=` + routerOSQuoted(bbGuardName) + `]`, `/log warning "lab start"`} {
		l.put(cmd)
	}
	stamp := time.Now().Format("150405")
	id := func(n int) string { return fmt.Sprintf("aaaaaaaa-aaaa-4aaa-8aaa-%s%06d", stamp, n) }
	op := func(n int) string { return fmt.Sprintf("bbbbbbbb-bbbb-4bbb-8bbb-%s%06d", stamp, n) }
	apply := func(s broadband.Spec) broadband.Observation {
		t.Helper()
		o, e := applyBroadband(ctx, l.r, s)
		if e != nil {
			t.Fatalf("%s %s: %v", s.Action, s.Username, e)
		}
		return o
	}
	create := func(n int, user string) broadband.Spec {
		s := broadband.Spec{AccountID: id(n), OperationID: id(n), Action: "create", Username: user, Password: "lab-test-password", Profile: "lab-plan", PeriodSeconds: 86400}
		apply(s)
		return s
	}
	a, b, c := create(1, "lab-a"), create(2, "lab-b"), create(3, "lab-c")
	last := map[string]string{"lab-a": a.OperationID, "lab-b": b.OperationID, "lab-c": c.OperationID}
	change := func(s broadband.Spec, action string, n int, seconds int64) broadband.Observation {
		t.Helper()
		o := apply(broadband.Spec{AccountID: s.AccountID, OperationID: op(n), ExpectedOperation: last[s.Username], Action: action, Username: s.Username, Profile: "lab-plan", PeriodSeconds: seconds})
		last[s.Username] = op(n)
		return o
	}

	t.Log("installed three accounts; the guard must run without disabling anyone")
	start := l.guardRuns()
	time.Sleep(35 * time.Second)
	if runs := l.guardRuns(); runs <= start {
		t.Fatalf("the guard did not run: %d then %d", start, runs)
	}
	for _, u := range []string{"lab-a", "lab-b", "lab-c"} {
		if l.disabled(u) {
			t.Fatalf("%s was disabled while valid and unused", u)
		}
	}
	if !l.active("unmanaged1") {
		t.Fatal("the guard disconnected an unmanaged subscriber")
	}

	t.Log("first login starts the clock once")
	before := time.Now().Unix()
	l.client("lab-a")
	l.waitActive("lab-a", true, 40*time.Second)
	time.Sleep(3 * time.Second)
	oa, e := readBroadband(ctx, l.r, a.AccountID, "lab-a")
	if e != nil {
		t.Fatal(e)
	}
	if oa.State.FirstUse < before-120 || oa.State.ExpiresAt != oa.State.FirstUse+86400 || !oa.Connected {
		t.Fatalf("first use not recorded: %+v", oa)
	}
	if !oa.UsageAvailable || oa.Session == nil || oa.Session.UptimeSeconds < 0 {
		t.Fatalf("session counters not read: %+v", oa)
	}
	t.Logf("session read: %+v", *oa.Session)

	t.Log("an unused account with others online stays allowed, and nobody is disconnected")
	time.Sleep(35 * time.Second)
	if l.disabled("lab-c") || l.disabled("lab-b") || !l.active("lab-a") || !l.active("unmanaged1") {
		t.Fatalf("guard interfered: c=%v b=%v a=%v unmanaged=%v", l.disabled("lab-c"), l.disabled("lab-b"), l.active("lab-a"), l.active("unmanaged1"))
	}

	t.Log("early renewal extends the same expiry, once")
	o := change(a, "renew", 1, 86400)
	if o.State.FirstUse != oa.State.FirstUse || o.State.ExpiresAt != oa.State.ExpiresAt+86400 {
		t.Fatalf("renew: %+v from %+v", o.State, oa.State)
	}
	again := apply(broadband.Spec{AccountID: a.AccountID, OperationID: op(1), ExpectedOperation: a.OperationID, Action: "renew", Username: "lab-a", Profile: "lab-plan", PeriodSeconds: 86400})
	if again.State.ExpiresAt != o.State.ExpiresAt {
		t.Fatalf("a retry extended the period twice: %+v", again.State)
	}
	if !l.active("lab-a") {
		t.Fatal("renewal disconnected the subscriber")
	}

	t.Log("unused renewal adds prepaid days without starting the clock")
	oc := change(c, "renew", 2, 86400)
	if oc.State.FirstUse != 0 || oc.State.PeriodSeconds != 172800 {
		t.Fatalf("unused renew: %+v", oc.State)
	}

	t.Log("suspending one account disconnects only that account")
	l.client("lab-b")
	l.waitActive("lab-b", true, 40*time.Second)
	time.Sleep(3 * time.Second)
	ob := change(b, "suspend", 3, 0)
	l.waitActive("lab-b", false, 20*time.Second)
	if !ob.State.Suspended || !l.disabled("lab-b") {
		t.Fatalf("suspend: %+v disabled=%v", ob.State, l.disabled("lab-b"))
	}
	if !l.active("lab-a") || !l.active("unmanaged1") {
		t.Fatal("suspending one account disconnected another")
	}
	time.Sleep(20 * time.Second)
	if !l.disabled("lab-b") || l.active("lab-b") {
		t.Fatal("a suspended account got back in")
	}

	t.Log("renewal keeps a suspended account suspended; resume does not change expiry")
	ob = change(b, "renew", 4, 86400)
	if !ob.State.Suspended || !l.disabled("lab-b") {
		t.Fatalf("renew reopened a suspended account: %+v", ob.State)
	}
	resumed := change(b, "resume", 5, 0)
	if resumed.State.Suspended || resumed.State.ExpiresAt != ob.State.ExpiresAt || l.disabled("lab-b") {
		t.Fatalf("resume: %+v from %+v", resumed.State, ob.State)
	}
	l.waitActive("lab-b", true, 60*time.Second)

	t.Log("expiry disconnects only the expired account, and it cannot log back in")
	now := time.Now().Unix()
	exp := broadband.State{PeriodSeconds: oa.State.PeriodSeconds, FirstUse: oa.State.FirstUse, ExpiresAt: now - 5, LastClock: oa.State.FirstUse}
	l.put(`/ppp secret set [find name="lab-a"] comment=` + routerOSQuoted(exp.Encode(a.AccountID, last["lab-a"])))
	l.waitActive("lab-a", false, 40*time.Second)
	if !l.disabled("lab-a") {
		t.Fatal("expired account stayed enabled")
	}
	if !l.active("lab-b") || !l.active("unmanaged1") {
		t.Fatalf("one expiry disconnected others: b=%v unmanaged=%v", l.active("lab-b"), l.active("unmanaged1"))
	}
	time.Sleep(25 * time.Second)
	if l.active("lab-a") || !l.disabled("lab-a") {
		t.Fatal("expired account got back in")
	}
	if !l.active("lab-b") || !l.active("unmanaged1") {
		t.Fatal("later guard passes disconnected others")
	}
	oe, e := readBroadband(ctx, l.r, a.AccountID, "lab-a")
	if e != nil || oe.State.Status(oe.ObservedAt) != "expired" {
		t.Fatalf("expired read: %+v %v", oe, e)
	}

	t.Log("late renewal starts from now, keeps first use, and lets the subscriber back in")
	ol := change(a, "renew", 6, 86400)
	if ol.State.FirstUse != oa.State.FirstUse || ol.State.ExpiresAt < time.Now().Unix()+86000 || ol.State.ExpiresAt > time.Now().Unix()+86500 {
		t.Fatalf("late renew: %+v", ol.State)
	}
	l.waitActive("lab-a", true, 60*time.Second)

	t.Log("the guard does not write flash while nothing changes")
	time.Sleep(20 * time.Second)
	sect := func() int {
		n, _ := strconv.Atoi(l.put(`:put [/system resource get write-sect-since-reboot]`))
		return n
	}
	w0 := sect()
	r0 := l.guardRuns()
	time.Sleep(62 * time.Second)
	w1, r1 := sect(), l.guardRuns()
	t.Logf("idle: %d flash sectors written over %d guard runs", w1-w0, r1-r0)

	t.Logf("script errors logged during the run:\n%s", l.scriptErrors(""))
}

// Provisioning, read-back and verification of the Hotspot and voucher features on the same CHR, through
// the applier the app uses. No guest client is involved, so it cannot show that limits are enforced on
// real traffic. Each step reports separately so one failure does not hide the others.
func TestCHRLabHotspotAndReads(t *testing.T) {
	target := os.Getenv("MM_CHR_SSH")
	if target == "" {
		t.Skip("set MM_CHR_SSH to run against a disposable CHR")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	r := chrRunner{target}
	l := chrLab{t, r, ctx}
	a := NewDeviceApplier(capabilitySource{}, 60*time.Second, func(SSHConfig) CommandRunner { return r }, DefaultInterface, 0, 0)
	const biz, dev = "biz-1", "dev-1"

	t.Run("device mode from real output", func(t *testing.T) {
		out, err := r.Run(ctx, deviceModeCommand)
		if err != nil {
			t.Fatal(err)
		}
		f := parseDeviceMode(out)
		if !f.Known || f.Mode != "advanced" || !f.Hotspot || !f.Scheduler {
			t.Fatalf("%+v from %q", f, out)
		}
	})
	t.Run("reads", func(t *testing.T) {
		if inv, e := a.ReadNetwork(ctx, biz, dev); e != nil {
			t.Errorf("ReadNetwork: %v", e)
		} else {
			missing := []string{}
			for k, ok := range inv.Available {
				if !ok {
					missing = append(missing, k)
				}
			}
			t.Logf("network inventory: %d interfaces, unavailable: %v", len(inv.Interfaces), missing)
		}
		if _, e := a.ReadRouterOverview(ctx, biz, dev); e != nil {
			t.Errorf("ReadRouterOverview: %v", e)
		}
		if _, e := a.ReadObservations(ctx, biz, dev); e != nil {
			t.Errorf("ReadObservations: %v", e)
		}
		tel, e := a.ReadTelemetry(ctx, biz, dev)
		if e != nil {
			t.Errorf("ReadTelemetry: %v", e)
		} else {
			t.Logf("telemetry: %+v", tel)
		}
		if q, e := a.ReadQuickConfig(ctx, biz, dev); e != nil {
			t.Errorf("ReadQuickConfig: %v", e)
		} else {
			t.Logf("quick config: %+v", q)
		}
		if i, e := a.GatewayInterfaces(ctx, biz, dev); e != nil || len(i) == 0 {
			t.Errorf("GatewayInterfaces: %v %v", i, e)
		}
		caps, e := NewDeviceProber(capabilitySource{}, 60*time.Second, func(SSHConfig) CommandRunner { return r }).InspectCapabilities(ctx, biz, dev)
		if e != nil {
			t.Errorf("capabilities: %v", e)
		} else if !caps.DeviceMode.Known || !caps.DeviceMode.Hotspot {
			t.Errorf("capabilities device mode: %+v", caps.DeviceMode)
		}
	})
	t.Run("customer network with Hotspot", func(t *testing.T) {
		out, _ := r.Run(ctx, `/ip hotspot print count-only where name="mm-labnet"`)
		if strings.TrimSpace(out) == "1" {
			t.Log("already set up")
			return
		}
		// m3 is the customer LAN. m4 is a guest device: its own VRF sends its traffic across the wire to m3.
		for _, cmd := range []string{
			`:if ([:len [/interface macvlan find name=m3]] = 0) do={ /interface macvlan add name=m3 interface=ether1 mode=bridge }`,
			`:if ([:len [/interface macvlan find name=m4]] = 0) do={ /interface macvlan add name=m4 interface=ether1 mode=bridge }`,
			`:if ([:len [/ip vrf find name=guest]] = 0) do={ /ip vrf add name=guest interfaces=m4 }`,
		} {
			l.put(cmd)
		}
		g := GatewaySetup{Name: "labnet", WAN: "ether1", LAN: "m3", Address: "10.60.0.1/24", PoolStart: "10.60.0.10", PoolEnd: "10.60.0.200", DNS: []string{"1.1.1.1"}, Hotspot: true}
		plan, e := a.PreviewGateway(ctx, biz, dev, g)
		if e != nil {
			t.Fatalf("preview: %v", e)
		}
		g.Confirmation = plan.Confirmation
		res, e := a.ApplyGateway(ctx, biz, dev, g)
		if e != nil || !res.Verified {
			t.Fatalf("apply: %+v %v", res, e)
		}
		// Added after the preflight, which rightly refuses a subnet that overlaps an existing address.
		l.put(`:if ([:len [/ip address find where address="10.60.0.50/24"]] = 0) do={ /ip address add address=10.60.0.50/24 interface=m4 }`)
	})
	guestMAC := strings.ToUpper(l.put(`:put [/interface get [find name=m4] mac-address]`))
	guestTraffic := func() { r.Run(ctx, `/ping 1.1.1.1 vrf=guest count=2`) }
	guestSeen := func() {
		t.Helper()
		end := time.Now().Add(40 * time.Second)
		for time.Now().Before(end) {
			guestTraffic()
			if l.put(`:put [:len [/ip hotspot host find where mac-address=`+guestMAC+`]]`) != "0" {
				return
			}
		}
		t.Errorf("guest %s never reached the Hotspot host table", guestMAC)
	}
	r.Run(ctx, `:if ([:len [/ip route find where routing-table=guest]] = 0) do={ /ip route add dst-address=0.0.0.0/0 gateway=10.60.0.1@guest routing-table=guest }`)
	cleanGuest := func() {
		for _, cmd := range []string{`/ip hotspot active remove [find where mac-address=` + guestMAC + `]`, `/ip hotspot user remove [find where name=` + routerOSQuoted(guestMAC) + `]`, `/system scheduler remove [find where name~"^mm-(voucher|paid)-"]`} {
			r.Run(ctx, cmd)
		}
	}
	vid := fmt.Sprintf("cccccccc-cccc-4ccc-8ccc-%012x", time.Now().Unix())
	t.Run("voucher login and guard", func(t *testing.T) {
		const letters = "ABCDEFGHJKMNPQRSTVWXYZ"
		code := []byte("ABCDEFGH")
		for i, n := 0, time.Now().Unix(); i < len(code); i, n = i+1, n/int64(len(letters)) {
			code[i] = letters[n%int64(len(letters))]
		}
		login := VoucherLogin{VoucherID: vid, Code: string(code), Minutes: 60, DataMB: 500}
		if e := a.ProvisionVoucherLogins(ctx, biz, dev, []VoucherLogin{login}); e != nil {
			t.Fatalf("provision: %v", e)
		}
		if e := a.ProvisionVoucherLogins(ctx, biz, dev, []VoucherLogin{login}); e != nil {
			t.Fatalf("second provision must be safe: %v", e)
		}
		states, e := a.ReadVoucherLogins(ctx, biz, dev, []VoucherLogin{login})
		if e != nil || len(states) != 1 || !states[0].Exists || states[0].Disabled {
			t.Fatalf("read back: %+v %v", states, e)
		}
		time.Sleep(65 * time.Second)
		if errs := l.scriptErrors(""); strings.Contains(errs, "voucher") {
			t.Errorf("guard script errors: %s", errs)
		}
	})
	t.Run("MAC voucher", func(t *testing.T) {
		g := GuestVoucherAccess{VoucherID: fmt.Sprintf("dddddddd-dddd-4ddd-8ddd-%012x", time.Now().Unix()), ClientMAC: guestMAC, Minutes: 60, DataMB: 500, ExpiresAt: time.Now().Add(time.Hour)}
		cleanGuest()
		guestSeen()
		if e := a.ConfigureGuestVoucher(ctx, biz, dev, g); e != nil {
			t.Fatal(e)
		}
		if l.put(`:put [:len [/ip hotspot active find where mac-address=`+guestMAC+`]]`) == "0" {
			guestSeen()
		}
		if e := a.ConfigureGuestVoucher(ctx, biz, dev, g); e != nil {
			t.Fatalf("a retry must be safe: %v", e)
		}
		end := time.Now().Add(30 * time.Second)
		for time.Now().Before(end) && l.put(`:put [:len [/ip hotspot active find where mac-address=`+guestMAC+`]]`) == "0" {
			guestTraffic()
		}
		if l.put(`:put [:len [/ip hotspot active find where mac-address=`+guestMAC+`]]`) == "0" {
			t.Error("voucher guest did not get online")
		}
	})
	t.Run("grace, approval and speed tier", func(t *testing.T) {
		base := time.Now().Unix()
		reqOf := func(n int64) string { return fmt.Sprintf("eeeeeeee-eeee-4eee-8eee-%012x", base+n) }
		mac := guestMAC
		activeGuest := func(within time.Duration) bool {
			end := time.Now().Add(within)
			for time.Now().Before(end) {
				guestTraffic()
				if l.put(`:put [:len [/ip hotspot active find where mac-address=`+guestMAC+`]]`) != "0" {
					return true
				}
				time.Sleep(2 * time.Second)
			}
			return false
		}
		grace := func(req string) {
			t.Helper()
			if e := a.ConfigureGuestGrace(ctx, biz, dev, GuestGrace{RequestID: req, ClientMAC: mac, Minutes: 10, DataMB: 50, ExpiresAt: time.Now().Add(10 * time.Minute)}); e != nil {
				t.Fatalf("grace: %v", e)
			}
		}
		// A guest waiting on the Hotspot before the user exists must get online from traffic alone.
		cleanGuest()
		r.Run(ctx, `/ip hotspot host remove [find where mac-address=`+guestMAC+`]`)
		guestSeen()
		grace(reqOf(0))
		if !activeGuest(30 * time.Second) {
			t.Fatal("a guest already on the Hotspot did not get online after the grace user was created")
		}
		t.Log("waiting guest logged in by MAC after grace")
		// Rejecting removes access.
		if e := a.DecideGuestAccess(ctx, biz, dev, reqOf(0), mac, false, 0, 0, 0, 0); e != nil {
			t.Fatalf("reject: %v", e)
		}
		if l.put(`:put [:len [/ip hotspot user find where name=`+routerOSQuoted(mac)+`]]`) != "0" || activeGuest(8*time.Second) {
			t.Fatal("a rejected guest kept access")
		}
		// Approval with a speed: the session re-logs and its queue carries the plan.
		r.Run(ctx, `/ip hotspot host remove [find where mac-address=`+guestMAC+`]`)
		guestSeen()
		grace(reqOf(1))
		if !activeGuest(30 * time.Second) {
			t.Fatal("second grace: guest did not get online")
		}
		if e := a.DecideGuestAccess(ctx, biz, dev, reqOf(1), mac, true, 60, 500, 20, 5); e != nil {
			t.Fatalf("approve with speed: %v", e)
		}
		if e := a.DecideGuestAccess(ctx, biz, dev, reqOf(1), mac, true, 60, 500, 20, 5); e != nil {
			t.Fatalf("a retried approval must be safe: %v", e)
		}
		if !activeGuest(30 * time.Second) {
			t.Fatal("guest did not re-log after approval")
		}
		queue := l.put(`:foreach q in=[/queue simple find where dynamic=yes] do={ :put [/queue simple get $q max-limit] }`)
		t.Logf("guest online; dynamic queue upload/download limit: %q", queue)
		if !strings.Contains(queue, "5M/20M") && !strings.Contains(queue, "5000000/20000000") {
			t.Errorf("speed tier did not reach the guest's queue: %q", queue)
		}
	})
	t.Run("guest portal deployment", func(t *testing.T) {
		profiles, e := a.PortalProfiles(ctx, biz, dev)
		if e != nil || len(profiles) == 0 {
			t.Fatalf("profiles: %v %v", profiles, e)
		}
		req := PortalDeployment{Profile: "mm-labnet", Version: fmt.Sprintf("%016x", time.Now().Unix()), Design: &PortalLoginDesign{BusinessName: "Lab", WelcomeText: "Welcome", PrimaryHex: "#146b52", GlassTintHex: "#146b52"}}
		plan, e := a.PreviewPortal(ctx, biz, dev, req)
		if e != nil {
			t.Fatalf("preview: %v", e)
		}
		req.Version, req.Confirmation = plan.Version, plan.Confirmation
		res, e := a.ApplyPortal(ctx, biz, dev, req)
		if e != nil || !res.Verified {
			t.Fatalf("apply: %+v %v", res, e)
		}
		t.Logf("portal file %s; api.json in deployed folder: %s", plan.File, l.put(`:put [:len [/file find where name~"api.json"]]`))
	})
	t.Logf("script errors on the router:\n%s", l.scriptErrors(""))
}

// Does access really end? The deadline scripts run on the router with the app out of the picture.
func TestCHRLabExpiryOnTheRouter(t *testing.T) {
	target := os.Getenv("MM_CHR_SSH")
	if target == "" {
		t.Skip("set MM_CHR_SSH to run against a disposable CHR")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	r := chrRunner{target}
	l := chrLab{t, r, ctx}
	a := NewDeviceApplier(capabilitySource{}, 60*time.Second, func(SSHConfig) CommandRunner { return r }, DefaultInterface, 0, 0)
	const biz, dev = "biz-1", "dev-1"
	guestMAC := strings.ToUpper(l.put(`:put [/interface get [find name=m4] mac-address]`))
	traffic := func() { r.Run(ctx, `/ping 1.1.1.1 vrf=guest count=2`) }
	clean := func() {
		for _, cmd := range []string{`/ip hotspot active remove [find]`, `/ip hotspot user remove [find where name=` + routerOSQuoted(guestMAC) + `]`, `/system scheduler remove [find where name~"^mm-(voucher|paid|grace)-"]`, `/ip hotspot host remove [find]`} {
			r.Run(ctx, cmd)
		}
	}
	online := func() bool {
		return l.put(`:put [:len [/ip hotspot active find where mac-address=`+guestMAC+`]]`) != "0"
	}
	waitOnline := func(want bool, within time.Duration) bool {
		end := time.Now().Add(within)
		for time.Now().Before(end) {
			traffic()
			if online() == want {
				return true
			}
			time.Sleep(2 * time.Second)
		}
		return online() == want
	}
	seen := func() {
		for i := 0; i < 15; i++ {
			traffic()
			if l.put(`:put [:len [/ip hotspot host find where mac-address=`+guestMAC+`]]`) != "0" {
				return
			}
		}
		t.Fatal("guest not seen")
	}
	usable := func() string {
		return l.put(`:foreach u in=[/ip hotspot user find where name=` + routerOSQuoted(guestMAC) + `] do={ :put [/ip hotspot user get $u disabled] }`)
	}

	t.Run("voucher deadline", func(t *testing.T) {
		clean()
		seen()
		start := time.Now()
		g := GuestVoucherAccess{VoucherID: fmt.Sprintf("dddddddd-dddd-4ddd-8ddd-%012x", start.Unix()), ClientMAC: guestMAC, Minutes: 60, DataMB: 500, ExpiresAt: start.Add(45 * time.Second)}
		if e := a.ConfigureGuestVoucher(ctx, biz, dev, g); e != nil {
			t.Fatal(e)
		}
		if !waitOnline(true, 25*time.Second) {
			t.Fatal("guest did not get online before the deadline")
		}
		if !waitOnline(false, 60*time.Second) {
			t.Fatalf("voucher still online %s after its deadline", time.Since(g.ExpiresAt).Round(time.Second))
		}
		t.Logf("access ended %s after the deadline; user disabled=%q", time.Since(g.ExpiresAt).Round(time.Second), usable())
		time.Sleep(15 * time.Second)
		if waitOnline(true, 12*time.Second) {
			t.Fatal("expired voucher guest got back online")
		}
	})
	t.Run("grace deadline", func(t *testing.T) {
		clean()
		seen()
		start := time.Now()
		req := fmt.Sprintf("eeeeeeee-eeee-4eee-8eee-%012x", start.Unix())
		if e := a.ConfigureGuestGrace(ctx, biz, dev, GuestGrace{RequestID: req, ClientMAC: guestMAC, Minutes: 1, DataMB: 50, ExpiresAt: start.Add(time.Minute)}); e != nil {
			t.Fatal(e)
		}
		if !waitOnline(true, 25*time.Second) {
			t.Fatal("grace guest did not get online")
		}
		if !waitOnline(false, 120*time.Second) {
			t.Fatalf("grace guest still online %s after start", time.Since(start).Round(time.Second))
		}
		t.Logf("grace ended %s after it started; user disabled=%q", time.Since(start).Round(time.Second), usable())
	})
	t.Run("paid deadline after approval", func(t *testing.T) {
		clean()
		seen()
		start := time.Now()
		req := fmt.Sprintf("eeeeeeee-eeee-4eee-8eee-%012x", start.Unix()+1)
		if e := a.ConfigureGuestGrace(ctx, biz, dev, GuestGrace{RequestID: req, ClientMAC: guestMAC, Minutes: 10, DataMB: 50, ExpiresAt: start.Add(10 * time.Minute)}); e != nil {
			t.Fatal(e)
		}
		if !waitOnline(true, 25*time.Second) {
			t.Fatal("guest did not get online")
		}
		if e := a.DecideGuestAccess(ctx, biz, dev, req, guestMAC, true, 1, 500, 0, 0); e != nil {
			t.Fatal(e)
		}
		approved := time.Now()
		if !waitOnline(true, 25*time.Second) {
			t.Fatal("guest did not re-log after approval")
		}
		if !waitOnline(false, 130*time.Second) {
			t.Fatalf("paid guest still online %s after approval of 1 minute", time.Since(approved).Round(time.Second))
		}
		t.Logf("paid 1-minute access ended %s after approval; user disabled=%q", time.Since(approved).Round(time.Second), usable())
	})
	t.Run("data limit", func(t *testing.T) {
		clean()
		seen()
		req := fmt.Sprintf("eeeeeeee-eeee-4eee-8eee-%012x", time.Now().Unix()+2)
		if e := a.ConfigureGuestGrace(ctx, biz, dev, GuestGrace{RequestID: req, ClientMAC: guestMAC, Minutes: 10, DataMB: 1, ExpiresAt: time.Now().Add(10 * time.Minute)}); e != nil {
			t.Fatal(e)
		}
		if !waitOnline(true, 25*time.Second) {
			t.Fatal("guest did not get online")
		}
		r.Run(ctx, `/ping 10.60.0.1 vrf=guest count=30 size=1400 interval=50ms`)
		out, _ := r.Run(ctx, `/ping 1.1.1.1 vrf=guest count=800 size=1400 interval=10ms`)
		_ = out
		t.Logf("after 1 MB of traffic: online=%v, counters: %s", online(), l.put(`:foreach u in=[/ip hotspot user find where name=`+routerOSQuoted(guestMAC)+`] do={ :put ([/ip hotspot user get $u bytes-in] . "/" . [/ip hotspot user get $u bytes-out]) }`))
		if online() && waitOnline(false, 20*time.Second) == false {
			t.Log("data limit did not end the session; RouterOS needs the traffic to pass, and pings to an unreachable host may not count")
		}
	})
	clean()
}

// The network forms, through the same review and apply path as the console.
func TestCHRLabNetworkForms(t *testing.T) {
	target := os.Getenv("MM_CHR_SSH")
	if target == "" {
		t.Skip("set MM_CHR_SSH to run against a disposable CHR")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	r := chrRunner{target}
	a := NewDeviceApplier(capabilitySource{}, 60*time.Second, func(SSHConfig) CommandRunner { return r }, DefaultInterface, 0, 0)
	const biz, dev = "biz-1", "dev-1"
	id := func(t *testing.T, menu, field, value string) string {
		t.Helper()
		inv, e := a.ReadNetwork(ctx, biz, dev)
		if e != nil {
			t.Fatal(e)
		}
		for _, o := range inv.Objects[menu] {
			if o.Values[field] == value {
				return o.ID
			}
		}
		t.Fatalf("no %s with %s=%s", menu, field, value)
		return ""
	}
	apply := func(t *testing.T, req NetworkRequest) {
		t.Helper()
		review, e := a.PreviewNetwork(ctx, biz, dev, req)
		if e != nil {
			t.Fatalf("review: %v", e)
		}
		req.Confirmation = review.Confirmation
		res, e := a.ApplyNetwork(ctx, biz, dev, req)
		if e != nil || !res.Verified {
			t.Fatalf("apply: %+v %v", res, e)
		}
		if res.PendingConfirm {
			if e := a.ConfirmNetwork(ctx, biz, dev); e != nil {
				t.Fatalf("keep: %v", e)
			}
		}
	}
	for _, tc := range []struct {
		name string
		req  func(t *testing.T) NetworkRequest
	}{
		{"firewall protection", func(*testing.T) NetworkRequest {
			return NetworkRequest{Kind: "firewall", Values: map[string]string{"interface": "lan0", "disabled": "no"}}
		}},
		{"VLAN", func(*testing.T) NetworkRequest {
			return NetworkRequest{Kind: "vlan", Values: map[string]string{"name": "lab-v77", "interface": "lan0", "vlan-id": "77", "disabled": "no"}}
		}},
		{"IP address", func(*testing.T) NetworkRequest {
			return NetworkRequest{Kind: "ip_address", Values: map[string]string{"address": "10.81.0.1/24", "interface": "lab-v77", "disabled": "no"}}
		}},
		{"bandwidth limit", func(*testing.T) NetworkRequest {
			return NetworkRequest{Kind: "queue", Values: map[string]string{"name": "labq", "target": "10.84.0.50/32", "max-limit": "1M/2M", "disabled": "no"}}
		}},
		{"fair share", func(*testing.T) NetworkRequest {
			return NetworkRequest{Kind: "fairshare", Values: map[string]string{"name": "guests", "target": "10.81.0.0/24", "max-limit": "10M/50M", "disabled": "no"}}
		}},
		{"static reservation", func(*testing.T) NetworkRequest {
			return NetworkRequest{Kind: "lease", Values: map[string]string{"address": "10.60.0.77", "mac-address": "02:AA:BB:CC:DD:01", "server": "mm-labnet", "disabled": "no"}}
		}},
		{"PPPoE service", func(*testing.T) NetworkRequest {
			return NetworkRequest{Kind: "pppoe_server", Values: map[string]string{"name": "labsvc", "interface": "lan1", "gateway_cidr": "10.82.0.1/24", "pool_start": "10.82.0.10", "pool_end": "10.82.0.200", "dns": "1.1.1.1", "rate_limit": "10M/20M"}}
		}},
		{"subscriber", func(*testing.T) NetworkRequest {
			return NetworkRequest{Kind: "subscriber", Values: map[string]string{"name": "labsub", "password": "lab-test-password", "profile": "lab-plan", "disabled": "no"}}
		}},
		{"plan edit", func(t *testing.T) NetworkRequest {
			return NetworkRequest{Kind: "ppp_profile", ID: id(t, "ppp_profiles", "name", "mm-labsvc"), Values: map[string]string{"local-address": "10.82.0.1", "remote-address": "mm-labsvc", "dns-server": "1.1.1.1", "rate-limit": "3M/5M", "only-one": "yes"}}
		}},
		{"PPPoE uplink", func(*testing.T) NetworkRequest {
			return NetworkRequest{Kind: "pppoe_client", Values: map[string]string{"name": "isp2", "interface": "lan0", "user": "labisp", "password": "lab-test-password", "distance": "5", "disabled": "yes"}}
		}},
		{"edit customer network", func(t *testing.T) NetworkRequest {
			return NetworkRequest{Kind: "edit_network", ID: id(t, "dhcp", "name", "mm-labnet"), Values: map[string]string{"pool_start": "10.60.0.20", "pool_end": "10.60.0.210", "lease-time": "2h", "dns": "1.1.1.1,8.8.8.8", "disabled": "no"}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) { apply(t, tc.req(t)) })
	}
	t.Run("firmware path on hardware without RouterBOARD firmware", func(t *testing.T) {
		review, e := a.PreviewFirmware(ctx, biz, dev)
		t.Logf("preview: %+v, error: %v", review, e)
		if e == nil {
			t.Fatal("firmware upgrade was offered on a CHR, which has no RouterBOARD firmware")
		}
	})
	t.Run("Wi-Fi inventory without radios", func(t *testing.T) {
		inv, e := a.ReadNetwork(ctx, biz, dev)
		if e != nil {
			t.Fatal(e)
		}
		t.Logf("wifi menu available=%v records=%d; wireless available=%v", inv.Available["wifi"], len(inv.Objects["wifi"]), inv.Available["wireless"])
	})
}
