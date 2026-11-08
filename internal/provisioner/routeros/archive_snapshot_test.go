package routeros

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"mega-magic/internal/secretbox"
)

type archiveTestRunner struct {
	mismatch bool
}

func (r *archiveTestRunner) Run(_ context.Context, cmd string) (string, error) {
	switch {
	case strings.Contains(cmd, "/file get") && strings.Contains(cmd, ".backup"):
		return "5\n", nil
	case strings.Contains(cmd, "/file get") && strings.Contains(cmd, ".rsc"):
		return "4\n", nil
	case strings.Contains(cmd, "/interface wireguard"):
		return "private-key: SHOULD-NOT-LEAK\n", nil
	default:
		return "", nil
	}
}
func (r *archiveTestRunner) DownloadFile(_ context.Context, name string, limit int64) ([]byte, error) {
	if limit <= 0 { return nil, errors.New("bad limit") }
	if strings.HasSuffix(name, ".backup") {
		if r.mismatch { return []byte("bad"), nil }
		return []byte("abcde"), nil
	}
	if strings.HasSuffix(name, ".rsc") { return []byte("wxyz"), nil }
	return nil, errors.New("unexpected file")
}

func TestArchiveSnapshotEncryptsAndVerifiesOffRouterCopies(t *testing.T) {
	const artifactKey = "AgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgI="
	box, err := secretbox.New(artifactKey)
	if err != nil { t.Fatal(err) }
	src := newFakeSource()
	src.put("biz-1", "device-1", pwTarget("device-1", "10.77.0.15:22", "admin", "unused-test-password"))
	archiver := NewDeviceApplier(src, time.Second,
		func(SSHConfig) CommandRunner { return &archiveTestRunner{} }, "wg-magichub", 0, 0).
		WithBackupPassword("Separate-backup-password-2026!").WithArchiveBox(box)
	snap, err := archiver.ArchiveSnapshot(context.Background(), "biz-1", "device-1")
	if err != nil { t.Fatal(err) }
	if snap.BackupBytes != 5 || snap.ExportBytes != 4 || len(snap.OffDeviceBackupSHA256) != 64 || len(snap.OffDeviceExportSHA256) != 64 {
		t.Fatalf("off-router artifact size/digest evidence missing: %+v", snap)
	}
	if plaintext, err := box.Open(snap.OffDeviceBackupSealed); err != nil || plaintext != "abcde" {
		t.Fatal("encrypted backup did not round-trip")
	}
	if plaintext, err := box.Open(snap.OffDeviceExportSealed); err != nil || plaintext != "wxyz" {
		t.Fatal("encrypted export did not round-trip")
	}
	raw, _ := json.Marshal(snap)
	for _, forbidden := range []string{"SHOULD-NOT-LEAK", "Separate-backup-password-2026!", "unused-test-password"} {
		if strings.Contains(string(raw), forbidden) { t.Fatalf("archive leaked sensitive value") }
	}
	if _, err := archiver.ArchiveSnapshot(context.Background(), "foreign-business", "device-1"); err == nil {
		t.Fatal("cross-tenant backup must fail before any SSH transport")
	}
}

func TestArchiveSnapshotFailsClosedOnTransferredSizeMismatch(t *testing.T) {
	box, err := secretbox.New("AgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgI=")
	if err != nil { t.Fatal(err) }
	src := newFakeSource()
	src.put("biz-1", "device-1", pwTarget("device-1", "10.77.0.15:22", "admin", "unused"))
	a := NewDeviceApplier(src, time.Second, func(SSHConfig) CommandRunner {
		return &archiveTestRunner{mismatch: true}
	}, "wg-magichub", 0, 0).WithBackupPassword("Separate-backup-password-2026!").WithArchiveBox(box)
	snap, err := a.ArchiveSnapshot(context.Background(), "biz-1", "device-1")
	if err == nil || snap.OffDeviceBackupSealed != "" {
		t.Fatalf("size mismatch must produce no archive record: %v", err)
	}
}
