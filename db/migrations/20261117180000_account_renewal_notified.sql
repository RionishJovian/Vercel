-- Platform Telegram alert for an app pass request: remember that it was delivered, so an undelivered one is retried.
ALTER TABLE app_account_pass_renewals ADD COLUMN IF NOT EXISTS notified_at timestamptz;
-- Decided requests never need an alert; pending ones stay NULL and are retried for seven days.
UPDATE app_account_pass_renewals SET notified_at = COALESCE(decided_at, created_at) WHERE status <> 'pending' AND notified_at IS NULL;
