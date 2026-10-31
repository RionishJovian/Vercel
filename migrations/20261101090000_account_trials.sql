-- Seven-day trial for self-registered accounts. Apply before enabling self registration.
-- A trial is access only: it never creates a paid pass, and it is granted once per login.
BEGIN;
CREATE TABLE app_account_trials (
 user_id uuid PRIMARY KEY,
 business_id uuid NOT NULL,
 started_at timestamptz NOT NULL DEFAULT now(),
 expires_at timestamptz NOT NULL,
 CONSTRAINT app_account_trial_duration CHECK (expires_at = started_at + interval '7 days'),
 FOREIGN KEY(user_id,business_id) REFERENCES users(user_id,business_id)
);
ALTER TABLE app_account_trials ENABLE ROW LEVEL SECURITY;
ALTER TABLE app_account_trials FORCE ROW LEVEL SECURITY;
CREATE POLICY app_account_trials_tenant ON app_account_trials
 USING (business_id = NULLIF(current_setting('app.current_business_id',true),'')::uuid)
 WITH CHECK (business_id = NULLIF(current_setting('app.current_business_id',true),'')::uuid);
GRANT SELECT, INSERT ON app_account_trials TO authenticated;
COMMIT;
