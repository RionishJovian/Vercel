package server

import (
	"fmt"
	"mega-magic/internal/config"
	"mega-magic/internal/database"
	"mega-magic/internal/handler"
	"mega-magic/internal/provisioner/routeros"
	"mega-magic/internal/secretbox"
)

func wireRouterObservations(cfg *config.Config, db *database.DB, h *handler.RouterHandler) error {
	if !cfg.Provisioner.InventoryEnabled {
		return nil
	}
	if db == nil || h == nil {
		return fmt.Errorf("router observations require database and handler")
	}
	a := cfg.Provisioner.Applier
	box, err := secretbox.New(a.CredentialKey)
	if err != nil {
		return fmt.Errorf("router observations credential configuration: %w", err)
	}
	source, err := routeros.NewCIDRTargetSource(database.NewDeviceCredentialSource(db, box), cfg.Provisioner.AllowedTargetCIDRs)
	if err != nil {
		return err
	}
	applier := routeros.NewDeviceApplier(source, a.Timeout, nil, a.Interface, a.ListenPortRotateTo, a.ResetHold)
	h.EnableObservations(applier)
	h.EnableGuestActions(applier) // Stays refused until MM_GUEST_ACTIONS_ENABLED is set.
	return nil
}
