package database

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// PlatformAccounts is a bounded SYSTEM-LEVEL read. Callers MUST be DEVELOPER.
// It never selects password hashes, refresh digests, session IDs or payment evidence.
func (r *PlatformBusinessRepository) PlatformAccounts(ctx context.Context, limit, offset int, subscriptionOnly bool, now time.Time) ([]map[string]any, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&total); err != nil {
		return nil, 0, err
	}
	var hasPurchases, hasTrials bool
	if err := r.db.QueryRowContext(ctx, `SELECT to_regclass('public.service_purchases') IS NOT NULL,to_regclass('public.business_subscription_trials') IS NOT NULL`).Scan(&hasPurchases, &hasTrials); err != nil {
		return nil, 0, err
	}
	if subscriptionOnly && (!hasPurchases || !hasTrials) {
		return nil, 0, fmt.Errorf("subscription reporting schema unavailable")
	}
	purchases := `SELECT id,business_id,user_id,service_key,status,decided_at,activated_expires_at FROM service_purchases`
	if !hasPurchases {
		purchases = `SELECT NULL::uuid AS id,NULL::uuid AS business_id,NULL::uuid AS user_id,NULL::text AS service_key,NULL::text AS status,NULL::timestamptz AS decided_at,NULL::timestamptz AS activated_expires_at WHERE false`
	}
	trials := `SELECT business_id,trial_started_at,trial_expires_at FROM business_subscription_trials`
	if !hasTrials {
		trials = `SELECT NULL::uuid AS business_id,NULL::timestamptz AS trial_started_at,NULL::timestamptz AS trial_expires_at WHERE false`
	}
	query := `WITH account_purchases AS (__PURCHASES__),account_trials AS (__TRIALS__),page AS (
 SELECT u.user_id,u.business_id,u.email,u.full_name,u.user_role,u.is_active AS user_active,u.last_login_at,
 b.company_name,b.is_active AS business_active,b.device_quota_limit
 FROM users u JOIN businesses b ON b.business_id=u.business_id ORDER BY b.company_name,u.email,u.user_id LIMIT $1 OFFSET $2
 ) SELECT jsonb_build_object(
 'user_id',u.user_id,'business_id',u.business_id,'email',u.email,'full_name',u.full_name,'user_role',u.user_role,
 'company_name',u.company_name,'is_active',(u.user_active AND u.business_active),'last_login_at',u.last_login_at,
 'presence',CASE WHEN NOT(u.user_active AND u.business_active) THEN 'offline' WHEN pr.online THEN 'online' WHEN pr.connected THEN 'idle' WHEN pr.last_seen_at IS NULL THEN 'unobserved' ELSE 'offline' END,
 'last_seen_at',pr.last_seen_at,'last_active_at',pr.last_active_at,
 'pass',jsonb_build_object('kind',CASE WHEN $3 THEN COALESCE(p.service_key,CASE WHEN tr.trial_expires_at IS NOT NULL THEN 'trial' ELSE 'none' END) ELSE COALESCE(e.tier,'none') END,
 'scope',CASE WHEN $3 THEN 'business' ELSE 'account' END,'activated_at',CASE WHEN $3 AND p.id IS NULL THEN tr.trial_started_at ELSE p.decided_at END,
 'expires_at',CASE WHEN $3 THEN COALESCE(p.activated_expires_at,tr.trial_expires_at) ELSE e.tier_expires_at END,
 'trial_expires_at',tr.trial_expires_at,'paid_expires_at',p.activated_expires_at),
 'limits',jsonb_build_object('scope','business','device_count',usage.devices,'device_quota_limit',u.device_quota_limit,
 'extra_slots',q.purchased_extra_slots,'ai_token_balance',q.ai_token_balance,'hub_pass_expires_at',q.webfig_pass_expires_at,
 'router_count',usage.routers,'router_limit',COALESCE(a.routers,1),'controller_count',usage.controllers,'controller_limit',COALESCE(a.controllers,0),
 'site_count',(SELECT count(*) FROM sites WHERE business_id=u.business_id),'site_limit',COALESCE(a.sites,0),
 'legacy_capacity_enforced',NOT $3),
 'ai_usage_30d',jsonb_build_object('calls',ai.calls,'tokens',ai.tokens),
 'magic_dude_key',EXISTS(SELECT 1 FROM unlock_key_activations k WHERE k.user_id=u.user_id AND k.business_id=u.business_id AND k.key_kind='magic_dude' AND k.consumed_at IS NULL AND k.expires_at>$4)
 )
 FROM page u
 LEFT JOIN account_entitlements e ON e.user_id=u.user_id AND e.business_id=u.business_id
 LEFT JOIN account_trials tr ON tr.business_id=u.business_id
 LEFT JOIN business_metered_quotas q ON q.business_id=u.business_id
 LEFT JOIN device_allowances a ON a.business_id=u.business_id
 LEFT JOIN LATERAL (SELECT id,service_key,decided_at,activated_expires_at FROM account_purchases sp
 WHERE sp.business_id=u.business_id AND sp.status='approved' AND sp.service_key IN ('monthly','annual') AND sp.activated_expires_at IS NOT NULL
 AND ($3 OR (sp.user_id=u.user_id AND sp.service_key=e.tier AND sp.activated_expires_at=e.tier_expires_at))
 ORDER BY sp.activated_expires_at DESC,sp.decided_at DESC,sp.id LIMIT 1) p ON true
 LEFT JOIN LATERAL (SELECT max(s.last_seen_at) AS last_seen_at,max(s.last_active_at) AS last_active_at,
 COALESCE(bool_or(NOT s.revoked AND s.expires_at>$4 AND s.user_role=u.user_role AND s.last_seen_at>$4-interval '90 seconds'),false) AS connected,
 COALESCE(bool_or(NOT s.revoked AND s.expires_at>$4 AND s.user_role=u.user_role AND s.last_seen_at>$4-interval '90 seconds' AND s.last_visible_at>$4-interval '90 seconds' AND s.last_active_at>$4-interval '5 minutes'),false) AS online
 FROM auth_sessions s WHERE s.user_id=u.user_id AND s.business_id=u.business_id) pr ON true
 LEFT JOIN LATERAL (SELECT count(*) AS devices,count(*) FILTER (WHERE deployment_layer IN ('MAIN_GATEWAY','DOWNSTREAM_SWITCH')) AS routers,
 count(*) FILTER (WHERE deployment_layer='CONTROLLER') AS controllers FROM devices WHERE business_id=u.business_id) usage ON true
 LEFT JOIN LATERAL (SELECT count(*) AS calls,COALESCE(sum(total_tokens),0) AS tokens FROM ai_usage_events WHERE business_id=u.business_id AND user_id=u.user_id AND created_at>=$4-interval '30 days') ai ON true
 ORDER BY u.company_name,u.email`
	query = strings.ReplaceAll(strings.ReplaceAll(query, "__PURCHASES__", purchases), "__TRIALS__", trials)
	rows, err := r.db.QueryContext(ctx, query, limit, offset, subscriptionOnly, now)
	if err != nil {
		return nil, 0, fmt.Errorf("platform accounts: %w", err)
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, 0, err
		}
		var a map[string]any
		if err := json.Unmarshal(raw, &a); err != nil {
			return nil, 0, err
		}
		out = append(out, a)
	}
	return out, total, rows.Err()
}
