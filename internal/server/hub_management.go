package server

import (
	"context"
	"errors"
	"mega-magic/internal/config"
	"mega-magic/internal/database"
	"mega-magic/internal/handler"
	"mega-magic/internal/provisioner/routeros"
	"mega-magic/internal/secretbox"
	"strings"
	"time"
)

type enrollmentTarget struct{ target *routeros.DeviceTarget }

func (s enrollmentTarget) TargetForDevice(context.Context, string, string) (*routeros.DeviceTarget, error) {
	return s.target, nil
}

func configureHubManagement(h *handler.VPSHandler, db *database.DB, cfg *config.Config) error {
	box, err := secretbox.New(cfg.Provisioner.Applier.CredentialKey)
	if err != nil {
		return errors.New("management credential encryption unavailable")
	}
	source := database.NewDeviceCredentialSource(db, box)
	configureAutomaticHubManagement(h, source, cfg.Provisioner.AllowedTargetCIDRs, cfg.WireGuard.Interface)
	// Validate the outbound allowlist once before registering the route callback.
	if _, err := routeros.NewCIDRTargetSource(enrollmentTarget{}, cfg.Provisioner.AllowedTargetCIDRs); err != nil {
		return err
	}
	h.ConfigureManagementAuthentication(func(ctx context.Context, biz, id, address, user, password string, port int, fingerprint string) error {
		target := &routeros.DeviceTarget{DeviceID: id, Addr: routeros.JoinHostPort(address, port), User: user, Secret: password, Kind: "password", Fingerprint: fingerprint}
		policy, err := routeros.NewCIDRTargetSource(enrollmentTarget{target}, cfg.Provisioner.AllowedTargetCIDRs)
		if err != nil {
			return err
		}
		checked, err := policy.TargetForDevice(ctx, biz, id)
		if err != nil {
			return err
		}
		previous, err := source.TargetForDevice(ctx, biz, id)
		if err != nil && !errors.Is(err, routeros.ErrNoTarget) {
			return err
		}
		if previous != nil && previous.Fingerprint != fingerprint {
			return errors.New("host key changes require a separate repair")
		}
		ssh, err := routeros.SSHConfigFromTarget(checked, 15*time.Second)
		if err != nil {
			return err
		}
		output, err := routeros.NewSSHTransport(ssh).Run(ctx, "/system resource print")
		if err != nil {
			return errors.New("pinned SSH verification failed")
		}
		// Do not accept a generic SSH host at a recycled tunnel address.
		if !strings.Contains(output, "version:") || !strings.Contains(output, "board-name:") {
			return errors.New("RouterOS identity verification failed")
		}
		return source.SetDeviceCredentials(ctx, biz, id, address, user, "password", password, port, fingerprint)
	})
	return nil
}
