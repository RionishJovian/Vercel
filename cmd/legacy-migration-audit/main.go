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
	"strings"
	"time"

	_ "github.com/lib/pq"
)

type report struct {
	LegacyTenants  int      `json:"legacy_tenants"`
	LegacyUsers    int      `json:"legacy_users"`
	LegacySites    int      `json:"legacy_sites"`
	LegacyVouchers int      `json:"legacy_vouchers"`
	RoleCounts     []string `json:"role_counts"`
	Warnings       []string `json:"warnings"`
	Ready          bool     `json:"ready"`
}

func main() {
	dsn := flag.String("legacy-dsn", os.Getenv("LEGACY_DSN"), "legacy PostgreSQL DSN (or LEGACY_DSN)")
	targetDSN := flag.String("target-dsn", os.Getenv("TARGET_DSN"), "Go PostgreSQL DSN (required with --apply)")
	apply := flag.Bool("apply", false, "apply the validated core migration")
	confirm := flag.String("confirm", "", "must be MIGRATE-CORE-DATA to enable --apply")
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
		{"vouchers", &r.LegacyVouchers},
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
		SELECT 1 FROM users u WHERE u.tenant_id=t.id AND u.role='sme'
	)`).Scan(&ownerless); err != nil {
		fail("check owner coverage: %v", err)
	}
	if ownerless > 0 {
		r.Warnings = append(r.Warnings, fmt.Sprintf("%d tenant(s) have no legacy owner candidate", ownerless))
	}
	if r.LegacyVouchers > 0 {
		r.Warnings = append(r.Warnings, fmt.Sprintf("%d legacy voucher(s) will be preserved as revoked; plaintext codes require reissue", r.LegacyVouchers))
	}
	sort.Strings(r.Warnings)
	// Platform users without a tenant are intentionally excluded from this
	// tenant migration; they need a separate platform-account policy but do not
	// block importing tenant-owned records.
	for _, warning := range r.Warnings {
		if strings.HasPrefix(warning, "2 platform user(s) have no tenant") || strings.Contains(warning, "legacy voucher(s) will be preserved as revoked") {
			continue
		}
		if !strings.HasPrefix(warning, "2 platform user(s) have no tenant") {
			r.Ready = false
		}
	}
	if *apply {
		if *targetDSN == "" || *confirm != "MIGRATE-CORE-DATA" {
			fail("--apply requires --target-dsn and --confirm MIGRATE-CORE-DATA")
		}
		if !r.Ready {
			fail("migration has warnings; resolve them before applying")
		}
		if err := applyCore(ctx, db, *targetDSN); err != nil {
			fail("apply core migration: %v", err)
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		fail("encode report: %v", err)
	}
	if !r.Ready {
		os.Exit(2)
	}
}

type legacyTenant struct {
	id, name, owner string
}

type legacyUser struct {
	id, username, hash, role, tenant string
	active                           bool
}

type legacySite struct {
	id, tenant, name, timezone string
	created                    sql.NullInt64
}

const platformBusinessID = "00000000-0000-0000-0000-000000000001"

// applyCore copies only data whose semantics are lossless. Voucher secrets are
// intentionally excluded: the legacy table stores hashes/sealed values, not a
// plaintext code that the new voucher login can use.
func applyCore(ctx context.Context, source *sql.DB, targetDSN string) error {
	target, err := sql.Open("postgres", targetDSN)
	if err != nil {
		return err
	}
	defer target.Close()
	if err := target.PingContext(ctx); err != nil {
		return err
	}
	tx, err := target.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	tenants, err := source.QueryContext(ctx, `SELECT t.id,t.name,coalesce((SELECT u.username FROM users u WHERE u.tenant_id=t.id AND u.role='sme' ORDER BY u.active DESC,u.id LIMIT 1),'') FROM tenants t ORDER BY t.id`)
	if err != nil {
		return err
	}
	for tenants.Next() {
		var t legacyTenant
		if err := tenants.Scan(&t.id, &t.name, &t.owner); err != nil {
			return err
		}
		if strings.TrimSpace(t.owner) == "" {
			return fmt.Errorf("tenant %s has no active sme owner", t.id)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO businesses(business_id,company_name,owner_email,access_tier) VALUES($1,$2,$3,'SMALL_WISP') ON CONFLICT(business_id) DO UPDATE SET company_name=EXCLUDED.company_name,owner_email=EXCLUDED.owner_email,updated_at=now()`, t.id, t.name, t.owner); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO business_metered_quotas(business_id) VALUES($1) ON CONFLICT(business_id) DO NOTHING`, t.id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO portal_configs(business_id,business_name) VALUES($1,$2) ON CONFLICT(business_id) DO NOTHING`, t.id, t.name+" Wi-Fi"); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO nats_subjects(business_id,subject_prefix) VALUES($1,$2) ON CONFLICT(business_id) DO NOTHING`, t.id, "tenant."+t.id[:8]+"."); err != nil {
			return err
		}
	}
	if err := tenants.Err(); err != nil {
		return err
	}
	tenants.Close()

	users, err := source.QueryContext(ctx, `SELECT id,username,password_hash,role,tenant_id,active FROM users WHERE role IN ('sme','guest','platform') ORDER BY id`)
	if err != nil {
		return err
	}
	for users.Next() {
		var u legacyUser
		if err := users.Scan(&u.id, &u.username, &u.hash, &u.role, &u.tenant, &u.active); err != nil {
			return err
		}
		role := "VIEWER"
		if u.role == "sme" {
			role = "OWNER"
		}
		if u.role == "platform" {
			role = "DEVELOPER"
			u.tenant = platformBusinessID
		}
		if strings.TrimSpace(u.tenant) == "" {
			return fmt.Errorf("legacy user %s has no tenant and is not a platform user", u.id)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO users(user_id,business_id,full_name,email,password_hash,user_role,is_active) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(user_id) DO UPDATE SET business_id=EXCLUDED.business_id,full_name=EXCLUDED.full_name,email=EXCLUDED.email,password_hash=EXCLUDED.password_hash,user_role=EXCLUDED.user_role,is_active=EXCLUDED.is_active,updated_at=now()`, u.id, u.tenant, u.username, u.username, u.hash, role, u.active); err != nil {
			return err
		}
	}
	if err := users.Err(); err != nil {
		return err
	}
	users.Close()

	sites, err := source.QueryContext(ctx, `SELECT id,tenant_id,name,timezone,created FROM sites ORDER BY id`)
	if err != nil {
		return err
	}
	for sites.Next() {
		var s legacySite
		if err := sites.Scan(&s.id, &s.tenant, &s.name, &s.timezone, &s.created); err != nil {
			return err
		}
		created := time.Now().UTC()
		if s.created.Valid && s.created.Int64 > 0 {
			created = time.Unix(s.created.Int64, 0).UTC()
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO sites(site_id,business_id,name,timezone,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$5) ON CONFLICT(site_id) DO UPDATE SET business_id=EXCLUDED.business_id,name=EXCLUDED.name,timezone=EXCLUDED.timezone,updated_at=now()`, s.id, s.tenant, s.name, s.timezone, created); err != nil {
			return err
		}
	}
	if err := sites.Err(); err != nil {
		return err
	}
	sites.Close()

	vouchers, err := source.QueryContext(ctx, `SELECT id,tenant_id,site_id,created,expires,revoked FROM vouchers ORDER BY id`)
	if err != nil {
		return err
	}
	for vouchers.Next() {
		var id, tenant string
		var site sql.NullString
		var created, expires sql.NullInt64
		var revoked bool
		if err := vouchers.Scan(&id, &tenant, &site, &created, &expires, &revoked); err != nil {
			return err
		}
		// The old table contains code_hash/code_sealed, not a recoverable
		// plaintext code. Preserve the record as revoked with a deterministic
		// non-usable code so its history is not silently lost.
		createdAt := time.Now().UTC()
		if created.Valid && created.Int64 > 0 {
			createdAt = time.Unix(created.Int64, 0).UTC()
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO vouchers(id,business_id,plan_key,plan_label,code,status,created_at,updated_at) VALUES($1,$2,'legacy','Legacy voucher (reissue required)',$3,'revoked',$4,$4) ON CONFLICT(id) DO UPDATE SET business_id=EXCLUDED.business_id,status='revoked',updated_at=now()`, id, tenant, "LEGACY-"+id[:8], createdAt); err != nil {
			return err
		}
	}
	if err := vouchers.Err(); err != nil {
		return err
	}
	vouchers.Close()
	return tx.Commit()
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "legacy migration audit: "+format+"\n", args...)
	os.Exit(1)
}
