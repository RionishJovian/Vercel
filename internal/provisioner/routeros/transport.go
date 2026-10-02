package routeros

import (
	"context"
	"fmt"
	"net"
	"strings"
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
}

func NewSSHTransport(cfg SSHConfig) *SSHTransport { return &SSHTransport{cfg: cfg} }

// Run opens an SSH connection, executes cmd as a RouterOS console command, and
// returns combined output. RouterOS returns exit status 0 even for script-level
// failures (proven: a failing /import prints "Script Error" and exits 0), so the
// output is also scanned for error markers.
func (t *SSHTransport) Run(ctx context.Context, cmd string) (string, error) {
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
		return "", fmt.Errorf("routeros: dial %s: %w", t.cfg.Addr, err)
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
		return "", fmt.Errorf("routeros: ssh handshake %s: %w", t.cfg.Addr, err)
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
