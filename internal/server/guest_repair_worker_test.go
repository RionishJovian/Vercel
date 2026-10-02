package server

import (
	"mega-magic/internal/config"
	"testing"
)

func TestGuestRepairWorkerOffHasNoDependencies(t *testing.T) {
	s := &Server{cfg: &config.Config{}}
	worker, err := s.buildGuestRepairWorker()
	if err != nil || worker != nil {
		t.Fatal("disabled recovery initialized live dependencies", err)
	}
}

func TestGuestRepairWorkerRequiresApplier(t *testing.T) {
	cfg := &config.Config{}
	cfg.GuestWifi.VoucherAccessEnabled = true
	worker, err := (&Server{cfg: cfg}).buildGuestRepairWorker()
	if err == nil || worker != nil {
		t.Fatal("repair started without approved router I/O")
	}
}
