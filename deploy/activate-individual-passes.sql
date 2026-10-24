-- Apply the additive migration first, then run this once at enforcement rollout.
-- Re-running preserves the original deployment clock and transition deadlines.
BEGIN;
SELECT pg_advisory_xact_lock(hashtextextended('individual-pass-rollout',0));
DO $$ BEGIN
 IF EXISTS(
  SELECT 1 FROM businesses b WHERE b.is_active
  AND EXISTS(SELECT 1 FROM users u WHERE u.business_id=b.business_id AND u.is_active)
  AND NOT EXISTS(SELECT 1 FROM users u WHERE u.business_id=b.business_id AND u.is_active AND u.user_role='DEVELOPER')
  AND NOT EXISTS(SELECT 1 FROM business_app_primary_users p JOIN users u ON u.user_id=p.user_id AND u.business_id=p.business_id WHERE p.business_id=b.business_id AND u.is_active AND u.user_role='OWNER')
 ) THEN RAISE EXCEPTION 'Review active client workspaces without an authorized owner login before enforcement'; END IF;
END $$;
INSERT INTO app_account_pass_rollouts(rollout_key,starts_at,transition_days)
 VALUES('individual-v1',clock_timestamp(),7) ON CONFLICT(rollout_key) DO NOTHING;
INSERT INTO app_account_pass_transitions(user_id,business_id,rollout_key,expires_at)
 SELECT u.user_id,u.business_id,r.rollout_key,r.starts_at+interval '168 hours'
 FROM users u CROSS JOIN app_account_pass_rollouts r
 WHERE r.rollout_key='individual-v1' AND u.created_at<=r.starts_at AND u.user_role<>'DEVELOPER'
 ON CONFLICT(user_id) DO NOTHING;
UPDATE app_account_pass_rollouts SET enforcement_enabled=true WHERE rollout_key='individual-v1';
COMMIT;
