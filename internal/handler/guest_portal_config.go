package handler

import (
	"context"
	"database/sql"

	"mega-magic/internal/database"
	"mega-magic/internal/guestaccess"
	"mega-magic/internal/model"
)

// GuestPortalConfigResolver shares the public portal's device/site policy while
// retaining the paid-access check for an active tenant. The gateway identity is
// supplied by the handler's trusted device lookup, never a JSON business/site ID.
func GuestPortalConfigResolver(db *database.DB) func(context.Context, string, string) (*model.PortalConfig, error) {
	return func(ctx context.Context, businessID, deviceID string) (cfg *model.PortalConfig, err error) {
		if db == nil {
			return nil, guestaccess.ErrProvisionDenied
		}
		tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		err = database.WithTenant(tx, businessID, func() error {
			var active bool
			if err := tx.QueryRowContext(ctx, `SELECT is_active FROM businesses WHERE business_id=$1`, businessID).Scan(&active); err != nil {
				return err
			}
			if !active {
				return guestaccess.ErrProvisionDenied
			}
			var err error
			cfg, err = database.NewPortalConfigRepository(db).GetByDeviceTXN(tx, businessID, deviceID)
			return err
		})
		return cfg, err
	}
}
