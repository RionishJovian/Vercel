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
	captureOnly := &BackupHandler{}
	captureOnly.EnableCapture(struct{ provisioner.Applier }{})
	if !captureOnly.captureAvailable() || captureOnly.restoreAvailable() {
		t.Fatal("capture-only must not grant live restore")
	}
	captureOnly.EnableLive(struct{ provisioner.Applier }{}, nil)
	if !captureOnly.captureAvailable() || captureOnly.restoreAvailable() {
		t.Fatal("missing OTP must fail closed even with full applier")
	}
}
