package handler

import (
	"testing"

	"mega-magic/internal/provisioner"
)

func TestBackupButtonsMatchWhatTheRoutesWouldDo(t *testing.T) {
	off := &BackupHandler{}
	if off.captureAvailable() || off.restoreAvailable() {
		t.Fatal("without a live applier both routes refuse")
	}
	noSecret := &BackupHandler{}
	noSecret.EnableLive(struct{ provisioner.Applier }{}, provisioner.NewTOTPVerifier(provisioner.TOTPConfig{}))
	if !noSecret.captureAvailable() || noSecret.restoreAvailable() {
		t.Fatal("capture works without a code; restore can't")
	}
	live := &BackupHandler{}
	live.EnableLive(struct{ provisioner.Applier }{}, provisioner.NewTOTPVerifier(provisioner.TOTPConfig{Secret: "JBSWY3DPEHPK3PXP"}))
	if !live.captureAvailable() || !live.restoreAvailable() {
		t.Fatal("both routes would work")
	}
}
