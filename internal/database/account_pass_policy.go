package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"mega-magic/internal/accountpass"
	"time"
)

// AccountPassBusinessAccess is for background app work without a browser JWT.
// It does not gate guest sessions, router enforcement or accounting sync.
func AccountPassBusinessAccess(ctx context.Context, db *DB, businessID string, now time.Time) (bool, error) {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	access := false
	err = WithTenant(tx, businessID, func() error {
		var user, role string
		e := tx.QueryRow(`SELECT u.user_id,u.user_role FROM users u JOIN businesses b ON b.business_id=u.business_id LEFT JOIN business_app_primary_users p ON p.business_id=u.business_id WHERE u.business_id=$1 AND u.is_active AND b.is_active AND (u.user_role='DEVELOPER' OR (p.user_id=u.user_id AND u.user_role='OWNER')) ORDER BY (u.user_role='DEVELOPER') DESC LIMIT 1`, businessID).Scan(&user, &role)
		if e == sql.ErrNoRows {
			return nil
		}
		if e != nil {
			return e
		}
		status, e := AccountPassStatusTXN(tx, businessID, user, true, role == "DEVELOPER", now)
		access = status.Access
		return e
	})
	return access, err
}

func AccountPassStatusTXN(tx *sql.Tx, businessID, userID string, enabled, operator bool, now time.Time) (accountpass.Status, error) {
	var start, end, revoked, transition sql.NullTime
	var plan, primary sql.NullString
	var operatorWorkspace bool
	err := tx.QueryRow(`SELECT p.plan,p.activated_at,p.expires_at,p.revoked_at,t.expires_at,a.user_id,EXISTS(SELECT 1 FROM users WHERE business_id=$2 AND is_active AND user_role='DEVELOPER')
 FROM (SELECT $1::uuid AS user_id) me
 LEFT JOIN app_account_passes p ON p.user_id=me.user_id AND p.business_id=$2
 LEFT JOIN app_account_pass_transitions t ON t.user_id=me.user_id AND t.business_id=$2
 LEFT JOIN business_app_primary_users a ON a.business_id=$2`, userID, businessID).Scan(&plan, &start, &end, &revoked, &transition, &primary, &operatorWorkspace)
	if err != nil {
		return accountpass.Status{}, err
	}
	pointer := func(v sql.NullTime) *time.Time {
		if !v.Valid {
			return nil
		}
		return &v.Time
	}
	status := accountpass.Evaluate(now, enabled, operator, pointer(start), pointer(end), pointer(revoked), pointer(transition))
	status.Authorized = operator || (!operatorWorkspace && primary.Valid && primary.String == userID)
	if enabled && !status.Authorized {
		status.Access = false
		status.State = "authorized_login_required"
	}
	if enabled && status.Authorized && !status.Access && status.State == "pass_required" {
		// Only logins without access pay for this lookup. The table is optional so a
		// release can ship before the trial migration is applied.
		var present bool
		if e := tx.QueryRow(`SELECT to_regclass('public.app_account_trials') IS NOT NULL`).Scan(&present); e != nil {
			return accountpass.Status{}, e
		}
		if present {
			var trialEnd sql.NullTime
			if e := tx.QueryRow(`SELECT expires_at FROM app_account_trials WHERE user_id=$1 AND business_id=$2`, userID, businessID).Scan(&trialEnd); e != nil && !errors.Is(e, sql.ErrNoRows) {
				return accountpass.Status{}, e
			}
			status = accountpass.ApplyTrial(status, now, pointer(trialEnd))
		}
	}
	status.Kind = "none"
	if plan.Valid {
		status.Kind = plan.String
	}
	return status, nil
}

func CheckIndividualPassSchema(db *DB) error {
	if _, err := db.Exec(`SELECT p.user_id,t.expires_at,r.starts_at,r.enforcement_enabled,a.user_id,n.status,n.amount_mmk,n.bank_account_id,n.bank_reference,n.bank_snapshot,n.idempotency_key,e.changed_by FROM app_account_passes p LEFT JOIN app_account_pass_transitions t ON t.user_id=p.user_id CROSS JOIN app_account_pass_rollouts r LEFT JOIN business_app_primary_users a ON a.business_id=p.business_id LEFT JOIN app_account_pass_renewals n ON n.user_id=p.user_id LEFT JOIN business_app_primary_events e ON e.business_id=p.business_id LIMIT 0`); err != nil {
		return err
	}
	var checkoutReady bool
	if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='app_account_pass_renewals'::regclass AND conname='account_pass_payment_evidence' AND contype='c' AND convalidated)
 AND EXISTS(SELECT 1 FROM pg_index WHERE indexrelid=to_regclass('account_pass_payment_key') AND indrelid='app_account_pass_renewals'::regclass AND indisunique AND indisvalid AND pg_get_indexdef(indexrelid) LIKE '%(user_id, idempotency_key)%')`).Scan(&checkoutReady); err != nil {
		return err
	}
	if !checkoutReady {
		return fmt.Errorf("individual pass checkout protections incomplete")
	}
	var protected, guards int
	if err := db.QueryRow(`SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relname IN('app_account_passes','app_account_pass_events','app_account_pass_transitions','app_account_pass_renewals','business_app_primary_users','business_app_primary_events') AND c.relrowsecurity AND c.relforcerowsecurity`).Scan(&protected); err != nil {
		return err
	}
	if protected != 6 {
		return fmt.Errorf("individual pass tenant protections incomplete")
	}
	if err := db.QueryRow(`SELECT count(*) FROM pg_trigger t JOIN pg_class c ON c.oid=t.tgrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND NOT t.tgisinternal AND t.tgenabled='O' AND ((c.relname='sites' AND t.tgname='app_site_capacity') OR(c.relname='devices' AND t.tgname='app_router_capacity') OR(c.relname='users' AND t.tgname='new_business_primary_login'))`).Scan(&guards); err != nil {
		return err
	}
	if guards != 3 {
		return fmt.Errorf("individual pass workspace guards incomplete")
	}
	var unsafe bool
	if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM unnest(ARRAY['app_account_passes','app_account_pass_events','app_account_pass_transitions','app_account_pass_rollouts','business_app_primary_users','business_app_primary_events']) t WHERE has_table_privilege('authenticated',t,'INSERT,UPDATE,DELETE')) OR has_table_privilege('authenticated','app_account_pass_renewals','UPDATE,DELETE')`).Scan(&unsafe); err != nil {
		return err
	}
	if unsafe {
		return fmt.Errorf("individual pass authority writable by tenant role")
	}
	var starts time.Time
	return db.QueryRow(`SELECT starts_at FROM app_account_pass_rollouts WHERE rollout_key='individual-v1' AND transition_days=7 AND enforcement_enabled AND starts_at<=now()`).Scan(&starts)
}
