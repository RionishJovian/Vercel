-- Blueprint v0.2: explicit tenant membership and staff invitations.
-- Existing users remain valid; memberships are backfilled from users so this
-- migration is additive and does not change current login behavior.

CREATE TABLE IF NOT EXISTS business_memberships (
    membership_id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    role text NOT NULL CHECK (role IN ('OWNER', 'ADMIN', 'OPERATOR', 'TECHNICIAN', 'VIEWER', 'SUPPORT')),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'revoked')),
    invited_by uuid REFERENCES users(user_id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (business_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_business_memberships_user
    ON business_memberships(user_id, status);
CREATE INDEX IF NOT EXISTS idx_business_memberships_business
    ON business_memberships(business_id, status, role);

INSERT INTO business_memberships (business_id, user_id, role, status, created_at, updated_at)
SELECT u.business_id, u.user_id, u.user_role, CASE WHEN u.is_active THEN 'active' ELSE 'revoked' END,
       u.created_at, u.updated_at
FROM users u
WHERE u.user_role IN ('OWNER', 'ADMIN', 'OPERATOR', 'TECHNICIAN', 'VIEWER', 'SUPPORT')
  AND NOT EXISTS (
      SELECT 1 FROM business_memberships m
      WHERE m.business_id = u.business_id AND m.user_id = u.user_id
  );

CREATE TABLE IF NOT EXISTS business_invitations (
    invitation_id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
    email text NOT NULL,
    full_name text NOT NULL,
    role text NOT NULL CHECK (role IN ('ADMIN', 'OPERATOR', 'TECHNICIAN', 'VIEWER', 'SUPPORT')),
    token_hash text NOT NULL UNIQUE,
    invited_by uuid NOT NULL REFERENCES users(user_id) ON DELETE RESTRICT,
    expires_at timestamptz NOT NULL,
    accepted_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (accepted_at IS NULL OR revoked_at IS NULL),
    CHECK (length(btrim(email)) > 0),
    CHECK (length(btrim(full_name)) > 0)
);

CREATE INDEX IF NOT EXISTS idx_business_invitations_business
    ON business_invitations(business_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_business_invitations_email
    ON business_invitations(lower(email), expires_at);

CREATE UNIQUE INDEX IF NOT EXISTS uq_business_pending_invitation
    ON business_invitations(business_id, lower(email))
    WHERE accepted_at IS NULL AND revoked_at IS NULL;
