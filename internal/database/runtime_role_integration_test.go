package database

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestRuntimeRoleHasServiceDMLWithoutDatabaseAdministration(t *testing.T) {
	admin := requireHarness(t)
	role := "mm_runtime_test_" + randHex(6)
	if _, err := admin.Exec(`CREATE TABLE runtime_role_private(id integer); ALTER TABLE runtime_role_private ENABLE ROW LEVEL SECURITY; ALTER TABLE runtime_role_private FORCE ROW LEVEL SECURITY;`); err != nil {
		t.Fatal(err)
	}
	script, err := os.ReadFile("../../deploy/runtime-db-role.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = admin.Exec(strings.ReplaceAll(string(script), "mega_magic_runtime", role)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(`DROP OWNED BY ` + role + `; DROP ROLE ` + role + `; DROP TABLE IF EXISTS runtime_role_private`)
	})
	if _, err = admin.Exec(fmt.Sprintf(`ALTER ROLE %s LOGIN PASSWORD 'disposable-runtime-test-only'`, role)); err != nil {
		t.Fatal(err)
	}
	cfg := harnessCfg
	cfg.User, cfg.Password = role, "disposable-runtime-test-only"
	runtime, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err = CheckRuntimePrivileges(runtime); err != nil {
		t.Fatal(err)
	}
	if err = CheckRuntimePrivileges(admin); err == nil {
		t.Fatal("production accepted the schema owner's administrative login")
	}
	var unsafe bool
	if err = runtime.QueryRow(`SELECT rolsuper OR rolbypassrls OR rolcreatedb OR rolcreaterole OR rolreplication FROM pg_roles WHERE rolname=current_user`).Scan(&unsafe); err != nil || unsafe {
		t.Fatalf("runtime has administration privileges: %v %v", unsafe, err)
	}
	a, b := insertBusiness(t, admin), insertBusiness(t, admin)
	var count int
	if err = runtime.QueryRow(`SELECT count(*) FROM businesses WHERE business_id IN ($1,$2)`, a, b).Scan(&count); err != nil || count != 2 {
		t.Fatalf("platform/worker lookup broken: %d %v", count, err)
	}
	if _, err = runtime.Exec(`INSERT INTO runtime_role_private VALUES(1)`); err != nil {
		t.Fatalf("forced-RLS service write: %v", err)
	}
	tx, err := runtime.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err = WithTenant(tx, a, func() error {
		return tx.QueryRow(`SELECT count(*) FROM businesses WHERE business_id IN ($1,$2)`, a, b).Scan(&count)
	}); err != nil || count != 1 {
		t.Fatalf("tenant downgrade failed: %d %v", count, err)
	}
	_ = tx.Rollback()
	for _, query := range []string{
		`CREATE TABLE public.runtime_must_not_create(id int)`,
		`ALTER ROLE ` + role + ` SUPERUSER`,
		`SET ROLE ` + harnessCfg.User,
		`TRUNCATE businesses`,
	} {
		if _, err = runtime.Exec(query); err == nil {
			t.Fatalf("administrative operation permitted: %s", query)
		}
	}
	conn, err := runtime.TenantConn(t.Context(), a)
	if err != nil {
		t.Fatal(err)
	}
	defer ReleaseTenantConn(t.Context(), conn)
	if _, err = conn.ExecContext(t.Context(), `INSERT INTO runtime_role_private VALUES(2)`); err == nil {
		t.Fatal("tenant connection retained service-only authority")
	}
}
