-- Hosted design only: publishing never grants paid playback.
BEGIN;
CREATE TABLE magic_media_publications (
 business_id uuid PRIMARY KEY REFERENCES businesses(business_id) ON DELETE CASCADE,
 public_id text NOT NULL UNIQUE CHECK (public_id ~ '^[0-9a-f]{64}$'),
 design jsonb NOT NULL CHECK (jsonb_typeof(design)='object' AND octet_length(design::text)<=3145728),
 revision bigint NOT NULL DEFAULT 1 CHECK (revision>0),
 published_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE magic_media_publications ENABLE ROW LEVEL SECURITY;
ALTER TABLE magic_media_publications FORCE ROW LEVEL SECURITY;
CREATE POLICY media_publications_tenant ON magic_media_publications
 USING (business_id=nullif(current_setting('app.current_business_id',true),'')::uuid)
 WITH CHECK (business_id=nullif(current_setting('app.current_business_id',true),'')::uuid);
GRANT SELECT,INSERT,UPDATE,DELETE ON magic_media_publications TO authenticated;
COMMIT;
