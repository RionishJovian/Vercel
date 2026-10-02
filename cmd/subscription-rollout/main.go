// Explicit, additive subscription rollout. Never invoked by application startup.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"mega-magic/internal/config"
	"mega-magic/internal/database"
)

func main() {
	action := flag.String("action", "check", "check, backup or apply")
	path := flag.String("config", "config.yaml", "protected configuration")
	backup := flag.String("backup", "", "absolute path for a new database backup")
	flag.Parse()
	cfg, err := config.Load(*path)
	if err != nil {
		fail("configuration unavailable")
	}
	db, err := database.New(cfg.Database)
	if err != nil {
		fail("database unavailable")
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	switch *action {
	case "backup":
		if !filepath.IsAbs(*backup) {
			fail("absolute backup path required")
		}
		f, e := os.OpenFile(*backup, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			fail("cannot create new protected backup")
		}
		defer f.Close()
		cmd := exec.CommandContext(ctx, "pg_dump", "--format=custom", "--host="+cfg.Database.Host, "--port="+strconv.Itoa(cfg.Database.Port), "--username="+cfg.Database.User, "--dbname="+cfg.Database.Name)
		for _, v := range os.Environ() {
			if !strings.HasPrefix(v, "PGPASSWORD=") && !strings.HasPrefix(v, "PGSSLMODE=") {
				cmd.Env = append(cmd.Env, v)
			}
		}
		cmd.Env = append(cmd.Env, "PGPASSWORD="+cfg.Database.Password, "PGSSLMODE="+cfg.Database.SSLMode)
		cmd.Stdout = f
		if cmd.Run() != nil {
			fail("backup failed; do not apply migration")
		}
		if f.Sync() != nil {
			fail("backup sync failed")
		}
		fmt.Println("protected database backup completed")
	case "apply":
		f, e := os.Open(*backup)
		if e != nil {
			fail("completed backup required")
		}
		var magic [5]byte
		_, e = f.Read(magic[:])
		f.Close()
		if e != nil || string(magic[:]) != "PGDMP" {
			fail("valid database backup required")
		}
		var bypass bool
		if db.QueryRowContext(ctx, `SELECT rolsuper OR rolbypassrls FROM pg_roles WHERE rolname=current_user`).Scan(&bypass) != nil || !bypass {
			fail("explicit migration role must bypass RLS for the one-time transition grant")
		}
		body, e := os.ReadFile("migrations/20261001160000_subscription_trials.sql")
		if e != nil {
			fail("migration unavailable")
		}
		if _, e = db.ExecContext(ctx, string(body)); e != nil {
			fail("migration failed and rolled back")
		}
		if database.CheckSubscriptionSchema(db) != nil {
			fail("subscription schema check failed")
		}
		fmt.Println("subscription migration applied; existing trials and paid expiry preserved")
	case "check":
		if database.CheckSubscriptionSchema(db) != nil {
			fail("subscription schema unavailable")
		}
		fmt.Println("subscription schema check passed")
	default:
		fail("unknown action")
	}
}

func fail(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }
