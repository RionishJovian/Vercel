-- Stage 1A — actionable AI findings.
-- Adds incidents.suggested_action (jsonb; NULL when absent) and re-asserts the
-- table grant so a generic ALL-TABLES grant that ran earlier cannot leave this
-- table ungranted. Idempotent: safe to re-run.

ALTER TABLE incidents ADD COLUMN IF NOT EXISTS suggested_action jsonb;

GRANT SELECT, INSERT, UPDATE, DELETE ON incidents TO authenticated;
