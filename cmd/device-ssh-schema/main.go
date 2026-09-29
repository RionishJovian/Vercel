// Command device-ssh-schema provides the controlled two-stage production
// rollout for per-device SSH columns and constraints.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"mega-magic/internal/config"
	"mega-magic/internal/database"
)

func main() {
	action := flag.String("action", "check", "schema action: check, apply, or validate")
	configPath := flag.String("config", "config.yaml", "configuration file")
	migrationPath := flag.String("migration", "migrations/20260929120000_device_ssh_host_key_pinning.sql", "migration SQL used by action=apply")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fatal("configuration invalid: %v", err)
	}
	db, err := database.New(cfg.Database)
	if err != nil {
		fatal("database connection failed: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	switch *action {
	case "check":
		err = database.CheckDeviceSSHSchema(ctx, db.DB)
	case "apply":
		var migration []byte
		migration, err = os.ReadFile(*migrationPath)
		if err == nil {
			_, err = db.ExecContext(ctx, string(migration))
		}
		if err == nil {
			err = database.CheckDeviceSSHStageOne(ctx, db.DB)
		}
	case "validate":
		err = database.ValidateDeviceSSHConstraints(ctx, db.DB)
	default:
		fatal("unknown action %q (want check, apply, or validate)", *action)
	}
	if err != nil {
		fatal("device SSH schema %s failed: %v", *action, err)
	}
	fmt.Printf("device SSH schema %s passed\n", *action)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
