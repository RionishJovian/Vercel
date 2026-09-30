// Command store-schema applies/checks the additive Store transaction core.
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
	migrationPath := flag.String("migration", "migrations/20260930070000_store_transaction_core.sql", "migration SQL")
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
		body, e := os.ReadFile(*migrationPath)
		if e != nil {
			fatal("read migration: %v", e)
		}
		if _, e = db.ExecContext(ctx, string(body)); e != nil {
			fatal("apply migration: %v", e)
		}
	} else if *action != "check" {
		fatal("unknown action %q", *action)
	}
	var purchases, audit, tenantColumn, idempotency, bankReference bool
	err = db.QueryRowContext(ctx, `SELECT
		to_regclass('public.service_purchases') IS NOT NULL,
		to_regclass('public.service_purchase_audit') IS NOT NULL,
		EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='service_purchases' AND column_name='business_id' AND is_nullable='NO'),
		EXISTS(SELECT 1 FROM pg_indexes WHERE schemaname='public' AND tablename='service_purchases' AND indexdef ILIKE '%idempotency_key%'),
		EXISTS(SELECT 1 FROM pg_indexes WHERE schemaname='public' AND indexname='service_purchases_bank_reference_once')`).Scan(&purchases, &audit, &tenantColumn, &idempotency, &bankReference)
	if err != nil {
		fatal("store schema check failed: %v", err)
	}
	if !purchases || !audit || !tenantColumn || !idempotency || !bankReference {
		fatal("store schema incomplete (purchases=%t audit=%t tenant=%t idempotency=%t bank_reference=%t)", purchases, audit, tenantColumn, idempotency, bankReference)
	}
	var purchasePrivileges, auditPrivileges, tenantPolicies bool
	err = db.QueryRowContext(ctx, `SELECT
		has_table_privilege('authenticated', 'public.service_purchases', 'SELECT') AND
		has_table_privilege('authenticated', 'public.service_purchases', 'INSERT') AND
		has_table_privilege('authenticated', 'public.service_purchases', 'UPDATE'),
		has_table_privilege('authenticated', 'public.service_purchase_audit', 'SELECT') AND
		has_table_privilege('authenticated', 'public.service_purchase_audit', 'INSERT'),
		(SELECT count(*) = 2 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
		 WHERE n.nspname='public' AND c.relname IN ('service_purchases','service_purchase_audit')
		 AND c.relrowsecurity AND EXISTS (SELECT 1 FROM pg_policy p WHERE p.polrelid=c.oid
		 AND p.polname=c.relname || '_tenant_isolation'))`).Scan(&purchasePrivileges, &auditPrivileges, &tenantPolicies)
	if err != nil {
		fatal("store runtime permission check failed: %v", err)
	}
	if !purchasePrivileges || !auditPrivileges || !tenantPolicies {
		fatal("store runtime access incomplete (purchases=%t audit=%t tenant_policies=%t); apply migrations/20260930210000_store_runtime_permissions.sql", purchasePrivileges, auditPrivileges, tenantPolicies)
	}
	fmt.Printf("store schema %s passed\n", *action)
}

func fatal(format string, args ...any) { fmt.Fprintf(os.Stderr, format+"\n", args...); os.Exit(1) }
