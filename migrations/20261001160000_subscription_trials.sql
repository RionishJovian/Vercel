-- Explicitly apply before enabling billing.subscription_only.
-- Existing businesses receive one transition trial at rollout. Never extend
-- an existing trial when this migration is re-run.
BEGIN;
CREATE TABLE IF NOT EXISTS business_subscription_trials (
 business_id uuid PRIMARY KEY REFERENCES businesses(business_id) ON DELETE CASCADE,
 trial_started_at timestamptz NOT NULL,
 trial_expires_at timestamptz NOT NULL,
 CONSTRAINT business_trial_duration CHECK (trial_expires_at = trial_started_at + interval '336 hours')
);
ALTER TABLE business_subscription_trials ENABLE ROW LEVEL SECURITY;
ALTER TABLE business_subscription_trials FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS subscription_trials_tenant ON business_subscription_trials;
CREATE POLICY subscription_trials_tenant ON business_subscription_trials
 USING (business_id = nullif(current_setting('app.current_business_id',true),'')::uuid)
 WITH CHECK (business_id = nullif(current_setting('app.current_business_id',true),'')::uuid);
GRANT SELECT, INSERT ON business_subscription_trials TO authenticated;
CREATE INDEX IF NOT EXISTS service_purchases_subscription_expiry
 ON service_purchases(business_id,activated_expires_at DESC)
 WHERE status='approved' AND service_key IN ('monthly','annual');
INSERT INTO business_subscription_trials(business_id,trial_started_at,trial_expires_at)
 SELECT business_id,now(),now()+interval '336 hours' FROM businesses b
 WHERE NOT EXISTS (SELECT 1 FROM service_purchases p WHERE p.business_id=b.business_id
  AND p.status='approved' AND p.service_key IN ('monthly','annual')
  AND p.activated_expires_at>now())
 ON CONFLICT(business_id) DO NOTHING;
COMMIT;
