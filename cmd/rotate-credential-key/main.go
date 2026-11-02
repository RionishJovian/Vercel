// Command rotate-credential-key replaces the applier credential key.
//
// The key seals every router login the app stores and the short-lived Hub
// enrollments. This tool re-seals all of them from the CURRENT key (read from
// the same configuration the API uses) to a NEW key given in the environment
// variable MM_NEW_CREDENTIAL_KEY (never a flag, so it stays out of the process
// list). It is a dry run unless -apply is passed, never prints key material or
// plaintext, and changes nothing if any value cannot be opened and re-verified.
//
// Stop the API before -apply and start it again with the new key; see
// docs/SECRET-ROTATION.md.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"mega-magic/internal/config"
	"mega-magic/internal/database"
	"mega-magic/internal/secretbox"
)

func main() {
	configPath := flag.String("config", "config.yaml", "configuration file (the same one the API uses)")
	apply := flag.Bool("apply", false, "commit the rotation; without it the run is verified and rolled back")
	timeout := flag.Duration("timeout", 2*time.Minute, "overall time limit")
	flag.Parse()
	if err := run(*configPath, *apply, *timeout); err != nil {
		fmt.Fprintln(os.Stderr, "rotate-credential-key:", err)
		os.Exit(1)
	}
}

func run(configPath string, apply bool, timeout time.Duration) error {
	newKey := strings.TrimSpace(os.Getenv("MM_NEW_CREDENTIAL_KEY"))
	if newKey == "" {
		return fmt.Errorf("set MM_NEW_CREDENTIAL_KEY to the new 32-byte key (base64 or hex)")
	}
	newBox, err := secretbox.New(newKey)
	if err != nil {
		return fmt.Errorf("MM_NEW_CREDENTIAL_KEY is not a valid key: %w", err)
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	oldKey := strings.TrimSpace(cfg.Provisioner.Applier.CredentialKey)
	if posture := strings.TrimSpace(cfg.Provisioner.PostureCapture.CredentialKey); posture != "" && posture != oldKey {
		return fmt.Errorf("posture capture has its own credential key; rotate it separately before using this tool")
	}
	oldBox, err := secretbox.New(oldKey)
	if err != nil {
		return fmt.Errorf("current credential key: %w", err)
	}
	db, err := database.New(cfg.Database)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	report, err := db.RotateCredentialKey(ctx, oldBox, newBox, apply, time.Now())
	if err != nil {
		return err
	}
	mode := "DRY RUN (verified, rolled back; nothing changed)"
	if report.Applied {
		mode = "APPLIED"
	}
	fmt.Printf("%s\n  device logins re-sealed:        %d\n  pending enrollments re-sealed:  %d\n  telegram bot tokens re-sealed:  %d\n  webhook secrets re-sealed:      %d\n  expired enrollments deleted:    %d\n  values verified under new key:  %d\n",
		mode, report.DeviceSecrets, report.PendingEnrollments, report.BotTokens, report.WebhookSecrets, report.ExpiredEnrollments, report.VerifiedUnderNewKey)
	if report.Applied {
		fmt.Println("Now put the new key in the API environment and start the API. The old key no longer opens any stored value.")
	}
	return nil
}
