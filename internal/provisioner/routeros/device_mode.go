package routeros

import (
	"context"
	"fmt"
	"strings"
)

const deviceModeCommand = "/system device-mode print"

// DeviceModeFacts reports what the router's device mode allows. Factory modes can
// turn off features the app depends on: `home` blocks Hotspot and Scheduler, `basic`
// blocks Hotspot. Known is false when the router could not report it, for example a
// RouterOS version without device mode.
type DeviceModeFacts struct {
	Known     bool   `json:"known"`
	Mode      string `json:"mode,omitempty"`
	Hotspot   bool   `json:"hotspot"`
	Scheduler bool   `json:"scheduler"`
}

func parseDeviceMode(out string) DeviceModeFacts {
	fields := parseValueList(out)
	mode := fields["mode"]
	switch mode {
	case "advanced", "basic", "home", "rose", "enterprise":
	default:
		return DeviceModeFacts{}
	}
	// An explicit flag wins over the mode's default: owners can switch one feature.
	allowed := func(key string, byDefault bool) bool {
		switch fields[key] {
		case "yes", "true":
			return true
		case "no", "false":
			return false
		}
		return byDefault
	}
	return DeviceModeFacts{
		Known:     true,
		Mode:      mode,
		Hotspot:   allowed("hotspot", mode != "home" && mode != "basic"),
		Scheduler: allowed("scheduler", mode != "home"),
	}
}

func readDeviceMode(ctx context.Context, r CommandRunner) DeviceModeFacts {
	out, err := r.Run(ctx, deviceModeCommand)
	if err != nil {
		return DeviceModeFacts{}
	}
	return parseDeviceMode(out)
}

// requireDeviceFeatures stops a setup before it changes anything when device mode
// would refuse part of it. An unknown mode does not block: the router's own refusal
// still stops the step that needs the feature. The app cannot change device mode:
// RouterOS requires someone at the router.
func requireDeviceFeatures(ctx context.Context, r CommandRunner, hotspot, scheduler bool) error {
	facts := readDeviceMode(ctx, r)
	if !facts.Known {
		return nil
	}
	var names, settings []string
	if hotspot && !facts.Hotspot {
		names, settings = append(names, "Hotspot"), append(settings, "hotspot=yes")
	}
	if scheduler && !facts.Scheduler {
		names, settings = append(names, "Scheduler"), append(settings, "scheduler=yes")
	}
	if len(names) == 0 {
		return nil
	}
	return fmt.Errorf("this router's device mode (%s) turns off %s, so nothing was changed. In WinBox New Terminal run /system/device-mode/update %s, then within 5 minutes press the router's reset or mode button, or unplug its power. A normal reboot cancels the change, and the router restarts once confirmed",
		facts.Mode, strings.Join(names, " and "), strings.Join(settings, " "))
}
