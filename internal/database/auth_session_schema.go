package database

import "fmt"

func CheckAuthSessionSchema(db *DB) error {
	rows, err := db.Query(`SELECT session_id,user_id,business_id,password_digest,user_role,refresh_digest,expires_at,revoked FROM public.auth_sessions LIMIT 0`)
	if err != nil {
		return fmt.Errorf("apply authentication session migration before deployment")
	}
	rows.Close()
	var triggers int
	err = db.QueryRow(`SELECT count(*) FROM pg_trigger WHERE (tgrelid='public.users'::regclass AND tgname='revoke_changed_user_sessions' OR tgrelid='public.businesses'::regclass AND tgname='revoke_suspended_business_sessions') AND tgenabled='O' AND NOT tgisinternal`).Scan(&triggers)
	if err != nil || triggers != 2 {
		return fmt.Errorf("authentication revocation triggers missing")
	}
	var private bool
	err = db.QueryRow(`SELECT relrowsecurity AND NOT has_table_privilege('authenticated', 'public.auth_sessions', 'SELECT,INSERT,UPDATE,DELETE') FROM pg_class WHERE oid='public.auth_sessions'::regclass`).Scan(&private)
	if err != nil || !private {
		return fmt.Errorf("authentication session table must deny tenant runtime access")
	}
	return nil
}
