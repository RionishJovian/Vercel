package routeros

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// WebFigResult is returned only after RouterOS reports HTTPS WebFig enabled,
// plaintext WebFig disabled, and the trusted-source restriction in place.
type WebFigResult struct {
	Backup      string `json:"backup"`
	Verified    bool   `json:"verified"`
	RolledBack  bool   `json:"rolled_back"`
	URL         string `json:"url,omitempty"`
	Certificate string `json:"certificate,omitempty"`
}

type webFigService struct {
	Disabled, Address, Certificate, TLSVersion string
	Port                                       int
}

var webFigLocks sync.Map

func webFigBool(value string) bool {
	return value == "yes" || value == "true"
}

func readWebFigService(ctx context.Context, runner CommandRunner, name string) (webFigService, error) {
	out, err := runner.Run(ctx, "/ip service print value-list detail where name="+routerOSQuoted(name))
	if err != nil {
		return webFigService{}, err
	}
	v := parseValueList(out)
	if v["name"] != name {
		return webFigService{}, fmt.Errorf("RouterOS service %s is unavailable", name)
	}
	port, err := strconv.Atoi(v["port"])
	if err != nil || port < 1 || port > 65535 {
		return webFigService{}, fmt.Errorf("RouterOS service %s returned an invalid port", name)
	}
	return webFigService{Disabled: v["disabled"], Address: v["address"], Certificate: v["certificate"], TLSVersion: v["tls-version"], Port: port}, nil
}

func restoreWebFigService(ctx context.Context, runner CommandRunner, name string, before webFigService) error {
	disabled := "no"
	if webFigBool(before.Disabled) {
		disabled = "yes"
	}
	cmd := fmt.Sprintf("/ip service set [find where name=%s] port=%d address=%s disabled=%s", routerOSQuoted(name), before.Port, routerOSQuoted(before.Address), disabled)
	if name == "www-ssl" {
		certificate := before.Certificate
		if certificate == "" {
			certificate = "none"
		}
		cmd += " certificate=" + routerOSQuoted(certificate)
		if before.TLSVersion != "" {
			cmd += " tls-version=" + routerOSQuoted(before.TLSVersion)
		}
	}
	_, err := runner.Run(ctx, cmd)
	return err
}

// ConfigureSecureWebFig performs a targeted, tenant-bound WebFig change. It
// creates an on-router backup, preserves an existing HTTPS certificate when
// present, otherwise creates a device-local self-signed certificate, restricts
// HTTPS to the configured management networks, disables plaintext HTTP, and
// reads the resulting service state back. A failed readback restores both
// service records and removes only a certificate created by this operation.
func (a *DeviceApplier) ConfigureSecureWebFig(ctx context.Context, businessID, deviceID string, trustedCIDRs []string) (WebFigResult, error) {
	result := WebFigResult{}
	trusted, err := validatedCIDRList(trustedCIDRs)
	if err != nil {
		return result, err
	}
	lock, _ := webFigLocks.LoadOrStore(businessID+":"+deviceID, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	target, err := a.src.TargetForDevice(ctx, businessID, deviceID)
	if err != nil {
		return result, err
	}
	cfg, err := SSHConfigFromTarget(target, a.timeout)
	if err != nil {
		return result, err
	}
	runner := a.factory(cfg)
	if runner == nil {
		return result, fmt.Errorf("routeros: no command runner")
	}
	plainBefore, err := readWebFigService(ctx, runner, "www")
	if err != nil {
		return result, fmt.Errorf("read plaintext WebFig: %w", err)
	}
	secureBefore, err := readWebFigService(ctx, runner, "www-ssl")
	if err != nil {
		return result, fmt.Errorf("read HTTPS WebFig: %w", err)
	}
	result.Backup = snapshotName(deviceID+"-webfig", time.Now().UTC())
	for _, command := range []string{"/export file=" + result.Backup, "/system backup save name=" + result.Backup} {
		if _, err = runner.Run(ctx, command); err != nil {
			return result, fmt.Errorf("WebFig backup failed; nothing was changed")
		}
	}

	createdCertificate := ""
	rollback := func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		ok := restoreWebFigService(cleanup, runner, "www", plainBefore) == nil
		if restoreWebFigService(cleanup, runner, "www-ssl", secureBefore) != nil {
			ok = false
		}
		if createdCertificate != "" {
			if _, removeErr := runner.Run(cleanup, "/certificate remove [find where name="+routerOSQuoted(createdCertificate)+"]"); removeErr != nil {
				ok = false
			}
		}
		result.RolledBack = ok
	}

	certificate := strings.TrimSpace(secureBefore.Certificate)
	if certificate == "" || certificate == "none" {
		hash := sha256.Sum256([]byte(businessID + ":" + deviceID))
		certificate = "mm-webfig-" + hex.EncodeToString(hash[:4])
		createdCertificate = certificate
		commonName := "mega-magic-webfig"
		if host, _, splitErr := net.SplitHostPort(target.Addr); splitErr == nil && net.ParseIP(host) != nil {
			commonName = host
		}
		if _, err = runner.Run(ctx, "/certificate add name="+routerOSQuoted(certificate)+" common-name="+routerOSQuoted(commonName)+" key-usage=tls-server"); err != nil {
			rollback()
			return result, fmt.Errorf("could not create WebFig certificate")
		}
		if _, err = runner.Run(ctx, "/certificate sign [find where name="+routerOSQuoted(certificate)+"]"); err != nil {
			rollback()
			return result, fmt.Errorf("could not sign WebFig certificate")
		}
		out, verifyErr := runner.Run(ctx, "/certificate print count-only where name="+routerOSQuoted(certificate)+" private-key=yes")
		if verifyErr != nil || strings.TrimSpace(out) != "1" {
			rollback()
			return result, fmt.Errorf("WebFig certificate did not verify")
		}
	}

	setSecure := "/ip service set [find where name=\"www-ssl\"] port=443 certificate=" + routerOSQuoted(certificate) + " address=" + routerOSQuoted(trusted) + " tls-version=only-1.2 disabled=no"
	if _, err = runner.Run(ctx, setSecure); err != nil {
		rollback()
		return result, fmt.Errorf("could not enable restricted HTTPS WebFig")
	}
	if _, err = runner.Run(ctx, "/ip service disable [find where name=\"www\"]"); err != nil {
		rollback()
		return result, fmt.Errorf("could not disable plaintext WebFig")
	}
	plainAfter, plainErr := readWebFigService(ctx, runner, "www")
	secureAfter, secureErr := readWebFigService(ctx, runner, "www-ssl")
	if plainErr != nil || secureErr != nil || !webFigBool(plainAfter.Disabled) || webFigBool(secureAfter.Disabled) || secureAfter.Port != 443 || secureAfter.Address != trusted || secureAfter.Certificate != certificate {
		rollback()
		return result, fmt.Errorf("WebFig readback failed; previous service settings were restored")
	}
	host, _, _ := net.SplitHostPort(target.Addr)
	if net.ParseIP(host) != nil {
		result.URL = "https://" + net.JoinHostPort(host, "443") + "/"
	}
	result.Certificate = certificate
	result.Verified = true
	return result, nil
}
