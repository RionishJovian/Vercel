package routeros

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

type webFigRunner struct {
	plain, secure webFigService
	certificates  map[string]bool
	commands      []string
	failDisable   bool
}

func (r *webFigRunner) service(name string) *webFigService {
	if name == "www" {
		return &r.plain
	}
	return &r.secure
}

func (r *webFigRunner) Run(_ context.Context, command string) (string, error) {
	r.commands = append(r.commands, command)
	if strings.HasPrefix(command, "/export ") || strings.HasPrefix(command, "/system backup ") || strings.HasPrefix(command, "/certificate sign ") {
		return "", nil
	}
	if strings.HasPrefix(command, "/ip service print value-list detail") {
		name := "www-ssl"
		if strings.Contains(command, `name="www"`) {
			name = "www"
		}
		s := r.service(name)
		return fmt.Sprintf("name: %s\nport: %d\ndisabled: %s\naddress: %s\ncertificate: %s\ntls-version: %s\n", name, s.Port, s.Disabled, s.Address, s.Certificate, s.TLSVersion), nil
	}
	if strings.HasPrefix(command, "/certificate add ") {
		name := valueAfter(command, `name="`)
		r.certificates[name] = true
		return "", nil
	}
	if strings.HasPrefix(command, "/certificate print count-only") {
		for _, present := range r.certificates {
			if present {
				return "1", nil
			}
		}
		return "0", nil
	}
	if strings.HasPrefix(command, "/certificate remove ") {
		for name := range r.certificates {
			delete(r.certificates, name)
		}
		return "", nil
	}
	if strings.HasPrefix(command, "/ip service disable ") {
		if r.failDisable {
			r.failDisable = false
			return "", errors.New("simulated write failure")
		}
		r.plain.Disabled = "true"
		return "", nil
	}
	if strings.HasPrefix(command, "/ip service set ") {
		name := "www-ssl"
		if strings.Contains(command, `name="www"`) {
			name = "www"
		}
		s := r.service(name)
		if strings.Contains(command, "disabled=yes") {
			s.Disabled = "true"
		} else if strings.Contains(command, "disabled=no") {
			s.Disabled = "false"
		}
		if v := valueAfter(command, `address="`); v != "" || strings.Contains(command, `address=""`) {
			s.Address = v
		}
		if v := valueAfter(command, `certificate="`); v != "" {
			s.Certificate = v
		}
		if v := valueAfter(command, `tls-version="`); v != "" {
			s.TLSVersion = v
		}
		if strings.Contains(command, "port=443") {
			s.Port = 443
		}
		return "", nil
	}
	return "", fmt.Errorf("unexpected command: %s", command)
}

func valueAfter(command, marker string) string {
	start := strings.Index(command, marker)
	if start < 0 {
		return ""
	}
	start += len(marker)
	end := strings.Index(command[start:], `"`)
	if end < 0 {
		return ""
	}
	return command[start : start+end]
}

func newWebFigRunner() *webFigRunner {
	return &webFigRunner{
		plain:        webFigService{Disabled: "false", Port: 80},
		secure:       webFigService{Disabled: "true", Port: 443, Certificate: "none", TLSVersion: "any"},
		certificates: map[string]bool{},
	}
}

func TestConfigureSecureWebFigBacksUpCreatesCertificateAndVerifies(t *testing.T) {
	runner := newWebFigRunner()
	applier := quickConfigApplier(runner)
	result, err := applier.ConfigureSecureWebFig(context.Background(), "biz-1", "dev-1", []string{"10.99.0.0/24"})
	if err != nil || !result.Verified || result.Backup == "" || result.URL != "https://10.99.0.3:443/" {
		t.Fatalf("result=%+v error=%v", result, err)
	}
	if !webFigBool(runner.plain.Disabled) || webFigBool(runner.secure.Disabled) || runner.secure.Address != "10.99.0.0/24" || runner.secure.Certificate == "none" {
		t.Fatalf("services were not secured: plain=%+v secure=%+v", runner.plain, runner.secure)
	}
}

func TestConfigureSecureWebFigRollsBackOnPartialFailure(t *testing.T) {
	runner := newWebFigRunner()
	runner.failDisable = true
	applier := quickConfigApplier(runner)
	result, err := applier.ConfigureSecureWebFig(context.Background(), "biz-1", "dev-1", []string{"10.99.0.0/24"})
	if err == nil || !result.RolledBack {
		t.Fatalf("expected verified rollback: result=%+v error=%v", result, err)
	}
	if webFigBool(runner.plain.Disabled) || !webFigBool(runner.secure.Disabled) || runner.secure.Certificate != "none" || len(runner.certificates) != 0 {
		t.Fatalf("rollback did not restore services: plain=%+v secure=%+v certs=%v", runner.plain, runner.secure, runner.certificates)
	}
}

func TestConfigureSecureWebFigRejectsUntrustedOrForeignTarget(t *testing.T) {
	runner := newWebFigRunner()
	applier := quickConfigApplier(runner)
	if _, err := applier.ConfigureSecureWebFig(context.Background(), "biz-1", "dev-1", []string{"not-a-network"}); err == nil {
		t.Fatal("invalid trusted network accepted")
	}
	if _, err := applier.ConfigureSecureWebFig(context.Background(), "other-business", "dev-1", []string{"10.99.0.0/24"}); err == nil {
		t.Fatal("foreign tenant target accepted")
	}
}
