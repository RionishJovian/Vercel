-- Explicit operator migration; existing bindings need an approver configured.
-- Zero is unconfigured and the guest approval bridge fails closed for it.
BEGIN;
ALTER TABLE business_telegram_bots
    ADD COLUMN IF NOT EXISTS approver_user_id bigint NOT NULL DEFAULT 0
    CHECK (approver_user_id >= 0);
COMMIT;
