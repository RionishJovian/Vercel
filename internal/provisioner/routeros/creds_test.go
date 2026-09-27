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
)

// fakeSource is a TargetSource backed by a map. It records lookups so a test
// can assert WHICH peer was resolved, not merely that resolution happened.
type fakeSource struct {
	mu      sync.Mutex
	targets map[string]*DeviceTarget // key: businessID + "/" + deviceID
	errs    map[string]error
	lookups []string
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

func (r *addrEchoRunner) Run(_ context.Context, _ string) (string, error) {
	if r.err != nil {
		return "", r.err
	}
	// Mimic RouterOS "print value-list": COLON-separated key/value lines.
	// (My first attempt used "=", which parseValueList does not split on — the
	// fixture was wrong, not the parser.)
	return fmt.Sprintf("version: 7.%s\nchannel: stable\n", r.addr), nil
}

// recordingFactory records every config handed to it.
type recordingFactory struct {
	mu      sync.Mutex
	configs []SSHConfig
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
	return &DeviceTarget{DeviceID: id, Addr: addr, User: user, Secret: pw, Kind: "password"}
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

	vA, _, err := p.ObserveVersion(context.Background(), "biz-1", "dev-A")
	if err != nil {
		t.Fatalf("dev-A: %v", err)
	}
	vB, _, err := p.ObserveVersion(context.Background(), "biz-1", "dev-B")
	if err != nil {
		t.Fatalf("dev-B: %v", err)
	}

	if !strings.Contains(vA, "10.0.0.1:22") {
		t.Errorf("dev-A read through the wrong transport: %q", vA)
	}
	if !strings.Contains(vB, "10.0.0.2:22") {
		t.Errorf("dev-B read through the wrong transport: %q", vB)
	}
	if vA == vB {
		t.Fatal("two devices produced identical results — the transport is being shared")
	}
	if got := rf.count(); got != 2 {
		t.Errorf("expected one transport per peer (2), built %d", got)
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

	a, _, err := p.ObserveVersion(context.Background(), "biz-1", "shared-id")
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := p.ObserveVersion(context.Background(), "biz-2", "shared-id")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(a, "10.10.0.1") || !strings.Contains(b, "10.20.0.1") {
		t.Fatalf("tenant scoping lost: biz-1=%q biz-2=%q", a, b)
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
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("dev-%02d", i)
			v, _, err := p.ObserveVersion(context.Background(), "biz-1", id)
			got[i], errs[i] = v, err
		}(i)
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("peer %d: %v", i, errs[i])
		}
		id := fmt.Sprintf("dev-%02d", i)
		if !strings.Contains(got[i], want[id]) {
			t.Errorf("peer %s read through %q, want its own %q", id, got[i], want[id])
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

	key := &DeviceTarget{DeviceID: "d", Addr: "10.0.0.1:22", User: "admin", Secret: "PEM", Kind: "key"}
	kc, err := SSHConfigFromTarget(key, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if string(kc.PrivateKeyPEM) != "PEM" || kc.Password != "" {
		t.Errorf("key target mapped wrong: %+v", kc)
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
