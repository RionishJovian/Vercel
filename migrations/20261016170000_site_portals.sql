-- Site-owned captive portal drafts and published revisions.
-- The legacy portal_configs row remains the business fallback during rollout.
CREATE UNIQUE INDEX IF NOT EXISTS sites_site_business_unique ON sites(site_id, business_id);

CREATE TABLE IF NOT EXISTS site_portals (
    site_id uuid PRIMARY KEY REFERENCES sites(site_id) ON DELETE CASCADE,
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    draft_config jsonb NOT NULL DEFAULT '{}'::jsonb,
    published_config jsonb NOT NULL DEFAULT '{}'::jsonb,
    draft_revision bigint NOT NULL DEFAULT 1 CHECK (draft_revision > 0),
    published_revision bigint NOT NULL DEFAULT 0 CHECK (published_revision >= 0),
    published_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT site_portals_business_site_fk FOREIGN KEY (site_id, business_id)
        REFERENCES sites(site_id, business_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS site_portals_business_idx ON site_portals(business_id);

GRANT SELECT, INSERT, UPDATE, DELETE ON site_portals TO authenticated;
ALTER TABLE site_portals ENABLE ROW LEVEL SECURITY;
CREATE POLICY site_portals_tenant_isolation ON site_portals
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);
