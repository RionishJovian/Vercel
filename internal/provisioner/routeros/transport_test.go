package routeros

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestDetectRouterOSError(t *testing.T) {
	clean := []string{
		"",
		"Configuration backup saved",
		"  name: wg-magichub",
	}
	for _, c := range clean {
		if err := detectRouterOSError(c); err != nil {
			t.Fatalf("detectRouterOSError(%q) = %v, want nil", c, err)
		}
	}
	dirty := []string{
		"Script Error: failure: already have interface with name wg-magichub (:import; line 1)",
		"failure: already have address",
		"bad parameter interface (line 1 column 63)",
		"bad command name print (line 1 column 16)",
		"no such item",
	}
	for _, d := range dirty {
		if err := detectRouterOSError(d); err == nil {
			t.Fatalf("detectRouterOSError(%q) = nil, want error", d)
		}
	}
}

func TestSSHTransport_emptyAddr(t *testing.T) {
	tr := NewSSHTransport(SSHConfig{})
	if _, err := tr.Run(context.Background(), "/system identity print"); err == nil {
		t.Fatal("empty addr must error")
	}
}

func TestSSHTransport_hostKeyFailClosed(t *testing.T) {
	tr := NewSSHTransport(SSHConfig{Addr: "172.20.0.250:22", User: "admin", InsecureIgnoreHostKey: false})
	_, err := tr.Run(context.Background(), "/system identity print")
	if err == nil || !strings.Contains(err.Error(), "host-key verification") {
		t.Fatalf("err = %v, want host-key fail-closed", err)
	}
}

func TestSSHTransport_pinnedHostKey(t *testing.T) {
	publicA, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicB, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keyA, err := ssh.NewPublicKey(publicA)
	if err != nil {
		t.Fatal(err)
	}
	keyB, err := ssh.NewPublicKey(publicB)
	if err != nil {
		t.Fatal(err)
	}

	tr := NewSSHTransport(SSHConfig{
		Addr:               "10.0.0.1:22",
		User:               "admin",
		Password:           "secret",
		HostKeyFingerprint: ssh.FingerprintSHA256(keyA),
	})
	cfg, err := tr.clientConfig()
	if err != nil {
		t.Fatalf("client config: %v", err)
	}
	remote := &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 22}
	if err := cfg.HostKeyCallback("10.0.0.1:22", remote, keyA); err != nil {
		t.Fatalf("matching key rejected: %v", err)
	}
	if err := cfg.HostKeyCallback("10.0.0.1:22", remote, keyB); err == nil || !strings.Contains(err.Error(), "host key mismatch") {
		t.Fatalf("mismatched key error = %v", err)
	}
}

func TestSSHTransport_unreachableDial(t *testing.T) {
	// No server: a dial error must surface, not hang.
	tr := NewSSHTransport(SSHConfig{Addr: "127.0.0.1:1", User: "admin", InsecureIgnoreHostKey: true, Timeout: 500 * time.Millisecond})
	if _, err := tr.Run(context.Background(), "/system identity print"); err == nil {
		t.Fatal("dial to a closed port must error")
	}
}
