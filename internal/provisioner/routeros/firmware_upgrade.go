package routeros

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Board firmware (RouterBOARD bootloader) upgrade. It is the one change here that cannot be
// undone from the app and that restarts the router, so it is a typed, previewed, confirmed
// operation: the preview shows the exact versions and warnings, apply re-reads the router and
// refuses if anything differs from what was reviewed, takes the usual export and backup first,
// writes the firmware and then schedules the restart. Success is not claimed here: the router
// is gone while it restarts, so the caller verifies afterwards that it reports the new firmware.
const (
	firmwareUpgradeCommand = `:do { /system routerboard upgrade } on-error={ :error "firmware upgrade refused" }`
	firmwareRebootCommand  = `:put [:execute { :delay 3s; /system reboot }]`
)

type FirmwareReview struct {
	Confirmation string    `json:"confirmation"`
	Model        string    `json:"model,omitempty"`
	Current      string    `json:"current"`
	Target       string    `json:"target"`
	Steps        []string  `json:"steps"`
	Warnings     []string  `json:"warnings"`
	ExpiresAt    time.Time `json:"expires_at"`
}
type FirmwareResult struct {
	Backup     string `json:"backup"`
	Current    string `json:"current"`
	Target     string `json:"target"`
	Restarting bool   `json:"restarting"`
}

// firmwareState reads the firmware facts fresh from the router (never from the retained telemetry)
// and refuses anything it cannot prove is a real, newer firmware bundled with the installed RouterOS.
func firmwareState(ctx context.Context, r CommandRunner) (*FirmwareStatus, error) {
	out, err := r.Run(ctx, TelemetryFirmwareCommand)
	if err != nil {
		return nil, fmt.Errorf("could not read the board firmware safely; nothing changed")
	}
	f := parseFirmware(out)
	if f == nil || f.CurrentFirmware == "" || f.UpgradeFirmware == "" {
		return nil, fmt.Errorf("could not read the board firmware safely; nothing changed")
	}
	if _, ok := versionTuple(f.CurrentFirmware); !ok {
		return nil, fmt.Errorf("could not read the board firmware safely; nothing changed")
	}
	if !f.FirmwareUpgradeable {
		return nil, fmt.Errorf("the board firmware is already up to date; nothing to upgrade")
	}
	// The firmware shipped with a RouterOS version can never be newer than that RouterOS.
	if f.InstalledVersion != "" && versionNewer(f.UpgradeFirmware, f.InstalledVersion) {
		return nil, fmt.Errorf("the offered firmware is newer than the installed RouterOS, which is not expected; nothing changed")
	}
	return f, nil
}

func firmwareRequest(f *FirmwareStatus) string {
	return networkHash([]string{"firmware-upgrade", f.CurrentFirmware, f.UpgradeFirmware})
}

func (a *DeviceApplier) PreviewFirmware(ctx context.Context, businessID, deviceID string) (FirmwareReview, error) {
	r, err := runnerForDevice(ctx, a.src, a.factory, a.timeout, businessID, deviceID)
	if err != nil {
		return FirmwareReview{}, err
	}
	if _, pending := networkPending.Load(businessID + ":" + deviceID); pending {
		return FirmwareReview{}, fmt.Errorf("a network change is waiting to be kept or reverted; finish it before upgrading firmware")
	}
	f, err := firmwareState(ctx, r)
	if err != nil {
		return FirmwareReview{}, err
	}
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return FirmwareReview{}, fmt.Errorf("preview unavailable")
	}
	token := hex.EncodeToString(raw)
	expiry := time.Now().Add(5 * time.Minute)
	networkApprovals.Lock()
	defer networkApprovals.Unlock()
	for key, entry := range networkApprovals.entries {
		if time.Now().After(entry.Expires) {
			delete(networkApprovals.entries, key)
		}
	}
	if len(networkApprovals.entries) >= 512 {
		return FirmwareReview{}, fmt.Errorf("too many active previews; retry shortly")
	}
	networkApprovals.entries[token] = networkApproval{businessID, deviceID, firmwareRequest(f), networkHash(*f), expiry}
	return FirmwareReview{
		Confirmation: token, Model: f.Model, Current: f.CurrentFirmware, Target: f.UpgradeFirmware, ExpiresAt: expiry,
		Steps: []string{
			"Save a backup and a full export on the router",
			"Write the board firmware " + f.CurrentFirmware + " → " + f.UpgradeFirmware,
			"Restart the router to start the new firmware",
		},
		Warnings: []string{
			"The router restarts. Everything connected to it, including guest internet, is offline for about one to three minutes.",
			"Do not switch the router off or unplug it while it restarts: a power cut while the firmware is written can leave it unable to start.",
			"This cannot be undone from the app. Going back to an older firmware needs a manual downgrade in WinBox.",
			"Choose a quiet time. The private Magic Hub connection comes back by itself; if it has not returned after ten minutes, check the router's power and cable.",
		},
	}, nil
}

func (a *DeviceApplier) ApplyFirmware(ctx context.Context, businessID, deviceID, confirmation string) (FirmwareResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	result := FirmwareResult{}
	networkApprovals.Lock()
	approved, ok := networkApprovals.entries[confirmation]
	delete(networkApprovals.entries, confirmation)
	networkApprovals.Unlock()
	if !ok || approved.Business != businessID || approved.Device != deviceID || time.Now().After(approved.Expires) {
		return result, fmt.Errorf("preview expired or changed; review again before upgrading")
	}
	lock, _ := gatewayLocks.LoadOrStore(businessID+":"+deviceID, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	guardKey := businessID + ":" + deviceID
	if until, ok := networkGuardUntil.Load(guardKey); ok && time.Now().Before(until.(time.Time)) {
		return result, fmt.Errorf("a previous router restoration timer may still be active; wait three minutes and reload before upgrading")
	}
	if _, pending := networkPending.Load(guardKey); pending {
		return result, fmt.Errorf("a network change is waiting to be kept or reverted; finish it before upgrading firmware")
	}
	r, err := runnerForDevice(ctx, a.src, a.factory, a.timeout, businessID, deviceID)
	if err != nil {
		return result, err
	}
	f, err := firmwareState(ctx, r)
	if err != nil {
		return result, err
	}
	if networkHash(*f) != approved.StateHash || firmwareRequest(f) != approved.RequestHash {
		return result, fmt.Errorf("the router's firmware state changed since the preview; reload and review again")
	}
	jobs, e := networkCount(ctx, r, "/system script job print count-only")
	// RouterOS includes the current SSH script itself in this count.
	if e != nil || jobs != 1 {
		return result, fmt.Errorf("router has an active script job; wait for it to finish before upgrading firmware")
	}
	result.Current, result.Target = f.CurrentFirmware, f.UpgradeFirmware
	result.Backup = snapshotName(deviceID+"-firmware", time.Now().UTC())
	for _, command := range []string{"/export file=" + result.Backup, "/system backup save name=" + result.Backup} {
		if _, err = r.Run(ctx, command); err != nil {
			return result, fmt.Errorf("backup failed; the firmware was not changed")
		}
	}
	if _, err = r.Run(ctx, firmwareUpgradeCommand); err != nil {
		return result, fmt.Errorf("the router refused the firmware upgrade; it was not restarted")
	}
	a.extras.forgetFirmware(businessID + "/" + deviceID)
	out, err := r.Run(ctx, firmwareRebootCommand)
	if err != nil || !networkIDPattern.MatchString(strings.TrimSpace(out)) {
		return result, fmt.Errorf("the firmware was written but the restart could not be scheduled; restart the router yourself to finish")
	}
	result.Restarting = true
	return result, nil
}
