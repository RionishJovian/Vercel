package routeros

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// fakeRouterSSH is a minimal SSH server that runs "exec" requests and echoes the
// command, counting the connections and commands it sees.
type fakeRouterSSH struct {
	addr        string
	connections atomic.Int32
	commands    atomic.Int32
	// refuseSecondSession makes a connection refuse any session after its first one,
	// like a router that only serves one channel per connection.
	refuseSecondSession bool
	// dropAfterCommand closes the connection after answering, like a router that
	// closed an idle connection.
	dropAfterCommand bool
	hold             chan struct{} // when set, commands wait for it to be closed
}

func startFakeRouterSSH(t *testing.T, f *fakeRouterSSH) {
	t.Helper()
	_, private, _ := ed25519.GenerateKey(rand.Reader)
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &ssh.ServerConfig{PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) { return nil, nil }}
	cfg.AddHostKey(signer)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	f.addr = l.Addr().String()
	go func() {
		for {
			raw, err := l.Accept()
			if err != nil {
				return
			}
			f.connections.Add(1)
			go func() {
				conn, chans, reqs, err := ssh.NewServerConn(raw, cfg)
				if err != nil {
					raw.Close()
					return
				}
				go ssh.DiscardRequests(reqs)
				sessions := 0
				var wg sync.WaitGroup
				for ch := range chans {
					sessions++
					if f.refuseSecondSession && sessions > 1 {
						_ = ch.Reject(ssh.ResourceShortage, "one session only")
						continue
					}
					channel, requests, err := ch.Accept()
					if err != nil {
						continue
					}
					wg.Add(1)
					go func() {
						defer wg.Done()
						for req := range requests {
							if req.Type != "exec" {
								_ = req.Reply(false, nil)
								continue
							}
							var payload struct{ Command string }
							_ = ssh.Unmarshal(req.Payload, &payload)
							_ = req.Reply(true, nil)
							f.commands.Add(1)
							if f.hold != nil {
								<-f.hold
							}
							_, _ = channel.Write([]byte("ok:" + payload.Command))
							_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
							channel.Close()
							if f.dropAfterCommand {
								conn.Close()
							}
							return
						}
					}()
				}
				wg.Wait()
				conn.Close()
			}()
		}
	}()
}

func reusing(f *fakeRouterSSH) *SSHTransport {
	return NewReusingSSHTransport(SSHConfig{Addr: f.addr, User: "admin", Password: "x", InsecureIgnoreHostKey: true, Timeout: 3 * time.Second})
}

func TestReusingTransportRunsManyCommandsOverOneConnection(t *testing.T) {
	f := &fakeRouterSSH{}
	startFakeRouterSSH(t, f)
	tr := reusing(f)
	defer tr.Close()
	for i := 0; i < 8; i++ {
		out, err := tr.Run(context.Background(), "/system resource print")
		if err != nil || out != "ok:/system resource print" {
			t.Fatalf("%q %v", out, err)
		}
	}
	if f.connections.Load() != 1 || f.commands.Load() != 8 {
		t.Fatalf("expected 1 connection for 8 commands, got %d connections, %d commands", f.connections.Load(), f.commands.Load())
	}
}

func TestPlainTransportStillConnectsPerCommand(t *testing.T) {
	f := &fakeRouterSSH{}
	startFakeRouterSSH(t, f)
	tr := NewSSHTransport(SSHConfig{Addr: f.addr, User: "admin", Password: "x", InsecureIgnoreHostKey: true, Timeout: 3 * time.Second})
	for i := 0; i < 3; i++ {
		if _, err := tr.Run(context.Background(), "x"); err != nil {
			t.Fatal(err)
		}
	}
	if f.connections.Load() != 3 {
		t.Fatalf("the plain transport must stay one connection per command, saw %d", f.connections.Load())
	}
}

func TestReusingTransportFallsBackWhenTheRouterServesOneSessionPerConnection(t *testing.T) {
	f := &fakeRouterSSH{refuseSecondSession: true}
	startFakeRouterSSH(t, f)
	tr := reusing(f)
	defer tr.Close()
	for i := 0; i < 4; i++ {
		if out, err := tr.Run(context.Background(), "cmd"); err != nil || out != "ok:cmd" {
			t.Fatalf("command %d: %q %v", i, out, err)
		}
	}
	if f.commands.Load() != 4 {
		t.Fatalf("every command must run exactly once, ran %d", f.commands.Load())
	}
}

func TestReusingTransportRecoversFromAConnectionTheRouterClosed(t *testing.T) {
	f := &fakeRouterSSH{dropAfterCommand: true}
	startFakeRouterSSH(t, f)
	tr := reusing(f)
	defer tr.Close()
	for i := 0; i < 3; i++ {
		time.Sleep(50 * time.Millisecond) // let the close arrive
		if out, err := tr.Run(context.Background(), "cmd"); err != nil || out != "ok:cmd" {
			t.Fatalf("command %d: %q %v", i, out, err)
		}
	}
	if f.commands.Load() != 3 {
		t.Fatalf("each command must run exactly once, ran %d", f.commands.Load())
	}
}

func TestReusingTransportBoundsAHungCommandAndReconnectsAfterwards(t *testing.T) {
	hold := make(chan struct{})
	f := &fakeRouterSSH{hold: hold}
	startFakeRouterSSH(t, f)
	tr := NewReusingSSHTransport(SSHConfig{Addr: f.addr, User: "admin", Password: "x", InsecureIgnoreHostKey: true, Timeout: 300 * time.Millisecond})
	defer tr.Close()
	start := time.Now()
	if _, err := tr.Run(context.Background(), "hang"); err == nil {
		t.Fatal("a hung command must fail")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("a hung command must be bounded by the timeout, took %s", time.Since(start))
	}
	close(hold)
	if out, err := tr.Run(context.Background(), "again"); err != nil || !strings.HasSuffix(out, "again") {
		t.Fatalf("must reconnect after a timeout: %q %v", out, err)
	}
}

func TestReusingTransportIdleConnectionClosesAndConcurrentCommandsAreSafe(t *testing.T) {
	f := &fakeRouterSSH{}
	startFakeRouterSSH(t, f)
	tr := reusing(f)
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := tr.Run(context.Background(), "parallel"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if f.connections.Load() != 1 {
		t.Fatalf("parallel commands should share one connection, saw %d", f.connections.Load())
	}
	tr.mu.Lock()
	idle := tr.idle
	tr.mu.Unlock()
	if idle == nil {
		t.Fatal("an idle timer must be armed after use")
	}
	tr.Close()
	tr.mu.Lock()
	closed := tr.client == nil
	tr.mu.Unlock()
	if !closed {
		t.Fatal("Close must drop the connection")
	}
}

// A concurrency-safe runner that records how many commands ran at the same time.
type timedRunner struct {
	*networkRunner
	mu      sync.Mutex
	running int
	peak    int
	delay   time.Duration
}

func (r *timedRunner) ConcurrentSafe() bool { return true }
func (r *timedRunner) Run(ctx context.Context, cmd string) (string, error) {
	r.mu.Lock()
	r.running++
	if r.running > r.peak {
		r.peak = r.running
	}
	r.mu.Unlock()
	time.Sleep(r.delay)
	r.mu.Lock()
	r.running--
	out, err := r.networkRunner.Run(ctx, cmd)
	r.mu.Unlock()
	return out, err
}

func TestInventoryReadRunsSeveralCommandsAtOnceWhenTheRunnerAllows(t *testing.T) {
	slow := &timedRunner{networkRunner: networkFixture(), delay: 40 * time.Millisecond}
	serial := quickConfigApplier(slow)
	start := time.Now()
	inv, err := serial.ReadNetwork(context.Background(), "biz-1", "dev-1")
	took := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if slow.peak < 2 || slow.peak > networkReadWorkers {
		t.Fatalf("expected between 2 and %d commands at once, peak was %d", networkReadWorkers, slow.peak)
	}
	if len(inv.Available) < 20 || !inv.Available["addresses"] || len(inv.Objects["addresses"]) == 0 {
		t.Fatalf("the inventory must be complete: %d menus", len(inv.Available))
	}
	// 22+ commands at 40ms each would be ~0.9s one at a time.
	if took > 600*time.Millisecond {
		t.Fatalf("parallel read took %s", took)
	}
}

func TestInventoryReadStaysSerialForARunnerThatIsNotConcurrencySafe(t *testing.T) {
	r := networkFixture()
	if _, err := quickConfigApplier(r).ReadNetwork(context.Background(), "biz-1", "dev-1"); err != nil {
		t.Fatal(err)
	}
}
