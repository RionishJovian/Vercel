// account-expiry-audit only reads expiry and deletion dependencies. It never
// deletes accounts, exposes credentials, or infers expiry from a missing plan.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"mega-magic/internal/config"
	"mega-magic/internal/database"
)

func main() {
	cfg, err := config.Load("config.yaml")
	if err != nil {
		fail("configuration unavailable")
	}
	db, err := database.New(cfg.Database)
	if err != nil {
		fail("database unavailable")
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		fail("read-only transaction unavailable")
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SET LOCAL statement_timeout = '10s'"); err != nil {
		fail("read timeout unavailable")
	}
	rows, err := tx.QueryContext(ctx, `SELECT u.user_id::text, u.business_id::text, u.full_name, u.user_role,
        u.is_active, b.company_name, e.tier, e.tier_expires_at,
        (lower(trim(u.full_name)) = 'kyaw' OR lower(split_part(u.email, '@', 1)) = 'kyaw') AS protected_kyaw,
        (e.tier IN ('trial','monthly','annual') AND e.tier_expires_at IS NOT NULL AND e.tier_expires_at < now()
          AND NOT EXISTS (SELECT 1 FROM account_entitlements a WHERE a.business_id=u.business_id
             AND a.tier IN ('trial','monthly','annual') AND (a.tier_expires_at > now() OR a.tier_expires_at IS NULL))
          AND NOT EXISTS (SELECT 1 FROM service_purchases p WHERE p.business_id=u.business_id
             AND p.status='approved' AND p.service_key IN ('monthly','annual') AND p.activated_expires_at > now())) AS expired_without_active_business_plan,
        (SELECT count(*) FROM users x WHERE x.business_id=u.business_id) AS business_users,
        (SELECT count(*) FROM service_purchases p WHERE p.user_id=u.user_id OR p.decided_by=u.user_id) AS linked_purchases,
        (SELECT count(*) FROM platform_admins a WHERE a.user_id=u.user_id) AS platform_admin_records
        FROM users u JOIN businesses b ON b.business_id=u.business_id
        LEFT JOIN account_entitlements e ON e.user_id=u.user_id ORDER BY lower(u.full_name), u.user_id`)
	if err != nil {
		fail("account inventory query failed; no changes made")
	}
	var accounts []map[string]any
	for rows.Next() {
		var id, business, name, role, company string
		var tier sql.NullString
		var expiry sql.NullTime
		var active, protected bool
		var expired sql.NullBool
		var users, purchases, admins int
		if rows.Scan(&id, &business, &name, &role, &active, &company, &tier, &expiry, &protected, &expired, &users, &purchases, &admins) != nil {
			fail("inventory decoding failed")
		}
		var end any
		if expiry.Valid {
			end = expiry.Time.UTC().Format(time.RFC3339)
		}
		accounts = append(accounts, map[string]any{"user_id": id, "business_id": business, "name": name, "role": role, "active": active, "business": company, "tier": tier.String, "expires_at": end, "protected_kyaw": protected, "expiry_candidate": expired.Valid && expired.Bool && !protected, "business_users": users, "linked_purchases": purchases, "platform_admin_records": admins})
	}
	if rows.Err() != nil {
		fail("inventory read failed")
	}
	rows.Close()
	fks, err := tx.QueryContext(ctx, `SELECT c.conrelid::regclass::text,c.conname,pg_get_constraintdef(c.oid)
       FROM pg_constraint c WHERE c.contype='f' AND c.confrelid IN ('public.users'::regclass,'public.businesses'::regclass)
       ORDER BY c.conrelid::regclass::text,c.conname`)
	if err != nil {
		fail("dependency inventory failed")
	}
	var dependencies []map[string]string
	for fks.Next() {
		var table, name, definition string
		if fks.Scan(&table, &name, &definition) != nil {
			fail("dependency decoding failed")
		}
		dependencies = append(dependencies, map[string]string{"table": table, "constraint": name, "definition": definition})
	}
	if fks.Err() != nil {
		fail("dependency read failed")
	}
	fks.Close()
	if err = tx.Commit(); err != nil {
		fail("read-only audit completion failed")
	}
	json.NewEncoder(os.Stdout).Encode(map[string]any{"read_only": true, "accounts": accounts, "foreign_keys": dependencies})
}

func fail(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }
