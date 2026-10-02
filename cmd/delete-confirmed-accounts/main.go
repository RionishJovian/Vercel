// One-time owner-confirmed cleanup. Default mode is read-only; the allowlist
// cannot be expanded through flags. Never delete businesses or network assets.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/lib/pq"
	"mega-magic/internal/config"
	"mega-magic/internal/database"
)

var targets = map[string]string{
	"7a74bcb9-4efa-440c-8e4f-fd88178101dc": "Leopard",
	"6e1b61e7-9cf6-463f-8f24-af36e36bf4d4": "magic",
	"6bfb5dd8-58a0-4175-af77-a5efde5f2fad": "mgkyaw",
	"67eb85cf-ccc5-4a02-aaa2-3fe08ca1e5be": "Min",
	"8478d0d4-ff0e-45d6-ba96-6c2e1ea2a522": "Phyo",
	"fdaf4e2e-4534-4f24-8a92-f87a4942b980": "PyaeSone",
	"39a5221a-e652-44dd-8e3a-7789fadab87b": "Thet Aung",
	"9c9c360a-eb52-4611-9775-13adacb00fd2": "venusvenus",
}

func main() {
	apply := flag.Bool("apply", false, "delete the eight explicitly confirmed IDs after safety checks")
	flag.Parse()
	if err := run(*apply); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(apply bool) error {
	cfg, err := config.Load("config.yaml")
	if err != nil {
		return fmt.Errorf("configuration unavailable")
	}
	db, err := database.New(cfg.Database)
	if err != nil {
		return fmt.Errorf("database unavailable")
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: !apply, Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("transaction unavailable")
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SET LOCAL statement_timeout='10s'; SET LOCAL lock_timeout='3s'"); err != nil {
		return fmt.Errorf("timeouts unavailable")
	}
	ids := make([]string, 0, len(targets))
	for id := range targets {
		ids = append(ids, id)
	}
	var beforeTotal, beforeBusinesses int
	if tx.QueryRowContext(ctx, "SELECT (SELECT count(*) FROM users),(SELECT count(*) FROM businesses)").Scan(&beforeTotal, &beforeBusinesses) != nil {
		return fmt.Errorf("baseline unavailable")
	}
	query := `SELECT user_id::text, full_name FROM users WHERE user_id=ANY($1::uuid[]) ORDER BY user_id`
	if apply {
		query += " FOR UPDATE"
	}
	rows, err := tx.QueryContext(ctx, query, pq.Array(ids))
	if err != nil {
		return fmt.Errorf("target lock unavailable")
	}
	count := 0
	for rows.Next() {
		var id, name string
		if rows.Scan(&id, &name) != nil {
			rows.Close()
			return fmt.Errorf("target decoding failed")
		}
		if targets[id] != name {
			rows.Close()
			return fmt.Errorf("target identity changed; nothing deleted")
		}
		count++
	}
	err = rows.Err()
	rows.Close()
	if err != nil || count != 8 {
		return fmt.Errorf("exact eight targets not present; nothing deleted")
	}
	var unsafe int
	err = tx.QueryRowContext(ctx, `SELECT count(*) FROM users u WHERE u.user_id=ANY($1::uuid[]) AND (
       u.user_role IN ('ADMIN','DEVELOPER') OR lower(trim(u.full_name)) IN ('kyaw','nish')
       OR u.user_id IN ('02075721-b1ac-44b6-b9c1-824092d67165'::uuid,'4c5f9e3b-4e7d-4746-9b16-dcdfc4ca42f9'::uuid,'ace018e8-e422-498c-bf3c-2d1ac7b3a3ea'::uuid)
       OR EXISTS(SELECT 1 FROM platform_admins a WHERE a.user_id=u.user_id)
       OR EXISTS(SELECT 1 FROM account_entitlements e WHERE e.user_id=u.user_id)
       OR EXISTS(SELECT 1 FROM account_entitlements e WHERE e.business_id=u.business_id AND e.tier IN ('trial','monthly','annual') AND (e.tier_expires_at IS NULL OR e.tier_expires_at>now()))
       OR EXISTS(SELECT 1 FROM service_purchases p WHERE p.business_id=u.business_id AND p.status='approved' AND p.service_key IN ('monthly','annual') AND p.activated_expires_at>now())
       OR EXISTS(SELECT 1 FROM magic_coin_wallets w WHERE w.user_id=u.user_id AND w.balance<>0))`, pq.Array(ids)).Scan(&unsafe)
	if err != nil || unsafe != 0 {
		if !apply && err == nil {
			details, e := tx.QueryContext(ctx, `SELECT u.full_name, u.user_role,
              lower(split_part(u.email,'@',1)) IN ('kyaw','nish'),
              (SELECT count(*) FROM account_entitlements a WHERE a.user_id=u.user_id),
              COALESCE((SELECT balance::text FROM magic_coin_wallets w WHERE w.user_id=u.user_id),'0')
              FROM users u WHERE u.user_id=ANY($1::uuid[]) ORDER BY u.full_name`, pq.Array(ids))
			if e == nil {
				for details.Next() {
					var name, role, balance string
					var protectedEmail bool
					var entitlements int
					if details.Scan(&name, &role, &protectedEmail, &entitlements, &balance) == nil {
						json.NewEncoder(os.Stdout).Encode(map[string]any{"name": name, "role": role, "protected_email_name": protectedEmail, "entitlement_records": entitlements, "wallet_balance": balance})
					}
				}
				details.Close()
			}
		}
		return fmt.Errorf("protected role, subscription or wallet state changed; nothing deleted")
	}
	// Catalog names are quoted; IDs remain parameterized. Inspect every direct
	// user FK before relying on CASCADE or SET NULL. Unknown blockers fail closed.
	rows, err = tx.QueryContext(ctx, `SELECT n.nspname,t.relname,a.attname,c.confdeltype::text
       FROM pg_constraint c JOIN pg_class t ON t.oid=c.conrelid JOIN pg_namespace n ON n.oid=t.relnamespace
       JOIN pg_attribute a ON a.attrelid=t.oid AND a.attnum=c.conkey[1]
       WHERE c.contype='f' AND c.confrelid='public.users'::regclass AND cardinality(c.conkey)=1 ORDER BY 1,2,3`)
	if err != nil {
		return fmt.Errorf("dependency catalog unavailable")
	}
	type dependency struct{ schema, table, column, action string }
	var deps []dependency
	for rows.Next() {
		var d dependency
		if rows.Scan(&d.schema, &d.table, &d.column, &d.action) != nil {
			rows.Close()
			return fmt.Errorf("dependency decoding failed")
		}
		deps = append(deps, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return fmt.Errorf("dependency inventory failed")
	}
	var impacts []map[string]any
	for _, d := range deps {
		var n int
		q := "SELECT count(*) FROM " + pq.QuoteIdentifier(d.schema) + "." + pq.QuoteIdentifier(d.table) + " WHERE " + pq.QuoteIdentifier(d.column) + "=ANY($1::uuid[])"
		if tx.QueryRowContext(ctx, q, pq.Array(ids)).Scan(&n) != nil {
			return fmt.Errorf("dependency count failed")
		}
		if n > 0 {
			impacts = append(impacts, map[string]any{"table": d.table, "column": d.column, "rows": n, "delete_action": d.action})
			if d.action != "c" && d.action != "n" {
				return fmt.Errorf("linked restrictive records in %s; nothing deleted", d.table)
			}
		}
	}
	if apply {
		result, err := tx.ExecContext(ctx, "DELETE FROM users WHERE user_id=ANY($1::uuid[])", pq.Array(ids))
		if err != nil {
			return fmt.Errorf("deletion rejected; transaction rolled back")
		}
		n, err := result.RowsAffected()
		if err != nil || n != 8 {
			return fmt.Errorf("deletion count mismatch; transaction rolled back")
		}
		var remaining, total, businesses int
		if tx.QueryRowContext(ctx, "SELECT (SELECT count(*) FROM users WHERE user_id=ANY($1::uuid[])),(SELECT count(*) FROM users),(SELECT count(*) FROM businesses)", pq.Array(ids)).Scan(&remaining, &total, &businesses) != nil || remaining != 0 || total != beforeTotal-8 || businesses != beforeBusinesses {
			return fmt.Errorf("verification mismatch; transaction rolled back")
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("transaction not confirmed; re-audit required before retry")
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"deleted": apply, "account_count": 8, "targets": targets, "linked_records": impacts, "businesses_preserved": true, "other_accounts_preserved": true})
}
