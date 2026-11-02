package routeros

import (
	"context"
	"errors"
	"fmt"

	"mega-magic/internal/model"
)

// Speed tiers for purchased guest plans. A speed is a Hotspot user profile carrying a rate limit; the
// guest's user is switched to it when the owner approves the purchase. RouterOS writes a rate limit as
// "rx/tx" from the router's side, so the client's UPLOAD comes first and the DOWNLOAD second.
// The profile is created only when missing, and an existing profile of the same name is adopted only if
// Mega Magic created it with exactly these limits; anything else is refused and left untouched.
const guestSpeedMarker = "mega-magic:speed"

func guestSpeedRate(downMbps, upMbps int) string { return fmt.Sprintf("%dM/%dM", upMbps, downMbps) }

func guestSpeedProfileName(downMbps, upMbps int) string {
	return fmt.Sprintf("mm-speed-u%d-d%d", upMbps, downMbps)
}

func ensureGuestSpeedProfile(ctx context.Context, r CommandRunner, downMbps, upMbps int) (string, error) {
	if downMbps == 0 || upMbps == 0 || !model.ValidGuestSpeed(downMbps, upMbps) {
		return "", fmt.Errorf("invalid guest speed")
	}
	name, rate := guestSpeedProfileName(downMbps, upMbps), guestSpeedRate(downMbps, upMbps)
	add := "name=" + routerOSQuoted(name) + " rate-limit=" + rate + " shared-users=1 comment=" + routerOSQuoted(guestSpeedMarker)
	ours := func(v map[string]string) bool {
		return v["name"] == name && v["comment"] == guestSpeedMarker && networkEqual("rate-limit", v["rate-limit"], rate) &&
			v["shared-users"] == "1" && v["on-login"] == "" && v["on-logout"] == ""
	}
	if err := ensureOrAdoptGuardObject(ctx, r, "/ip hotspot user profile", name, add, ours); err != nil {
		if errors.Is(err, errVoucherGuardConflict) {
			return "", fmt.Errorf("a Hotspot profile named %s already exists but was not created by Mega Magic or has other limits; nothing was changed", name)
		}
		return "", fmt.Errorf("the speed profile could not be prepared on the router; nothing was changed")
	}
	return name, nil
}
