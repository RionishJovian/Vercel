// guest-router-check performs fixed, read-only CLI compatibility checks on the
// explicitly named CHR lab device. It never prints credentials or user records.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"mega-magic/internal/config"
	"mega-magic/internal/database"
	"mega-magic/internal/provisioner/routeros"
	"mega-magic/internal/secretbox"
)

func main() {
	path := flag.String("config", "config.yaml", "configuration path")
	device := flag.String("device", "", "existing CHR lab device UUID")
	flag.Parse()
	if err := run(*path, *device); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(path, device string) error {
	if device == "" {
		return fmt.Errorf("explicit lab device required")
	}
	cfg, err := config.Load(path)
	if err != nil {
		return fmt.Errorf("configuration unavailable")
	}
	db, err := database.New(cfg.Database)
	if err != nil {
		return fmt.Errorf("database unavailable")
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	var business, name string
	if err := db.QueryRowContext(ctx, "SELECT business_id, hardware_name FROM devices WHERE device_id=$1", device).Scan(&business, &name); err != nil || !strings.HasPrefix(name, "CHR lab router") {
		return fmt.Errorf("target is not the expected CHR lab device")
	}
	box, err := secretbox.New(cfg.Provisioner.Applier.CredentialKey)
	if err != nil {
		return fmt.Errorf("credential key unavailable")
	}
	target, err := database.NewDeviceCredentialSource(db, box).TargetForDevice(ctx, business, device)
	if err != nil {
		return fmt.Errorf("lab target unavailable")
	}
	sshCfg, err := routeros.SSHConfigFromTarget(target, 10*time.Second)
	if err != nil {
		return fmt.Errorf("pinned lab transport unavailable")
	}
	runner := routeros.NewSSHTransport(sshCfg)
	checks := []struct{ name, command string }{
		{"epoch clock", ":put [:tonsec [:timestamp]]"},
		{"user counter get syntax", `:local ids [/ip hotspot user find]; :if ([:len $ids] = 0) do={ :put "NO-USERS" } else={ :local id [:pick $ids 0]; :put ("bytes-in: " . [/ip hotspot user get $id bytes-in]); :put ("bytes-out: " . [/ip hotspot user get $id bytes-out]) }`},
		{"active session count syntax", `/ip hotspot active print count-only where user="mm-read-only-probe-no-user"`},
		{"cookie count syntax", `/ip hotspot cookie print count-only where user="mm-read-only-probe-no-user"`},
		{"NTP synchronization status", `:put [/system ntp client get enabled]; :put [/system ntp client get status]`},
	}
	var clockErr error
	for _, check := range checks {
		before := time.Now()
		out, err := runner.Run(ctx, check.command)
		if err != nil {
			return fmt.Errorf("%s: failed (router output withheld)", check.name)
		}
		if check.name == "epoch clock" {
			ns, parseErr := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
			at := time.Unix(0, ns)
			if parseErr != nil {
				clockErr = fmt.Errorf("epoch clock: non-integer timestamp output")
			}
			if at.Before(before.Add(-5*time.Second)) || at.After(time.Now().Add(5*time.Second)) {
				if clockErr == nil {
					clockErr = fmt.Errorf("epoch clock: skew %.0f seconds (access must remain blocked)", at.Sub(before).Seconds())
				}
			}
			if clockErr != nil {
				fmt.Println("FAIL:", clockErr)
				continue
			}
		}
		if strings.Contains(check.name, "count syntax") && strings.TrimSpace(out) != "0" {
			return fmt.Errorf("%s: unexpected count response", check.name)
		}
		if check.name == "user counter get syntax" && (!strings.Contains(out, "bytes-in:") || !strings.Contains(out, "bytes-out:")) {
			return fmt.Errorf("user counters not verified; an existing lab user is required")
		}
		if check.name == "NTP synchronization status" {
			fields := strings.Fields(out)
			enabled, synchronized := false, false
			for _, field := range fields {
				enabled = enabled || field == "true" || field == "yes"
				synchronized = synchronized || field == "synchronized"
			}
			fmt.Printf("NTP: enabled=%t synchronized=%t\n", enabled, synchronized)
			continue
		}
		fmt.Printf("PASS: %s\n", check.name)
	}
	fmt.Println("Read-only compatibility checks only; traffic, restart, recovery and expiry acceptance remain untested.")
	return clockErr
}
