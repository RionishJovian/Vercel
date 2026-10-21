package server

import (
	"testing"

	"go.uber.org/zap"
	"mega-magic/internal/config"
)

// The voucher-login accounting worker shares the voucher opt-in: nothing runs
// or connects when the flag is off, and a voucher-enabled deployment cannot
// start without a live applier and database.

func syncTestServer(t *testing.T, guestEnabled, applierEnabled bool, withDB bool) *Server {
	t.Helper()
	cfg := &config.Config{}
	cfg.GuestWifi.VoucherAccessEnabled = guestEnabled
	cfg.Provisioner.Applier.Enabled = applierEnabled
	s := &Server{cfg: cfg, sugar: zap.NewNop().Sugar()}
	if withDB {
		// A nil DB would panic inside real wiring; the database gate must
		// reject before any of that runs, so the nil value IS the test.
		s.db = nil
	}
	return s
}

func TestGuestVoucherSyncWorker_gatesOnTheVoucherFlag(t *testing.T) {
	s := syncTestServer(t, false, true, false)
	worker, err := s.buildGuestVoucherSyncWorker()
	if err != nil || worker != nil {
		t.Fatalf("voucher flag off must disable the worker entirely, got worker=%v err=%v", worker != nil, err)
	}
}

func TestGuestVoucherSyncWorker_requiresApplierAndDatabase(t *testing.T) {
	s := syncTestServer(t, true, false, false)
	if worker, err := s.buildGuestVoucherSyncWorker(); err == nil || worker != nil {
		t.Fatal("voucher flag on must require the live applier")
	}
	s = syncTestServer(t, true, true, false)
	if worker, err := s.buildGuestVoucherSyncWorker(); err == nil || worker != nil {
		t.Fatal("voucher flag on must require the database")
	}
}

func TestGuestVoucherSyncWorker_startStopAreIdempotent(t *testing.T) {
	s := syncTestServer(t, false, true, false)
	// Flag off: start is a no-op and stop never blocks.
	if err := s.startGuestVoucherSyncWorker(); err != nil {
		t.Fatal(err)
	}
	s.stopGuestVoucherSyncWorker()
	s.stopGuestVoucherSyncWorker() // second stop must not panic or hang
}
