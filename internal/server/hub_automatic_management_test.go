package server

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"net"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"mega-magic/internal/provisioner/routeros"
)

func TestAutomaticRouterProof(t *testing.T) {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	p := pendingManagement{private: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), user: "mm-test", proof: strings.Repeat("a", 64), routerKey: "expected-wireguard-key"}
	for _, tc := range []struct {
		name, proof, key string
		success          bool
	}{
		{"valid", p.proof, p.routerKey, true},
		{"wrong proof", "wrong", p.routerKey, false},
		{"wrong router key", p.proof, "other-key", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			cfg := &ssh.ServerConfig{PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
				if string(key.Marshal()) != string(signer.PublicKey().Marshal()) {
					t.Error("wrong authentication key")
				}
				return nil, nil
			}}
			cfg.AddHostKey(signer)
			done := make(chan struct{})
			go func() {
				defer close(done)
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				server, chans, reqs, err := ssh.NewServerConn(conn, cfg)
				if err != nil {
					return
				}
				defer server.Close()
				go ssh.DiscardRequests(reqs)
				for ch := range chans {
					channel, requests, err := ch.Accept()
					if err != nil {
						return
					}
					for req := range requests {
						if req.Type == "exec" {
							_ = req.Reply(true, nil)
							_, _ = channel.Write([]byte("mega-magic:" + p.user + ":" + tc.proof + "\n" + tc.key + "\nversion: 7.20\nboard-name: CHR\n"))
							_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
							_ = channel.Close()
							_ = server.Wait()
							return
						} else {
							_ = req.Reply(false, nil)
						}
					}
				}
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			pin, err := verifyAutomaticRouter(ctx, &routeros.DeviceTarget{DeviceID: "router", Addr: listener.Addr().String()}, p)
			if (err == nil) != tc.success {
				t.Fatalf("success=%v, err=%v", tc.success, err)
			}
			if tc.success && pin != ssh.FingerprintSHA256(signer.PublicKey()) {
				t.Fatal("wrong host key pinned")
			}
			if !tc.success && pin != "" {
				t.Fatal("unverified host key escaped")
			}
			<-done
		})
	}
}

func TestAutomaticManagementScriptAndOutputBounds(t *testing.T) {
	p := pendingManagement{user: "mm-123", public: "ssh-rsa public", private: "PRIVATE MUST STAY IN APP", proof: strings.Repeat("b", 64)}
	script := automaticManagementScript("router", p)
	if strings.Contains(script, "%!") || strings.Contains(script, p.private) {
		t.Fatal("invalid script or private SSH key disclosure")
	}
	if !strings.Contains(script, "address=10.77.0.9/32") || !strings.Contains(script, "in-interface=\"mmh-") {
		t.Fatal("missing private management restrictions")
	}
	b := &boundedEnrollmentOutput{}
	if _, err := b.Write(make([]byte, 65536)); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Write([]byte("x")); err == nil {
		t.Fatal("unbounded router response")
	}
}
