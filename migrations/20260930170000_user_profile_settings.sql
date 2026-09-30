-- Personal account presentation and preferences. Images are deliberately
-- bounded in both the HTTP handler and the database so a profile upload cannot
-- become an unbounded storage path.
CREATE TABLE IF NOT EXISTS user_profile_settings (
    user_id uuid PRIMARY KEY REFERENCES users(user_id) ON DELETE CASCADE,
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    language text NOT NULL DEFAULT 'en' CHECK (language IN ('en', 'my', 'zh')),
    avatar_data bytea CHECK (avatar_data IS NULL OR octet_length(avatar_data) <= 2097152),
    avatar_mime text CHECK (avatar_mime IS NULL OR avatar_mime IN ('image/jpeg', 'image/png', 'image/webp')),
    cover_data bytea CHECK (cover_data IS NULL OR octet_length(cover_data) <= 5242880),
    cover_mime text CHECK (cover_mime IS NULL OR cover_mime IN ('image/jpeg', 'image/png', 'image/webp')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_user_profile_settings_business
    ON user_profile_settings(business_id);

ALTER TABLE user_profile_settings ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS user_profile_settings_tenant_isolation ON user_profile_settings;
CREATE POLICY user_profile_settings_tenant_isolation ON user_profile_settings
    FOR ALL TO authenticated
    USING (business_id = current_setting('app.current_business_id')::uuid)
    WITH CHECK (business_id = current_setting('app.current_business_id')::uuid);

GRANT SELECT, INSERT, UPDATE, DELETE ON user_profile_settings TO authenticated;
