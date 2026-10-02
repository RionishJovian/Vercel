-- Apply once after the connector pilot migration, before deploying dashboard jobs.
BEGIN;
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '30s';
ALTER TABLE public.magic_media_jobs DROP CONSTRAINT magic_media_jobs_operation_check;
ALTER TABLE public.magic_media_jobs ADD CONSTRAINT magic_media_jobs_operation_check
 CHECK (operation IN ('status','libraries','add_library','scan','dashboard'));
COMMIT;
