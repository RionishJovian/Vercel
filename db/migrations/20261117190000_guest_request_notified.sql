-- Owner Telegram alert for a paid guest request: remember it was delivered, so an undelivered one is retried.
ALTER TABLE guest_purchase_requests ADD COLUMN IF NOT EXISTS notified_at timestamptz;
-- Decided requests never need an alert; pending ones with payment details stay NULL and are retried for seven days.
UPDATE guest_purchase_requests SET notified_at = COALESCE(decided_at, updated_at) WHERE status <> 'pending' AND notified_at IS NULL;
