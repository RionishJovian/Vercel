package database

import "fmt"

// CheckRuntimePrivileges prevents production from serving with migration/owner
// credentials. Cross-tenant service DML is granted through explicit RLS policies;
// tenant requests still downgrade to authenticated before accessing their data.
func CheckRuntimePrivileges(db *DB) error {
	var unsafe bool
	err := db.QueryRow(`SELECT rolsuper OR rolbypassrls OR rolcreatedb OR rolcreaterole OR rolreplication OR has_schema_privilege(current_user,'public','CREATE') FROM pg_roles WHERE rolname=current_user`).Scan(&unsafe)
	if err != nil {
		return fmt.Errorf("runtime database privilege check failed: %w", err)
	}
	if unsafe {
		return fmt.Errorf("production database login must not have administrative, RLS-bypass or schema-creation privileges")
	}
	return nil
}
