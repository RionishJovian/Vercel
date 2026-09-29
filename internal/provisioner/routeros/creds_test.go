package routeros

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"mega-magic/internal/provisioner"
)

// fakeSource is a TargetSource backed by a map. It records lookups so a test
// can assert WHICH peer was resolved, not merely that resolution happened.
type fakeSource struct {
	mu      sync.Mutex
	targets map[string]*DeviceTarget // key: businessID + "/" + deviceID
	errs    map[string]error
	lookups []string
}

type rotatingTargetSource struct {
	mu      sync.Mutex
	targets []*DeviceTarget
	calls   int
}

func (s *rotatingTargetSource) TargetForDevice(context.Context, string, string) (*DeviceTarget, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.targets) == 0 {
		return nil, ErrNoTarget
	}
	index := s.calls
	if index >= len(s.targets) {
		index = len(s.targets) - 1
	}
	s.calls++
	return s.targets[index], nil
}

func (s *rotatingTargetSource) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func newFakeSource() *fakeSource {
	return &fakeSource{targets: map[string]*DeviceTarget{}, errs: map[string]error{}}
}

func (f *fakeSource) put(businessID, deviceID string, t *DeviceTarget) {
	f.targets[businessID+"/"+deviceID] = t
}

func (f *fakeSource) fail(businessID, deviceID string, err error) {
	f.errs[businessID+"/"+deviceID] = err
}

func (f *fakeSource) TargetForDevice(_ context.Context, businessID, deviceID string) (*DeviceTarget, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := businessID + "/" + deviceID
	f.lookups = append(f.lookups, key)
	if err, ok := f.errs[key]; ok {
		return nil, err
	}
	if t, ok := f.targets[key]; ok {
		return t, nil
	}
	return nil, ErrNoTarget
}

func (f *fakeSource) lookupCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.lookups)
}

// addrEchoRunner answers with a value derived from the config it was built with,
// so "which transport served this read" is observable. If a prober ever reuses
// one transport for a different peer, the returned version shows the wrong
// address and the isolation tests fail. (Named addrEchoRunner, not fakeRunner,
// because transport_test.go already defines a fakeRunner in this package.)
type addrEchoRunner struct {
	addr string
	err  error
}

// Run returns output shaped EXACTLY like real RouterOS `/system/resource print`
// (captured from RouterOS 7.15.3). Two details matter and both were previously
// wrong in this fixture:
//
//  1. Real output is COLON-separated, not "key=value".
//  2. There is NO `channel:` field. The channel is the parenthesised suffix of
//     `version`, so the per-peer marker has to ride there — which also proves
//     the channel is extracted from the right place.
func (r *addrEchoRunner) Run(_ context.Context, _ string) (string, error) {
	if r.err != nil {
		return "", r.err
	}
	return fmt.Sprintf(`                   uptime: 9m1s
                  version: 7.15.3 (stable-%s)
               build-time: 2024-07-24 10:39:01
         factory-software: 7.1
              free-memory: 307.9MiB
            architecture-name: x86_64
               board-name: CHR QEMU Standard PC
                 platform: MikroTik
`, r.addr), nil
}

// recordingFactory records every config handed to it.
type recordingFactory struct {
	mu      sync.Mutex
	configs []SSHConfig
}

type recordedCommand struct {
	addr string
	cmd  string
}

type commandRecordingFactory struct {
	mu    sync.Mutex
	calls []recordedCommand
}

type commandRunnerFunc func(context.Context, string) (string, error)

func (f commandRunnerFunc) Run(ctx context.Context, cmd string) (string, error) {
	return f(ctx, cmd)
}

func (rf *commandRecordingFactory) build(cfg SSHConfig) CommandRunner {
	return commandRunnerFunc(func(_ context.Context, cmd string) (string, error) {
		rf.mu.Lock()
		rf.calls = append(rf.calls, recordedCommand{addr: cfg.Addr, cmd: cmd})
		rf.mu.Unlock()
		return "", nil
	})
}

func (rf *commandRecordingFactory) snapshot() []recordedCommand {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	return append([]recordedCommand(nil), rf.calls...)
}

func (rf *recordingFactory) build(cfg SSHConfig) CommandRunner {
	rf.mu.Lock()
	rf.configs = append(rf.configs, cfg)
	rf.mu.Unlock()
	return &addrEchoRunner{addr: cfg.Addr}
}

func (rf *recordingFactory) count() int {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	return len(rf.configs)
}

func (rf *recordingFactory) addrs() []string {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	out := make([]string, len(rf.configs))
	for i, c := range rf.configs {
		out[i] = c.Addr
	}
	return out
}

func pwTarget(id, addr, user, pw string) *DeviceTarget {
	return &DeviceTarget{DeviceID: id, Addr: addr, User: user, Secret: pw, Kind: "password", Fingerprint: "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
}

// ---------------------------------------------------------------------------
// THE REGRESSION THIS WORK EXISTS FOR
// ---------------------------------------------------------------------------

// Each peer must be read through ITS OWN address. Under the previous design a
// single transport was built from config and reused for every peerID, so both
// calls below would have returned the same version.
func TestObserveVersionRoutesPerPeer(t *testing.T) {
	src := newFakeSource()
	src.put("biz-1", "dev-A", pwTarget("dev-A", "10.0.0.1:22", "admin", "pwA"))
	src.put("biz-1", "dev-B", pwTarget("dev-B", "10.0.0.2:22", "admin", "pwB"))

	rf := &recordingFactory{}
	p := NewDeviceProber(src, time.Second, rf.build)

	vA, cA, err := p.ObserveVersion(context.Background(), "biz-1", "dev-A")
	if err != nil {
		t.Fatalf("dev-A: %v", err)
	}
	vB, cB, err := p.ObserveVersion(context.Background(), "biz-1", "dev-B")
	if err != nil {
		t.Fatalf("dev-B: %v", err)
	}

	// The version is identical on both (same image); the per-peer marker rides
	// in the channel suffix, so that is what proves WHICH transport answered.
	// See addrEchoRunner.
	if !strings.Contains(cA, "10.0.0.1:22") {
		t.Errorf("dev-A read through the wrong transport: version=%q channel=%q", vA, cA)
	}
	if !strings.Contains(cB, "10.0.0.2:22") {
		t.Errorf("dev-B read through the wrong transport: version=%q channel=%q", vB, cB)
	}
	if cA == cB {
		t.Fatal("two devices produced identical results — the transport is being shared")
	}
	// The version must be split cleanly away from the channel suffix, so the
	// classifier never sees "7.15.3 (stable-10.0.0.1:22)".
	if vA != "7.15.3" || vB != "7.15.3" {
		t.Errorf("version not split from the channel suffix: %q / %q", vA, vB)
	}
	if got := rf.count(); got != 2 {
		t.Errorf("expected one transport per peer (2), built %d", got)
	}
}

func TestDeviceApplierRoutesWritesPerTenantAndDevice(t *testing.T) {
	src := newFakeSource()
	src.put("biz-1", "dev-A", pwTarget("dev-A", "10.10.0.1:22", "admin", "pwA"))
	src.put("biz-1", "dev-B", pwTarget("dev-B", "10.10.0.2:22", "admin", "pwB"))
	// Same device id in another tenant must resolve to that tenant's own target.
	src.put("biz-2", "dev-A", pwTarget("dev-A", "10.20.0.1:22", "admin", "pwC"))

	rf := &commandRecordingFactory{}
	applier := NewDeviceApplier(src, time.Second, rf.build, "wg-magichub", 0, 0)
	change := []provisioner.Change{{Action: provisioner.ActionEnsureKeepalive}}

	for _, target := range []struct {
		businessID string
		deviceID   string
	}{
		{"biz-1", "dev-A"},
		{"biz-1", "dev-B"},
		{"biz-2", "dev-A"},
	} {
		if err := applier.Apply(context.Background(), target.businessID, target.deviceID, change); err != nil {
			t.Fatalf("apply %s/%s: %v", target.businessID, target.deviceID, err)
		}
	}

	calls := rf.snapshot()
	if len(calls) != 3 {
		t.Fatalf("command count = %d, want 3: %+v", len(calls), calls)
	}
	wantAddrs := []string{"10.10.0.1:22", "10.10.0.2:22", "10.20.0.1:22"}
	for i, want := range wantAddrs {
		if calls[i].addr != want {
			t.Fatalf("write %d went to %q, want %q", i, calls[i].addr, want)
		}
		if !strings.Contains(calls[i].cmd, "persistent-keepalive=25s") {
			t.Fatalf("write %d used unexpected command %q", i, calls[i].cmd)
		}
	}

	before := len(calls)
	if err := applier.Apply(context.Background(), "biz-2", "dev-B", change); !errors.Is(err, ErrNoTarget) {
		t.Fatalf("unresolved target error = %v, want ErrNoTarget", err)
	}
	if after := len(rf.snapshot()); after != before {
		t.Fatalf("unresolved target issued a command: before=%d after=%d", before, after)
	}
}

func TestApplyServicePinsOneResolvedTargetForWholeOperation(t *testing.T) {
	// The source deliberately changes from router A to router B after its first
	// lookup. A vulnerable implementation re-resolves between snapshot/apply/
	// verify/rollback and emits commands to both addresses.
	src := &rotatingTargetSource{targets: []*DeviceTarget{
		pwTarget("dev", "10.77.0.10:22", "admin", "pwA"),
		pwTarget("dev", "10.77.0.20:22", "admin", "pwB"),
	}}
	rf := &commandRecordingFactory{}
	applier := NewDeviceApplier(src, time.Second, rf.build, "wg-magichub", 0, 0).
		WithControlConfig("", time.Minute)
	svc := provisioner.NewApplyService(applier, NewDeviceProber(src, time.Second, rf.build))

	res, err := svc.ApplyFixes(context.Background(), provisioner.ApplyRequest{
		BusinessID: "biz",
		PeerID:     "dev",
		Changes:    []provisioner.Change{{Action: provisioner.ActionEnsureKeepalive}},
	})
	if err != nil {
		t.Fatalf("ApplyFixes: %v", err)
	}
	if !res.SnapshotCaptured || !res.RolledBack {
		t.Fatalf("expected snapshot and rollback after empty fake verification: %+v", res)
	}
	if got := src.callCount(); got != 1 {
		t.Fatalf("target resolved %d times, want exactly once for the whole operation", got)
	}
	for i, call := range rf.snapshot() {
		if call.addr != "10.77.0.10:22" {
			t.Fatalf("command %d drifted to %s: %q", i, call.addr, call.cmd)
		}
	}
}

// The same device id in two tenants must resolve to two different devices.
// Without business scoping, a fleet sweep could read across tenants.
func TestObserveVersionIsTenantScoped(t *testing.T) {
	src := newFakeSource()
	// Deliberately the SAME device id in both tenants.
	src.put("biz-1", "shared-id", pwTarget("shared-id", "10.10.0.1:22", "u", "p1"))
	src.put("biz-2", "shared-id", pwTarget("shared-id", "10.20.0.1:22", "u", "p2"))

	rf := &recordingFactory{}
	p := NewDeviceProber(src, time.Second, rf.build)

	a, ca, err := p.ObserveVersion(context.Background(), "biz-1", "shared-id")
	if err != nil {
		t.Fatal(err)
	}
	b, cb, err := p.ObserveVersion(context.Background(), "biz-2", "shared-id")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ca, "10.10.0.1") || !strings.Contains(cb, "10.20.0.1") {
		t.Fatalf("tenant scoping lost: biz-1=%q/%q biz-2=%q/%q", a, ca, b, cb)
	}
}

// An unresolvable peer must FAIL. It must not fall back to any other transport,
// because a fallback is exactly how one router's version gets written onto
// another device's row.
func TestUnresolvablePeerDoesNotFallBack(t *testing.T) {
	src := newFakeSource()
	src.put("biz-1", "dev-A", pwTarget("dev-A", "10.0.0.1:22", "admin", "pwA"))
	// dev-B is NOT configured.
	src.fail("biz-1", "dev-B", ErrNoTarget)

	rf := &recordingFactory{}
	p := NewDeviceProber(src, time.Second, rf.build)

	// A working peer first — proves the prober is otherwise functional, so the
	// failure below is attributable to the missing target, not a broken prober.
	if _, _, err := p.ObserveVersion(context.Background(), "biz-1", "dev-A"); err != nil {
		t.Fatalf("dev-A should work: %v", err)
	}

	_, _, err := p.ObserveVersion(context.Background(), "biz-1", "dev-B")
	if !errors.Is(err, ErrNoTarget) {
		t.Fatalf("unconfigured peer: got %v, want ErrNoTarget", err)
	}
	// The decisive assertion: no transport was built for the failing peer.
	if got := len(rf.addrs()); got != 1 {
		t.Fatalf("a transport was built for an unresolvable peer (addrs=%v)", rf.addrs())
	}
	if addr := rf.addrs()[0]; !strings.Contains(addr, "10.0.0.1") {
		t.Fatalf("only dev-A's own address may ever be dialled, built %q", addr)
	}
}

// "Never provisioned" and "unreachable" must be distinguishable, or an outage
// silently reads as an endless rollout.
func TestNoTargetIsDistinctFromTransportFailure(t *testing.T) {
	src := newFakeSource()
	src.put("biz-1", "unreachable", pwTarget("unreachable", "10.0.0.9:22", "u", "p"))
	src.put("biz-1", "unprovisioned", nil) // present but empty -> incomplete

	p := NewDeviceProber(src, time.Second, func(cfg SSHConfig) CommandRunner {
		return &addrEchoRunner{addr: cfg.Addr, err: errors.New("connection refused")}
	})

	_, _, errUnreachable := p.ObserveVersion(context.Background(), "biz-1", "unreachable")
	if errUnreachable == nil {
		t.Fatal("expected an error for an unreachable device")
	}
	if errors.Is(errUnreachable, ErrNoTarget) {
		t.Error("a transport failure was reported as 'no target configured'")
	}
	if !strings.Contains(errUnreachable.Error(), "connection refused") {
		t.Errorf("the underlying cause was lost: %v", errUnreachable)
	}

	_, _, errUnprovisioned := p.ObserveVersion(context.Background(), "biz-1", "unprovisioned")
	if errUnprovisioned == nil {
		t.Fatal("expected an error for an unprovisioned device")
	}
	if !errors.Is(errUnprovisioned, ErrIncompleteTarget) && !errors.Is(errUnprovisioned, ErrNoTarget) {
		t.Errorf("unprovisioned device reported as %v", errUnprovisioned)
	}
}

// A sweep evaluates peers concurrently. Each must still get its own transport —
// a shared one would race and cross-contaminate.
func TestConcurrentPeersStayIsolated(t *testing.T) {
	const n = 12
	src := newFakeSource()
	want := map[string]string{}
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("dev-%02d", i)
		addr := fmt.Sprintf("10.0.0.%d:22", i+1)
		src.put("biz-1", id, pwTarget(id, addr, "u", "p"))
		want[id] = addr
	}

	rf := &recordingFactory{}
	p := NewDeviceProber(src, time.Second, rf.build)

	var wg sync.WaitGroup
	got := make([]string, n)
	chans := make([]string, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("dev-%02d", i)
			v, c, err := p.ObserveVersion(context.Background(), "biz-1", id)
			got[i], chans[i], errs[i] = v, c, err
		}(i)
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("peer %d: %v", i, errs[i])
		}
		id := fmt.Sprintf("dev-%02d", i)
		if !strings.Contains(chans[i], want[id]) {
			t.Errorf("peer %s read through %q (version %q), want its own %q", id, chans[i], got[i], want[id])
		}
	}
}

// ---------------------------------------------------------------------------
// Fail-closed edges
// ---------------------------------------------------------------------------

func TestEmptyPeerIDRefused(t *testing.T) {
	p := NewDeviceProber(newFakeSource(), time.Second, (&recordingFactory{}).build)
	if _, _, err := p.ObserveVersion(context.Background(), "biz-1", ""); !errors.Is(err, ErrNoTarget) {
		t.Fatalf("empty peer id: got %v, want ErrNoTarget", err)
	}
}

func TestNilSourceRefused(t *testing.T) {
	p := NewDeviceProber(nil, time.Second, (&recordingFactory{}).build)
	if _, _, err := p.ObserveVersion(context.Background(), "biz-1", "dev"); err == nil {
		t.Fatal("a prober with no target source returned success")
	}
}

func TestNilRunnerRefused(t *testing.T) {
	src := newFakeSource()
	src.put("biz-1", "dev", pwTarget("dev", "10.0.0.1:22", "u", "p"))
	p := NewDeviceProber(src, time.Second, func(SSHConfig) CommandRunner { return nil })
	if _, _, err := p.ObserveVersion(context.Background(), "biz-1", "dev"); err == nil {
		t.Fatal("a nil runner produced success")
	}
}

func TestSSHConfigFromTarget(t *testing.T) {
	pw, err := SSHConfigFromTarget(pwTarget("d", "10.0.0.1:22", "admin", "s3cret"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if pw.Password != "s3cret" || pw.User != "admin" || pw.Addr != "10.0.0.1:22" {
		t.Errorf("password target mapped wrong: %+v", pw)
	}
	if len(pw.PrivateKeyPEM) != 0 {
		t.Error("password target carried a key")
	}

	if pw.HostKeyFingerprint != "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" || pw.InsecureIgnoreHostKey {
		t.Errorf("password target did not enforce its pinned host key: %+v", pw)
	}

	key := &DeviceTarget{DeviceID: "d", Addr: "10.0.0.1:22", User: "admin", Secret: "PEM", Kind: "key", Fingerprint: "SHA256:BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBA"}
	kc, err := SSHConfigFromTarget(key, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if string(kc.PrivateKeyPEM) != "PEM" || kc.Password != "" {
		t.Errorf("key target mapped wrong: %+v", kc)
	}
	if kc.HostKeyFingerprint != "SHA256:BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBA" || kc.InsecureIgnoreHostKey {
		t.Errorf("key target did not enforce its pinned host key: %+v", kc)
	}

	unpinned := pwTarget("d", "10.0.0.1:22", "admin", "s3cret")
	unpinned.Fingerprint = ""
	if _, err := SSHConfigFromTarget(unpinned, time.Second); !errors.Is(err, ErrHostKeyUnpinned) {
		t.Fatalf("unpinned target error = %v, want ErrHostKeyUnpinned", err)
	}
	for _, invalid := range []string{
		"SHA256:abc",
		"SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"MD5:00:11:22:33",
		"not-a-fingerprint",
	} {
		unpinned.Fingerprint = invalid
		if _, err := SSHConfigFromTarget(unpinned, time.Second); !errors.Is(err, ErrHostKeyUnpinned) {
			t.Errorf("invalid fingerprint %q: error = %v, want ErrHostKeyUnpinned", invalid, err)
		}
	}

	// Incomplete / unknown kind / nil must all be refused, never defaulted.
	if _, err := SSHConfigFromTarget(nil, time.Second); !errors.Is(err, ErrNoTarget) {
		t.Errorf("nil target: %v", err)
	}
	for name, bad := range map[string]*DeviceTarget{
		"no addr":   {DeviceID: "d", User: "u", Secret: "s", Kind: "password"},
		"no user":   {DeviceID: "d", Addr: "a:22", Secret: "s", Kind: "password"},
		"no secret": {DeviceID: "d", Addr: "a:22", User: "u", Kind: "password"},
		"bad kind":  {DeviceID: "d", Addr: "a:22", User: "u", Secret: "s", Kind: "telnet"},
	} {
		if _, err := SSHConfigFromTarget(bad, time.Second); err == nil {
			t.Errorf("%s: accepted an incomplete target", name)
		}
	}
}

func TestValidateHostKeyFingerprint(t *testing.T) {
	valid := "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	if err := ValidateHostKeyFingerprint(valid); err != nil {
		t.Fatalf("valid fingerprint rejected: %v", err)
	}
	if err := ValidateHostKeyFingerprint("  " + valid + "\n"); err != nil {
		t.Fatalf("trimmed valid fingerprint rejected: %v", err)
	}
}

func TestJoinHostPort(t *testing.T) {
	cases := []struct {
		host string
		port int
		want string
	}{
		{"10.0.0.1", 22, "10.0.0.1:22"},
		{"10.0.0.1", 2222, "10.0.0.1:2222"},
		{"10.0.0.1", 0, "10.0.0.1:22"},         // default port
		{"10.0.0.1:2222", 22, "10.0.0.1:2222"}, // already has a port
		{"router.local", 22, "router.local:22"},
		{"10.0.0.1/32", 22, "10.0.0.1:22"},     // inet cast carries a mask
		{"10.0.0.1/24", 2222, "10.0.0.1:2222"}, // non-host mask too
		{"  10.0.0.1/32  ", 22, "10.0.0.1:22"}, // padded
		{"/32", 22, ""},                        // mask with no address -> empty, not ":22"
		{"", 22, ""},                           // empty stays empty -> ErrNoTarget, never ":22"
		{"  ", 22, ""},
	}
	for _, c := range cases {
		if got := JoinHostPort(c.host, c.port); got != c.want {
			t.Errorf("JoinHostPort(%q, %d) = %q, want %q", c.host, c.port, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Leak guards
// ---------------------------------------------------------------------------

// A resolved target carries a DECRYPTED router password. Every ordinary way a
// struct escapes into a log line must therefore redact it. This test is the
// regression guard for that: it fails the moment someone adds a naive Stringer
// or removes MarshalJSON.
func TestDeviceTargetNeverLeaksSecret(t *testing.T) {
	const secret = "sup3r-s3cret-router-pw"
	target := &DeviceTarget{
		DeviceID: "dev-1",
		Addr:     "10.0.0.1:22",
		User:     "admin",
		Secret:   secret,
		Kind:     "password",
	}

	// %v / %s -> String()
	if got := fmt.Sprintf("%v", *target); strings.Contains(got, secret) {
		t.Errorf("%%v leaked the secret: %s", got)
	}
	if got := fmt.Sprintf("%s", *target); strings.Contains(got, secret) {
		t.Errorf("%%s leaked the secret: %s", got)
	}
	// %+v -> String() as well
	if got := fmt.Sprintf("%+v", *target); strings.Contains(got, secret) {
		t.Errorf("%%+v leaked the secret: %s", got)
	}
	// %#v -> GoString()
	if got := fmt.Sprintf("%#v", *target); strings.Contains(got, secret) {
		t.Errorf("%%#v leaked the secret: %s", got)
	}
	// json.Marshal -> MarshalJSON()
	blob, err := json.Marshal(*target)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(blob), secret) {
		t.Errorf("json.Marshal leaked the secret: %s", blob)
	}
	// ...but the useful non-secret facts must SURVIVE, or the redaction has
	// destroyed the diagnostic value that makes it acceptable.
	if !strings.Contains(string(blob), "10.0.0.1:22") || !strings.Contains(string(blob), "admin") {
		t.Errorf("redaction dropped legitimate fields: %s", blob)
	}
	if !strings.Contains(string(blob), "true") {
		t.Errorf("redaction lost has_secret: %s", blob)
	}
	// A pointer formatting must also be covered.
	if got := fmt.Sprintf("%v", target); strings.Contains(got, secret) {
		t.Errorf("pointer %%v leaked the secret: %s", got)
	}
}
