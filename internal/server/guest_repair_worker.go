package server

import (
	"context"
	"fmt"
	"time"

	"mega-magic/internal/database"
	"mega-magic/internal/guestaccess"
	"mega-magic/internal/provisioner/routeros"
	"mega-magic/internal/secretbox"
)

// buildGuestRepairWorker shares the voucher opt-in. No worker, schema access or
// router mutation is enabled on existing deployments when the flag is false.
func (s *Server) buildGuestRepairWorker() (func(context.Context), error) {
	if !s.cfg.GuestWifi.VoucherAccessEnabled {
		return nil, nil
	}
	if !s.cfg.Provisioner.Applier.Enabled {
		return nil, fmt.Errorf("guest recovery requires the live applier")
	}
	box, err := secretbox.New(s.cfg.Provisioner.Applier.CredentialKey)
	if err != nil {
		return nil, err
	}
	source, err := routeros.NewCIDRTargetSource(database.NewDeviceCredentialSource(s.db, box), s.cfg.Provisioner.AllowedTargetCIDRs)
	if err != nil {
		return nil, err
	}
	a := s.cfg.Provisioner.Applier
	router := routeros.NewDeviceApplier(source, a.Timeout, nil, a.Interface, a.ListenPortRotateTo, a.ResetHold).WithControlConfig(a.PingTarget, a.TDown)
	repair := guestaccess.RecoveryReconciler{Store: database.GuestRecoveryRepository{DB: s.db}, Router: router}
	return func(ctx context.Context) {
		// The system worker enumerates tenant IDs only; every recovery read/write
		// runs under that tenant's RLS context. Include inactive businesses because
		// containment must not depend on their subscription or suspension status.
		cursor := ""
		scan := func() {
			query, cancel := context.WithTimeout(ctx, 5*time.Second)
			rows, err := s.db.QueryContext(query, `SELECT business_id::text FROM businesses WHERE business_id::text>$1 ORDER BY business_id::text LIMIT 25`, cursor)
			if err != nil {
				cancel()
				s.sugar.Warn("Guest recovery tenant scan unavailable")
				return
			}
			var tenants []string
			for rows.Next() {
				var biz string
				if err = rows.Scan(&biz); err != nil {
					break
				}
				tenants = append(tenants, biz)
			}
			if err == nil {
				err = rows.Err()
			}
			rows.Close()
			cancel()
			if err != nil {
				s.sugar.Warn("Guest recovery tenant scan unavailable")
				return
			}
			if len(tenants) < 25 {
				cursor = ""
			} else {
				cursor = tenants[len(tenants)-1]
			}
			for _, biz := range tenants {
				if ctx.Err() != nil {
					return
				}
				work, stop := context.WithTimeout(ctx, 30*time.Second)
				result, err := repair.Reconcile(work, biz)
				stop()
				if err != nil || result.Pending > 0 {
					s.sugar.Warnw("Guest recovery containment pending", "business_id", biz, "pending", result.Pending)
				}
			}
		}
		scan() // Startup reconciliation, not only the next scheduled tick.
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				scan()
			}
		}
	}, nil
}
