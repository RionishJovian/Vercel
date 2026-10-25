package server

import (
	"context"
	"fmt"
	"time"

	"mega-magic/internal/database"
	"mega-magic/internal/handler"
	"mega-magic/internal/provisioner/routeros"
	"mega-magic/internal/secretbox"
)

// Periodic router -> hub -> app voucher-login accounting synchronization. The
// RouterBOARD validates issued codes locally (ground-operation design); this
// worker pulls each gateway's live accounting through the Magic Hub so the
// app's vouchers, activation timestamps and reports follow the router without
// an operator pressing Sync.
const guestVoucherSyncInterval = 60 * time.Second

// buildGuestVoucherSyncWorker shares the voucher opt-in: no worker, schema
// access or router communication is enabled when the flag is false.
func (s *Server) buildGuestVoucherSyncWorker() (func(context.Context), error) {
	if !s.cfg.GuestWifi.VoucherAccessEnabled {
		return nil, nil
	}
	if !s.cfg.Provisioner.Applier.Enabled {
		return nil, fmt.Errorf("voucher login synchronization requires the live applier")
	}
	if s.db == nil {
		return nil, fmt.Errorf("voucher login synchronization requires the database")
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
	applier := routeros.NewDeviceApplier(source, a.Timeout, nil, a.Interface, a.ListenPortRotateTo, a.ResetHold).WithControlConfig(a.PingTarget, a.TDown)
	h := &handler.GuestVoucherHandler{
		DB:          s.db,
		Vouchers:    handler.NewVoucherHandler(s.db, database.NewVoucherRepository(s.db), database.NewBusinessMeteredQuotaRepository(s.db)),
		Provisioner: voucherLoginTransport{applier: applier},
		Syncer:      voucherLoginTransport{applier: applier},
	}
	return func(ctx context.Context) {
		syncTenant := func(biz string) {
			if ctx.Err() != nil {
				return
			}
			work, stop := context.WithTimeout(ctx, 60*time.Second)
			defer stop()
			applied, err := h.SyncBusinessVouchers(work, biz)
			if err != nil {
				s.sugar.Warnw("Voucher login synchronization pending", "business_id", biz, "error", err)
				return
			}
			if applied > 0 {
				s.sugar.Infow("Voucher login accounting synchronized", "business_id", biz, "gateways_applied", applied)
			}
		}
		// The system worker enumerates tenant IDs only; every accounting read
		// and write runs under that tenant's RLS context (SyncBusinessVouchers
		// opens a tenant transaction per gateway).
		scan := func() {
			cursor := ""
			for {
				query, cancel := context.WithTimeout(ctx, 5*time.Second)
				rows, err := s.db.QueryContext(query, `SELECT business_id::text FROM businesses WHERE business_id::text>$1 ORDER BY business_id::text LIMIT 25`, cursor)
				if err != nil {
					cancel()
					s.sugar.Warn("Voucher login synchronization tenant scan unavailable")
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
					s.sugar.Warn("Voucher login synchronization tenant scan unavailable")
					return
				}
				for _, biz := range tenants {
					if ctx.Err() != nil {
						return
					}
					syncTenant(biz)
				}
				if len(tenants) < 25 {
					return
				}
				cursor = tenants[len(tenants)-1]
			}
		}
		scan() // Startup reconciliation, not only the next scheduled tick.
		ticker := time.NewTicker(guestVoucherSyncInterval)
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

// startGuestVoucherSyncWorker launches the periodic accounting worker once.
func (s *Server) startGuestVoucherSyncWorker() error {
	worker, err := s.buildGuestVoucherSyncWorker()
	if err != nil {
		return err
	}
	if worker == nil {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.guestVSyncMu.Lock()
	defer s.guestVSyncMu.Unlock()
	if s.guestVSyncCancel != nil {
		cancel()
		return fmt.Errorf("voucher login synchronization worker already started")
	}
	s.guestVSyncCancel = cancel
	done := make(chan struct{})
	s.guestVSyncDone = done
	go func() { defer close(done); worker(ctx) }()
	return nil
}

// stopGuestVoucherSyncWorker cancels the worker and waits briefly for it.
func (s *Server) stopGuestVoucherSyncWorker() {
	s.guestVSyncMu.Lock()
	cancel, done := s.guestVSyncCancel, s.guestVSyncDone
	s.guestVSyncCancel, s.guestVSyncDone = nil, nil
	s.guestVSyncMu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	if done == nil {
		return
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
}
