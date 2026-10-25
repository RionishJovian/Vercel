package database

import (
	"context"
	"os"
	"strings"
	"testing"
)

// The role-prune migration and the legacy entitlement importer must keep the
// documented consolidation contract: existing OPERATOR/TECHNICIAN/SUPPORT rows
// become ADMIN before the stricter CHECK constraints are enforced, the
// supported roles stay valid, removed roles are rejected afterwards, tenant
// associations survive, and a failed migration leaves no partial schema
// changes. These tests run against a dedicated throwaway database (separate
// from the shared harness DB) because the pre-prune fixture intentionally
// recreates the public.* tables the migration hard-codes.

func requireRoleMigrationDB(t *testing.T) (*DB, func()) {
	t.Helper()
	if os.Getenv("MM_TEST_DB_URL") == "" {
		t.Skip("DB integration harness skipped: set MM_TEST_DB_URL=postgres://user:***@host:port/db?sslmode=disable to run against a throwaway DB")
	}
	if harnessSkip != nil {
		t.Skipf("DB integration harness skipped: DB not reachable/usable: %v", harnessSkip)
	}
	if harnessFatal != nil {
		t.Fatalf("DB integration harness setup failed after connect: %v", harnessFatal)
	}
	name := "mega_magic_test_roles_" + randHex(8)
	if _, err := harnessAdmin.Exec("CREATE DATABASE " + name); err != nil {
		t.Fatalf("create role-migration fixture db: %v", err)
	}
	cfg := harnessCfg
	cfg.Name = name
	db, err := New(cfg)
	if err != nil {
		t.Fatalf("connect role-migration fixture db: %v", err)
	}
	return db, func() {
		_ = db.Close()
		_, _ = harnessAdmin.Exec("DROP DATABASE IF EXISTS " + name)
	}
}

// createPrePruneRoleSchema recreates the RBAC tables with the OLD role CHECK
// constraints (before the pruning), plus an optional extra role accepted only
// by portal_mode_role_defaults — used to force a late ADD CONSTRAINT failure
// and prove the migration rolls back completely.
func createPrePruneRoleSchema(t *testing.T, db *DB, portalExtraRole string) {
	t.Helper()
	ddl := `
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE TABLE public.users (
    user_id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    business_id uuid NOT NULL,
    full_name text NOT NULL DEFAULT '',
    email text NOT NULL UNIQUE,
    password_hash text NOT NULL DEFAULT 'x',
    user_role text NOT NULL DEFAULT 'VIEWER'
        CHECK (user_role IN ('OWNER','ADMIN','OPERATOR','TECHNICIAN','VIEWER','SUPPORT','DEVELOPER')),
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE public.business_memberships (
    business_id uuid NOT NULL,
    user_id uuid NOT NULL REFERENCES public.users(user_id) ON DELETE CASCADE,
    role text NOT NULL CHECK (role IN ('OWNER','ADMIN','OPERATOR','TECHNICIAN','VIEWER','SUPPORT','DEVELOPER')),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (business_id, user_id)
);
CREATE TABLE public.business_invitations (
    invitation_id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    business_id uuid NOT NULL,
    email text NOT NULL,
    role text NOT NULL CHECK (role IN ('OWNER','ADMIN','OPERATOR','TECHNICIAN','VIEWER','SUPPORT','DEVELOPER')),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE public.operator_feature_role_defaults (
    role text NOT NULL CHECK (role IN ('OWNER','ADMIN','OPERATOR','TECHNICIAN','VIEWER','SUPPORT','DEVELOPER')),
    feature text NOT NULL CHECK (feature IN ('vouchers','portal_deploy','cash_sales','bank_edit','router_config','reboot','alerts','syslog_ai','poe','telegram')),
    legacy_role text NOT NULL,
    updated_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (legacy_role, feature)
);
CREATE TABLE public.portal_mode_role_defaults (
    role text NOT NULL CHECK (role IN ('OWNER','ADMIN','OPERATOR','TECHNICIAN','VIEWER','SUPPORT','DEVELOPER'` + portalExtraRole + `)),
    mode text NOT NULL CHECK (mode IN ('hybrid_light','commerce')),
    legacy_role text NOT NULL,
    updated_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (legacy_role, mode)
);`
	if _, err := db.Exec(ddl); err != nil {
		t.Fatalf("create pre-prune fixture schema: %v", err)
	}
}

func insertPruneUser(t *testing.T, db *DB, id, email, role, businessID string) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO public.users (user_id, business_id, email, user_role) VALUES ($1::uuid, $2::uuid, $3, $4)`,
		id, businessID, email, role,
	); err != nil {
		t.Fatalf("insert fixture user %s: %v", email, err)
	}
}

func pruneUserRole(t *testing.T, db *DB, email string) string {
	t.Helper()
	var role string
	if err := db.QueryRow(`SELECT user_role FROM public.users WHERE email = $1`, email).Scan(&role); err != nil {
		t.Fatalf("read user role for %s: %v", email, err)
	}
	return role
}

// applyRolePruneMigration executes the migration the way a runner does: the
// whole script on ONE connection. On failure the script's transaction is
// aborted; issue an explicit ROLLBACK on that same connection (the Go
// equivalent of psql exiting its session) so the aborted transaction can never
// leak back into the pool and poison later queries.
func applyRolePruneMigration(t *testing.T, db *DB) error {
	t.Helper()
	raw, err := os.ReadFile("../../migrations/20261018180000_prune_roles_oper_tech_supp.sql")
	if err != nil {
		t.Fatalf("read prune-roles migration: %v", err)
	}
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("dedicated migration connection: %v", err)
	}
	defer conn.Close()
	_, err = conn.ExecContext(ctx, string(raw))
	if err != nil {
		_, _ = conn.ExecContext(ctx, "ROLLBACK")
	}
	return err
}

const (
	bizA        = "11111111-1111-1111-1111-111111111111"
	bizB        = "22222222-2222-2222-2222-222222222222"
	ownerID     = "aaaaaaa1-0000-0000-0000-000000000001"
	adminID     = "aaaaaaa2-0000-0000-0000-000000000002"
	viewerID    = "aaaaaaa3-0000-0000-0000-000000000003"
	developerID = "aaaaaaa4-0000-0000-0000-000000000004"
	operatorID  = "aaaaaaa5-0000-0000-0000-000000000005"
	techID      = "aaaaaaa6-0000-0000-0000-000000000006"
	supportID   = "aaaaaaa7-0000-0000-0000-000000000007"
)

func TestRolePruneMigrationReassignsRemovedRolesAndKeepsTenantBoundaries(t *testing.T) {
	db, cleanup := requireRoleMigrationDB(t)
	defer cleanup()
	createPrePruneRoleSchema(t, db, "")

	insertPruneUser(t, db, ownerID, "owner@example.test", "OWNER", bizA)
	insertPruneUser(t, db, adminID, "admin@example.test", "ADMIN", bizA)
	insertPruneUser(t, db, viewerID, "viewer@example.test", "VIEWER", bizB)
	insertPruneUser(t, db, developerID, "developer@example.test", "DEVELOPER", bizB)
	insertPruneUser(t, db, operatorID, "operator@example.test", "OPERATOR", bizA)
	insertPruneUser(t, db, techID, "technician@example.test", "TECHNICIAN", bizB)
	insertPruneUser(t, db, supportID, "support@example.test", "SUPPORT", bizA)

	fixtures := []string{
		`INSERT INTO public.business_memberships (business_id, user_id, role) VALUES ('` + bizA + `','` + operatorID + `','OPERATOR')`,
		`INSERT INTO public.business_memberships (business_id, user_id, role) VALUES ('` + bizA + `','` + ownerID + `','OWNER')`,
		`INSERT INTO public.business_invitations (business_id, email, role) VALUES ('` + bizB + `','invite@example.test','TECHNICIAN')`,
		`INSERT INTO public.operator_feature_role_defaults (role, feature, legacy_role) VALUES ('OPERATOR','reboot','agent')`,
		`INSERT INTO public.operator_feature_role_defaults (role, feature, legacy_role) VALUES ('OPERATOR','vouchers','site_manager')`,
		`INSERT INTO public.operator_feature_role_defaults (role, feature, legacy_role) VALUES ('OWNER','vouchers','client')`,
		`INSERT INTO public.portal_mode_role_defaults (role, mode, legacy_role) VALUES ('OPERATOR','commerce','agent')`,
		`INSERT INTO public.portal_mode_role_defaults (role, mode, legacy_role) VALUES ('OWNER','hybrid_light','client')`,
	}
	for _, q := range fixtures {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("seed fixture: %v\n%s", err, q)
		}
	}

	if err := applyRolePruneMigration(t, db); err != nil {
		t.Fatalf("prune-roles migration failed on an upgraded database: %v", err)
	}

	// Removed roles are consolidated to ADMIN; supported roles are untouched.
	for email, want := range map[string]string{
		"owner@example.test":      "OWNER",
		"admin@example.test":      "ADMIN",
		"viewer@example.test":     "VIEWER",
		"developer@example.test":  "DEVELOPER",
		"operator@example.test":   "ADMIN",
		"technician@example.test": "ADMIN",
		"support@example.test":    "ADMIN",
	} {
		if got := pruneUserRole(t, db, email); got != want {
			t.Fatalf("%s role = %s, want %s", email, got, want)
		}
	}

	// Tenant associations survive: membership stays bound to the same business.
	var membershipBiz, membershipRole string
	if err := db.QueryRow(
		`SELECT business_id::text, role FROM public.business_memberships WHERE user_id = $1::uuid`, operatorID,
	).Scan(&membershipBiz, &membershipRole); err != nil {
		t.Fatalf("read upgraded membership: %v", err)
	}
	if membershipBiz != bizA || membershipRole != "ADMIN" {
		t.Fatalf("membership = (%s, %s), want (%s, ADMIN)", membershipBiz, membershipRole, bizA)
	}

	var invitationRole string
	if err := db.QueryRow(
		`SELECT role FROM public.business_invitations WHERE email = 'invite@example.test'`,
	).Scan(&invitationRole); err != nil {
		t.Fatalf("read upgraded invitation: %v", err)
	}
	if invitationRole != "ADMIN" {
		t.Fatalf("invitation role = %s, want ADMIN", invitationRole)
	}

	for _, check := range []struct{ query, want string }{
		{`SELECT role FROM public.operator_feature_role_defaults WHERE legacy_role='agent' AND feature='reboot'`, "ADMIN"},
		{`SELECT role FROM public.operator_feature_role_defaults WHERE legacy_role='site_manager' AND feature='vouchers'`, "ADMIN"},
		{`SELECT role FROM public.operator_feature_role_defaults WHERE legacy_role='client' AND feature='vouchers'`, "OWNER"},
		{`SELECT role FROM public.portal_mode_role_defaults WHERE legacy_role='agent'`, "ADMIN"},
		{`SELECT role FROM public.portal_mode_role_defaults WHERE legacy_role='client'`, "OWNER"},
	} {
		var got string
		if err := db.QueryRow(check.query).Scan(&got); err != nil {
			t.Fatalf("read role default (%s): %v", check.query, err)
		}
		if got != check.want {
			t.Fatalf("role default = %s, want %s (%s)", got, check.want, check.query)
		}
	}

	// Supported roles remain insertable everywhere the constraints were tightened.
	for _, q := range []string{
		`INSERT INTO public.users (user_id, business_id, email, user_role) VALUES (uuid_generate_v4(),'` + bizA + `','newviewer@example.test','VIEWER')`,
		`INSERT INTO public.business_invitations (business_id, email, role) VALUES ('` + bizA + `','invite-viewer@example.test','VIEWER')`,
		`INSERT INTO public.operator_feature_role_defaults (role, feature, legacy_role) VALUES ('DEVELOPER','alerts','client')`,
		`INSERT INTO public.portal_mode_role_defaults (role, mode, legacy_role) VALUES ('ADMIN','commerce','agent2')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("supported role rejected after migration: %v\n%s", err, q)
		}
	}

	// Removed roles are rejected by every tightened constraint afterwards.
	for _, q := range []string{
		`INSERT INTO public.users (user_id, business_id, email, user_role) VALUES (uuid_generate_v4(),'` + bizA + `','old@example.test','OPERATOR')`,
		`INSERT INTO public.business_memberships (business_id, user_id, role) VALUES ('` + bizA + `','` + ownerID + `','SUPPORT')`,
		`INSERT INTO public.business_invitations (business_id, email, role) VALUES ('` + bizA + `','invite-old@example.test','TECHNICIAN')`,
		`INSERT INTO public.business_invitations (business_id, email, role) VALUES ('` + bizA + `','invite-owner@example.test','OWNER')`,
		`INSERT INTO public.operator_feature_role_defaults (role, feature, legacy_role) VALUES ('OPERATOR','alerts','agent3')`,
		`INSERT INTO public.portal_mode_role_defaults (role, mode, legacy_role) VALUES ('SUPPORT','commerce','agent4')`,
	} {
		if _, err := db.Exec(q); err == nil {
			t.Fatalf("removed role accepted after migration: %s", q)
		} else if !strings.Contains(err.Error(), "check constraint") {
			t.Fatalf("expected CHECK constraint rejection, got: %v\n%s", err, q)
		}
	}
}

func TestRolePruneMigrationFailureLeavesNoPartialSchemaChanges(t *testing.T) {
	db, cleanup := requireRoleMigrationDB(t)
	defer cleanup()
	// The old portal-mode constraint also accepts a role the migration cannot
	// consolidate ('GHOST'), so the FINAL ADD CONSTRAINT fails after earlier
	// UPDATEs and ALTERs in the same script have already succeeded.
	createPrePruneRoleSchema(t, db, ",'GHOST'")

	insertPruneUser(t, db, operatorID, "operator@example.test", "OPERATOR", bizA)
	for _, q := range []string{
		`INSERT INTO public.business_memberships (business_id, user_id, role) VALUES ('` + bizA + `','` + operatorID + `','OPERATOR')`,
		`INSERT INTO public.operator_feature_role_defaults (role, feature, legacy_role) VALUES ('OPERATOR','reboot','agent')`,
		`INSERT INTO public.portal_mode_role_defaults (role, mode, legacy_role) VALUES ('GHOST','commerce','ghost')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("seed failure fixture: %v\n%s", err, q)
		}
	}

	err := applyRolePruneMigration(t, db)
	if err == nil {
		t.Fatal("expected the sabotage fixture to fail the migration, got success")
	}
	if !strings.Contains(err.Error(), "portal_mode_role_defaults_role_check") {
		t.Fatalf("expected the portal_mode_role_defaults constraint to fail, got: %v", err)
	}

	// Nothing from the failed run survived: the OPERATOR row, its membership,
	// the old (pre-prune) constraint that still accepts OPERATOR, and the
	// GHOST row are all exactly as before the attempt.
	if got := pruneUserRole(t, db, "operator@example.test"); got != "OPERATOR" {
		t.Fatalf("user role = %s after failed migration, want OPERATOR (reassignment must roll back)", got)
	}
	var membershipRole string
	if err := db.QueryRow(
		`SELECT role FROM public.business_memberships WHERE user_id = $1::uuid`, operatorID,
	).Scan(&membershipRole); err != nil {
		t.Fatalf("read membership after failed migration: %v", err)
	}
	if membershipRole != "OPERATOR" {
		t.Fatalf("membership role = %s after failed migration, want OPERATOR", membershipRole)
	}
	if _, err := db.Exec(
		`INSERT INTO public.users (user_id, business_id, email, user_role) VALUES (uuid_generate_v4(),'` + bizA + `','still-old@example.test','OPERATOR')`,
	); err != nil {
		t.Fatalf("users constraint was tightened despite the failed migration (earlier ALTER must roll back): %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO public.portal_mode_role_defaults (role, mode, legacy_role) VALUES ('GHOST','hybrid_light','ghost2')`,
	); err != nil {
		t.Fatalf("portal_mode constraint changed despite the failed migration: %v", err)
	}
}

func TestLegacyEntitlementsImportEmitsSupportedRoles(t *testing.T) {
	db, cleanup := requireRoleMigrationDB(t)
	defer cleanup()
	// The committed (pruned) role policy, as in database/init.sql.
	ddl := `
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE TABLE public.users (
    user_id uuid PRIMARY KEY,
    business_id uuid NOT NULL,
    full_name text NOT NULL DEFAULT '',
    email text NOT NULL UNIQUE,
    password_hash text NOT NULL DEFAULT 'x',
    user_role text NOT NULL DEFAULT 'VIEWER'
        CHECK (user_role IN ('OWNER', 'ADMIN', 'VIEWER', 'DEVELOPER')),
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE public.account_entitlements (
    user_id uuid PRIMARY KEY,
    business_id uuid NOT NULL,
    tier text NOT NULL CHECK (tier IN ('spark','trial','monthly','annual')),
    tier_expires_at timestamptz,
    plus boolean NOT NULL DEFAULT false,
    plus_since timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE public.operator_feature_grants (
    user_id uuid NOT NULL,
    business_id uuid NOT NULL,
    feature text NOT NULL CHECK (feature IN ('vouchers','portal_deploy','cash_sales','bank_edit','router_config','reboot','alerts','syslog_ai','poe','telegram')),
    granted_by uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, feature)
);
CREATE TABLE public.operator_feature_role_defaults (
    role text NOT NULL CHECK (role IN ('OWNER','ADMIN','VIEWER','DEVELOPER')),
    feature text NOT NULL CHECK (feature IN ('vouchers','portal_deploy','cash_sales','bank_edit','router_config','reboot','alerts','syslog_ai','poe','telegram')),
    legacy_role text NOT NULL,
    updated_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (legacy_role, feature)
);
CREATE TABLE public.portal_mode_role_defaults (
    role text NOT NULL CHECK (role IN ('OWNER','ADMIN','VIEWER','DEVELOPER')),
    mode text NOT NULL CHECK (mode IN ('hybrid_light','commerce')),
    legacy_role text NOT NULL,
    updated_by uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (legacy_role, mode)
);
CREATE TABLE public._stg_entitlements (user_id text, tier text, tier_expires_at text, plus text, plus_since text, created_at text, updated_at text, owner_id text);
CREATE TABLE public._stg_feature_grants (user_id text, feature text, granted_by text, created_at text);
CREATE TABLE public._stg_role_defaults (legacy_role text, feature text, updated_by text, updated_at text);
CREATE TABLE public._stg_portal_mode (legacy_role text, mode text, updated_by text, updated_at text);`
	if _, err := db.Exec(ddl); err != nil {
		t.Fatalf("create import fixture schema: %v", err)
	}

	staffID := "bbbbbbb1-0000-0000-0000-000000000001"
	for _, q := range []string{
		`INSERT INTO public.users (user_id, business_id, email, user_role) VALUES ('` + ownerID + `','` + bizA + `','owner@example.test','OWNER')`,
		`INSERT INTO public.users (user_id, business_id, email, user_role) VALUES ('` + staffID + `','` + bizA + `','staff@example.test','ADMIN')`,
		// A pre-existing (client, vouchers) default must be preserved by ON CONFLICT DO NOTHING.
		`INSERT INTO public.operator_feature_role_defaults (role, feature, legacy_role) VALUES ('OWNER','vouchers','client')`,
		`INSERT INTO public._stg_entitlements VALUES ('` + staffID + `','monthly','','true','','2026-01-01T00:00:00Z','2026-01-02T00:00:00Z','')`,
		`INSERT INTO public._stg_feature_grants VALUES ('` + staffID + `','reboot','','2026-01-01T00:00:00Z')`,
		`INSERT INTO public._stg_role_defaults VALUES ('client','reboot','','2026-01-01T00:00:00Z')`,
		`INSERT INTO public._stg_role_defaults VALUES ('agent','reboot','','2026-01-01T00:00:00Z')`,
		`INSERT INTO public._stg_role_defaults VALUES ('site_manager','vouchers','','2026-01-01T00:00:00Z')`,
		`INSERT INTO public._stg_role_defaults VALUES ('other','alerts','','2026-01-01T00:00:00Z')`,
		`INSERT INTO public._stg_portal_mode VALUES ('client','hybrid_light','','2026-01-01T00:00:00Z')`,
		`INSERT INTO public._stg_portal_mode VALUES ('agent','commerce','','2026-01-01T00:00:00Z')`,
		`INSERT INTO public._stg_portal_mode VALUES ('other','commerce','','2026-01-01T00:00:00Z')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("seed import fixture: %v\n%s", err, q)
		}
	}

	raw, err := os.ReadFile("../../scripts/migration/sql/120_entitlements_features.sql")
	if err != nil {
		t.Fatalf("read legacy entitlements importer: %v", err)
	}
	if _, err := db.Exec(string(raw)); err != nil {
		t.Fatalf("legacy entitlements importer failed against the pruned schema: %v", err)
	}

	for _, check := range []struct{ query, want string }{
		{`SELECT role FROM public.operator_feature_role_defaults WHERE legacy_role='client' AND feature='reboot'`, "OWNER"},
		{`SELECT role FROM public.operator_feature_role_defaults WHERE legacy_role='agent' AND feature='reboot'`, "ADMIN"},
		{`SELECT role FROM public.operator_feature_role_defaults WHERE legacy_role='site_manager' AND feature='vouchers'`, "ADMIN"},
		{`SELECT role FROM public.operator_feature_role_defaults WHERE legacy_role='client' AND feature='vouchers'`, "OWNER"},
		{`SELECT role FROM public.portal_mode_role_defaults WHERE legacy_role='client'`, "OWNER"},
		{`SELECT role FROM public.portal_mode_role_defaults WHERE legacy_role='agent'`, "ADMIN"},
	} {
		var got string
		if err := db.QueryRow(check.query).Scan(&got); err != nil {
			t.Fatalf("read imported role default (%s): %v", check.query, err)
		}
		if got != check.want {
			t.Fatalf("imported role default = %s, want %s (%s)", got, check.want, check.query)
		}
	}

	// Unknown legacy roles are excluded by the importer's WHERE clause, and no
	// removed role may survive the import anywhere.
	for _, check := range []struct {
		query string
		want  int
	}{
		{`SELECT count(*) FROM public.operator_feature_role_defaults WHERE legacy_role='other'`, 0},
		{`SELECT count(*) FROM public.portal_mode_role_defaults WHERE legacy_role='other'`, 0},
		{`SELECT count(*) FROM public.operator_feature_role_defaults WHERE role IN ('OPERATOR','TECHNICIAN','SUPPORT')`, 0},
		{`SELECT count(*) FROM public.portal_mode_role_defaults WHERE role IN ('OPERATOR','TECHNICIAN','SUPPORT')`, 0},
		{`SELECT count(*) FROM public.operator_feature_role_defaults WHERE legacy_role='client' AND feature='vouchers'`, 1},
	} {
		var got int
		if err := db.QueryRow(check.query).Scan(&got); err != nil {
			t.Fatalf("count imported rows (%s): %v", check.query, err)
		}
		if got != check.want {
			t.Fatalf("count = %d, want %d (%s)", got, check.want, check.query)
		}
	}

	var tier string
	var plus bool
	if err := db.QueryRow(
		`SELECT tier, plus FROM public.account_entitlements WHERE user_id = $1::uuid`, staffID,
	).Scan(&tier, &plus); err != nil {
		t.Fatalf("read imported entitlement: %v", err)
	}
	if tier != "monthly" || !plus {
		t.Fatalf("imported entitlement = (%s, %t), want (monthly, true)", tier, plus)
	}
	var granted string
	if err := db.QueryRow(
		`SELECT feature FROM public.operator_feature_grants WHERE user_id = $1::uuid`, staffID,
	).Scan(&granted); err != nil {
		t.Fatalf("read imported feature grant: %v", err)
	}
	if granted != "reboot" {
		t.Fatalf("imported feature grant = %s, want reboot", granted)
	}
}
