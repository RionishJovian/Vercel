// Command legacy-migration-audit inspects the legacy MikroMagic database and
// reports whether its core tenant records can be imported into Mega Magic.
// It is deliberately read-only; it never writes either database.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"

	_ "github.com/lib/pq"
)

type report struct {
	LegacyTenants int      `json:"legacy_tenants"`
	LegacyUsers   int      `json:"legacy_users"`
	LegacySites   int      `json:"legacy_sites"`
	RoleCounts    []string `json:"role_counts"`
	Warnings      []string `json:"warnings"`
	Ready         bool     `json:"ready"`
}

func main() {
	dsn := flag.String("legacy-dsn", os.Getenv("LEGACY_DSN"), "legacy PostgreSQL DSN (or LEGACY_DSN)")
	flag.Parse()
	if *dsn == "" {
		fail("legacy-dsn is required; this command is read-only")
	}
	db, err := sql.Open("postgres", *dsn)
	if err != nil {
		fail("open legacy database: %v", err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		fail("ping legacy database: %v", err)
	}

	r := report{Ready: true}
	for _, q := range []struct {
		name string
		dst  *int
	}{
		{"tenants", &r.LegacyTenants},
		{"users", &r.LegacyUsers},
		{"sites", &r.LegacySites},
	} {
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+q.name).Scan(q.dst); err != nil {
			fail("count %s: %v", q.name, err)
		}
	}
	rows, err := db.QueryContext(ctx, "SELECT coalesce(role, '<null>'), count(*) FROM users GROUP BY role ORDER BY role")
	if err != nil {
		fail("count roles: %v", err)
	}
	for rows.Next() {
		var role string
		var count int
		if err := rows.Scan(&role, &count); err != nil {
			fail("scan role count: %v", err)
		}
		r.RoleCounts = append(r.RoleCounts, fmt.Sprintf("%s=%d", role, count))
		if role != "sme" && role != "guest" && role != "platform" {
			r.Warnings = append(r.Warnings, "unmapped legacy role: "+role)
		}
	}
	if err := rows.Err(); err != nil {
		fail("iterate role counts: %v", err)
	}
	// Platform users have no tenant in the legacy schema and need an explicit
	// platform-account policy before import; do not silently assign a tenant.
	var platformWithoutTenant int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM users WHERE role='platform' AND nullif(btrim(tenant_id),'') IS NULL").Scan(&platformWithoutTenant); err != nil {
		fail("check platform users: %v", err)
	}
	if platformWithoutTenant > 0 {
		r.Warnings = append(r.Warnings, fmt.Sprintf("%d platform user(s) have no tenant and require explicit platform-account mapping", platformWithoutTenant))
	}
	// The Go schema requires a unique owner_email. Legacy usernames are allowed
	// to be TLD-less, but a tenant still needs one deterministic owner record.
	var ownerless int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM tenants t WHERE NOT EXISTS (
		SELECT 1 FROM users u WHERE u.tenant_id=t.id AND u.active AND u.role IN ('sme','platform')
	)`).Scan(&ownerless); err != nil {
		fail("check owner coverage: %v", err)
	}
	if ownerless > 0 {
		r.Warnings = append(r.Warnings, fmt.Sprintf("%d tenant(s) have no active legacy owner candidate", ownerless))
	}
	sort.Strings(r.Warnings)
	r.Ready = len(r.Warnings) == 0
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		fail("encode report: %v", err)
	}
	if !r.Ready {
		os.Exit(2)
	}
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "legacy migration audit: "+format+"\n", args...)
	os.Exit(1)
}
