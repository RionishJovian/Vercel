package routeros

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// CommandRunner executes one RouterOS console command and returns its combined
// output. It is the seam that makes the real Applier/Prober unit-testable with
// a fake runner (no SSH).
type CommandRunner interface {
	Run(ctx context.Context, cmd string) (string, error)
}

// SSHConfig configures the real SSH transport. Credentials are always injected
// (config/tests); nothing is hardcoded.
type SSHConfig struct {
	Addr                  string
	User                  string
	Password              string
	PrivateKeyPEM         []byte
	HostKeyFingerprint    string
	InsecureIgnoreHostKey bool
	Timeout               time.Duration
}

// SSHTransport runs RouterOS console commands over SSH. It dials a fresh
// connection per command: simple, and each command is independently bounded by
// the caller's context.
type SSHTransport struct {
	cfg SSHConfig

	// Connection reuse (NewReusingSSHTransport only). Opening an SSH connection over
	// the tunnel costs about three seconds, far more than running a command, so a
	// workflow that runs dozens of read commands one connection at a time is slow.
	// A reusing transport keeps one connection and opens a fresh session per command.
	reuse    bool
	mu       sync.Mutex
	client   *ssh.Client
	inflight int
	idle     *time.Timer
}

func NewSSHTransport(cfg SSHConfig) *SSHTransport { return &SSHTransport{cfg: cfg} }

// reuseIdle is how long a reused connection may sit unused before it is closed.
const reuseIdle = 6 * time.Second

// NewReusingSSHTransport behaves like NewSSHTransport but runs successive commands
// over one connection. Every command is still its own SSH session and is bounded by
// the same timeout; cancelling a command closes the connection, so the next command
// reconnects. If the router refuses a new session on a reused connection, the
// command has not run, and it is retried once on a fresh connection.
func NewReusingSSHTransport(cfg SSHConfig) *SSHTransport {
	return &SSHTransport{cfg: cfg, reuse: true}
}

// ConcurrentSafe reports that Run may be called from several goroutines at once;
// only a reusing transport is, because each command is its own session.
func (t *SSHTransport) ConcurrentSafe() bool { return t.reuse }

// Close drops any reused connection. It is optional: an idle connection closes itself.
func (t *SSHTransport) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.dropLocked(t.client)
}

func (t *SSHTransport) dropLocked(c *ssh.Client) {
	if c == nil {
		return
	}
	if t.client == c {
		t.client = nil
		if t.idle != nil {
			t.idle.Stop()
		}
	}
	_ = c.Close()
}

func (t *SSHTransport) drop(c *ssh.Client) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.dropLocked(c)
}

// begin returns the shared client, connecting if needed, and marks a command in
// flight so the idle timer cannot close the connection under it.
func (t *SSHTransport) begin(ctx context.Context, forceFresh bool) (*ssh.Client, bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if forceFresh {
		t.dropLocked(t.client)
	}
	fresh := false
	if t.client == nil {
		c, err := t.connect(ctx)
		if err != nil {
			return nil, false, err
		}
		t.client, fresh = c, true
	}
	t.inflight++
	return t.client, fresh, nil
}

func (t *SSHTransport) end(c *ssh.Client) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.inflight--
	if t.client != c {
		return
	}
	if t.idle == nil {
		t.idle = time.AfterFunc(reuseIdle, t.expire)
	} else {
		t.idle.Reset(reuseIdle)
	}
}

func (t *SSHTransport) expire() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.inflight > 0 {
		t.idle.Reset(reuseIdle)
		return
	}
	t.dropLocked(t.client)
}

// connect dials and authenticates. Only the handshake is bounded by the deadline;
// each command carries its own bound afterwards.
func (t *SSHTransport) connect(ctx context.Context) (*ssh.Client, error) {
	if t.cfg.Addr == "" {
		return nil, fmt.Errorf("routeros: empty ssh address")
	}
	ccfg, err := t.clientConfig()
	if err != nil {
		return nil, err
	}
	timeout := t.cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", t.cfg.Addr)
	if err != nil {
		return nil, fmt.Errorf("routeros: dial %s: %w", t.cfg.Addr, errors.Join(errSSHConnect, err))
	}
	deadline := time.Now().Add(timeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		conn.Close()
		return nil, fmt.Errorf("routeros: cannot bound ssh connection")
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	sconn, chans, reqs, err := ssh.NewClientConn(conn, t.cfg.Addr, ccfg)
	stopped := stop()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("routeros: ssh handshake %s: %w", t.cfg.Addr, errors.Join(errSSHConnect, err))
	}
	if !stopped || ctx.Err() != nil {
		sconn.Close()
		return nil, fmt.Errorf("routeros: ssh handshake %s: %w", t.cfg.Addr, errors.Join(errSSHConnect, ctx.Err()))
	}
	_ = conn.SetDeadline(time.Time{})
	return ssh.NewClient(sconn, chans, reqs), nil
}

// runReused runs one command as its own session on the shared connection.
func (t *SSHTransport) runReused(ctx context.Context, cmd string) (string, error) {
	timeout := t.cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	client, fresh, err := t.begin(ctx, false)
	if err != nil {
		return "", err
	}
	session, err := client.NewSession()
	if err != nil && !fresh {
		// A reused connection the router has since closed: nothing ran, so retry once.
		t.end(client)
		t.drop(client)
		if client, _, err = t.begin(ctx, true); err != nil {
			return "", err
		}
		session, err = client.NewSession()
	}
	if err != nil {
		t.end(client)
		t.drop(client)
		return "", fmt.Errorf("routeros: ssh session: %w", err)
	}
	// Cancelling or timing out interrupts the command by closing the connection.
	stop := context.AfterFunc(ctx, func() { t.drop(client) })
	raw, runErr := session.CombinedOutput(cmd)
	stop()
	_ = session.Close()
	t.end(client)
	text := string(raw)
	if runErr != nil {
		var exit *ssh.ExitError
		if !errors.As(runErr, &exit) {
			t.drop(client)
		}
		return "", fmt.Errorf("routeros: command execution failed: %w", runErr)
	}
	if perr := detectRouterOSError(text); perr != nil {
		return "", fmt.Errorf("routeros: router rejected the command")
	}
	return text, nil
}

// Run opens an SSH connection, executes cmd as a RouterOS console command, and
// returns combined output. RouterOS returns exit status 0 even for script-level
// failures (proven: a failing /import prints "Script Error" and exits 0), so the
// output is also scanned for error markers.
func (t *SSHTransport) Run(ctx context.Context, cmd string) (string, error) {
	// RouterOS intermittently refuses or drops a new SSH handshake when a
	// workflow opens many connections in a row. Nothing has been executed at
	// that point, so retrying the connection (not the command) is safe.
	const attempts = 3
	var out string
	var err error
	for i := 0; i < attempts; i++ {
		out, err = t.runOnce(ctx, cmd)
		var ne net.Error
		if err == nil || !errors.Is(err, errSSHConnect) || ctx.Err() != nil || (errors.As(err, &ne) && ne.Timeout()) {
			return out, err
		}
		select {
		case <-ctx.Done():
			return out, err
		case <-time.After(time.Duration(i+1) * 300 * time.Millisecond):
		}
	}
	return out, err
}

// errSSHConnect marks dial and handshake failures: the command never ran.
var errSSHConnect = errors.New("ssh connect")

func (t *SSHTransport) runOnce(ctx context.Context, cmd string) (string, error) {
	if t.reuse {
		return t.runReused(ctx, cmd)
	}
	if t.cfg.Addr == "" {
		return "", fmt.Errorf("routeros: empty ssh address")
	}
	ccfg, err := t.clientConfig()
	if err != nil {
		return "", err
	}

	timeout := t.cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", t.cfg.Addr)
	if err != nil {
		return "", fmt.Errorf("routeros: dial %s: %w", t.cfg.Addr, errors.Join(errSSHConnect, err))
	}
	defer conn.Close()
	// Bound handshake and command I/O as well as dial. Cancellation must also
	// interrupt an established connection so callers can enter containment.
	deadline := time.Now().Add(timeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return "", fmt.Errorf("routeros: cannot bound ssh connection")
	}
	stopCancellation := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopCancellation()

	sconn, chans, reqs, err := ssh.NewClientConn(conn, t.cfg.Addr, ccfg)
	if err != nil {
		return "", fmt.Errorf("routeros: ssh handshake %s: %w", t.cfg.Addr, errors.Join(errSSHConnect, err))
	}
	client := ssh.NewClient(sconn, chans, reqs)
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("routeros: ssh session: %w", err)
	}
	defer session.Close()

	raw, runErr := session.CombinedOutput(cmd)
	text := string(raw)
	if runErr != nil {
		return "", fmt.Errorf("routeros: command execution failed: %w", runErr)
	}
	if perr := detectRouterOSError(text); perr != nil {
		// RouterOS output may echo secret-bearing arguments. Do not return
		// command text or raw provider output through API/audit errors.
		return "", fmt.Errorf("routeros: router rejected the command")
	}
	return text, nil
}

func (t *SSHTransport) clientConfig() (*ssh.ClientConfig, error) {
	var auths []ssh.AuthMethod
	if len(t.cfg.PrivateKeyPEM) > 0 {
		signer, err := ssh.ParsePrivateKey(t.cfg.PrivateKeyPEM)
		if err != nil {
			return nil, fmt.Errorf("routeros: parse ssh private key: %w", err)
		}
		auths = append(auths, ssh.PublicKeys(signer))
	}
	if t.cfg.Password != "" || len(auths) == 0 {
		auths = append(auths, ssh.Password(t.cfg.Password))
	}
	var hostKeyCallback ssh.HostKeyCallback
	if fingerprint := strings.TrimSpace(t.cfg.HostKeyFingerprint); fingerprint != "" {
		hostKeyCallback = func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			actual := ssh.FingerprintSHA256(key)
			if actual != fingerprint {
				return fmt.Errorf("routeros: ssh host key mismatch for %s: got %s", hostname, actual)
			}
			return nil
		}
	} else if t.cfg.InsecureIgnoreHostKey {
		hostKeyCallback = ssh.InsecureIgnoreHostKey()
	} else {
		return nil, fmt.Errorf("routeros: host-key verification requested but no SHA256 fingerprint is configured; refusing")
	}
	return &ssh.ClientConfig{
		User:            t.cfg.User,
		Auth:            auths,
		HostKeyCallback: hostKeyCallback,
		Timeout:         t.cfg.Timeout,
	}, nil
}

// rosErrorMarkers are output strings RouterOS emits for script-level failures
// that do not surface as a non-zero exit status.
var rosErrorMarkers = []string{
	"Script Error",
	"failure:",
	"bad command name",
	"bad parameter",
	"no such item",
}

func detectRouterOSError(out string) error {
	for _, line := range strings.Split(out, "\n") {
		for _, prefix := range []string{"syntax error (line ", "expected end of command (line ", "expected value value (line ", "input does not match any value of "} {
			if strings.HasPrefix(strings.TrimSpace(line), prefix) {
				return fmt.Errorf("routeros reported a syntax error")
			}
		}
		for _, m := range rosErrorMarkers {
			if strings.Contains(line, m) {
				return fmt.Errorf("routeros reported an error: %s", strings.TrimSpace(line))
			}
		}
	}
	return nil
}

func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return ""
}
