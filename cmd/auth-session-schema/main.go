package main

import (
	"flag"
	"fmt"
	"mega-magic/internal/config"
	"mega-magic/internal/database"
	"os"
)

func main() {
	action := flag.String("action", "check", "check or apply")
	path := flag.String("config", "config.yaml", "configuration path")
	flag.Parse()
	if err := run(*action, *path); err != nil {
		fmt.Fprintln(os.Stderr, "authentication schema check/apply failed; inspect configuration and migration")
		os.Exit(1)
	}
	fmt.Println("Authentication session schema ready")
}
func run(action, path string) error {
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	db, err := database.New(cfg.Database)
	if err != nil {
		return err
	}
	defer db.Close()
	if action == "apply" {
		data, err := os.ReadFile("migrations/20260930220000_auth_sessions.sql")
		if err != nil {
			return err
		}
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err = tx.Exec(string(data)); err != nil {
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	} else if action != "check" {
		return fmt.Errorf("unknown action")
	}
	return database.CheckAuthSessionSchema(db)
}
