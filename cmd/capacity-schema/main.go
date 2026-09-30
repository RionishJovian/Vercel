// Command capacity-schema applies/checks the additive capacity-request schema.
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
	action := flag.String("action", "check", "schema action: check or apply")
	configPath := flag.String("config", "config.yaml", "configuration file")
	migrationPath := flag.String("migration", "migrations/20260930110000_capacity_request_quantities.sql", "migration SQL")
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
	if *action == "apply" {
		migration, readErr := os.ReadFile(*migrationPath)
		if readErr != nil {
			fatal("read migration: %v", readErr)
		}
		if _, err = db.ExecContext(ctx, string(migration)); err != nil {
			fatal("apply migration: %v", err)
		}
	} else if *action != "check" {
		fatal("unknown action %q (want check or apply)", *action)
	}

	var quantityOK, routerDefaultOK bool
	err = db.QueryRowContext(ctx, `SELECT
		EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='device_requests' AND column_name='quantity' AND is_nullable='NO'),
		EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='device_allowances' AND column_name='routers' AND column_default='1')`).
		Scan(&quantityOK, &routerDefaultOK)
	if err != nil {
		fatal("capacity schema check failed: %v", err)
	}
	if !quantityOK || !routerDefaultOK {
		fatal("capacity schema incomplete (quantity=%t router_default=%t)", quantityOK, routerDefaultOK)
	}
	fmt.Printf("capacity schema %s passed\n", *action)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
