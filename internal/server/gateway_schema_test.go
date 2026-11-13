package server

import (
	"os"
	"testing"

	"mega-magic/internal/database"
)

// applyGatewayCapacitySchema loads the 2026-11-13 gateway capacity migration into
// a fixture database built from database/init.sql (which predates the Store
// tables), so tests exercise the same capacity function and trigger as production.
func applyGatewayCapacitySchema(t *testing.T, db *database.DB) {
	t.Helper()
	var present bool
	if err := db.QueryRow(`SELECT to_regclass('public.store_addon_requests') IS NOT NULL`).Scan(&present); err != nil {
		t.Fatal(err)
	}
	files := []string{}
	if !present {
		files = append(files, "../../migrations/20261031100000_store_addons.sql")
	}
	files = append(files, "../../db/migrations/20261113120000_gateway_capacity.sql")
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(string(raw)); err != nil {
			t.Fatalf("load %s: %v", f, err)
		}
	}
}
