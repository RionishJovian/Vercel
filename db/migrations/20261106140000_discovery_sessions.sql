-- Discovery sessions: a short pairing between the owner's console and a local helper that listens for nearby
-- routers. Everything here is an unverified observation; it never authenticates or configures a router.
-- Rows are deleted soon after a session ends (see the sweeper in internal/discoverystore).
CREATE TABLE discovery_sessions (
 session_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
 site_id uuid,
 created_by uuid NOT NULL,
 state text NOT NULL DEFAULT 'open' CHECK (state IN ('open','paired','cancelled','closed')),
 short_id text NOT NULL CHECK (short_id ~ '^[A-HJKMNP-TV-Z]{6}$'),
 pair_hash text,
 pair_expires_at timestamptz,
 pair_attempts integer NOT NULL DEFAULT 0 CHECK (pair_attempts >= 0),
 token_hash text,
 token_expires_at timestamptz,
 expires_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 paired_at timestamptz,
 last_upload_at timestamptz,
 ended_at timestamptz,
 CONSTRAINT discovery_session_length CHECK (expires_at <= created_at + interval '15 minutes')
);
-- One live session per business, and a short id is unique while it can still be used.
CREATE UNIQUE INDEX discovery_sessions_one_live ON discovery_sessions(business_id) WHERE state IN ('open','paired');
CREATE UNIQUE INDEX discovery_sessions_short_live ON discovery_sessions(short_id) WHERE state IN ('open','paired');
CREATE INDEX discovery_sessions_token ON discovery_sessions(token_hash) WHERE token_hash IS NOT NULL;
CREATE INDEX discovery_sessions_sweep ON discovery_sessions(expires_at);

CREATE TABLE discovery_observations (
 session_id uuid NOT NULL REFERENCES discovery_sessions(session_id) ON DELETE CASCADE,
 business_id uuid NOT NULL,
 device_key text NOT NULL CHECK (device_key ~ '^(sw:[0-9a-f]{16}|mac:[0-9A-F]{2}(:[0-9A-F]{2}){5})$'),
 macs text[] NOT NULL DEFAULT '{}' CHECK (cardinality(macs) <= 8),
 identity text CHECK (length(identity) <= 64),
 board text CHECK (length(board) <= 64),
 platform text CHECK (length(platform) <= 64),
 version text CHECK (length(version) <= 64),
 interface_name text CHECK (length(interface_name) <= 64),
 claimed_ip inet,
 source_ip inet,
 ip_mismatch boolean NOT NULL DEFAULT false,
 ip_changed boolean NOT NULL DEFAULT false,
 heard_count integer NOT NULL DEFAULT 1 CHECK (heard_count >= 1),
 first_received_at timestamptz NOT NULL DEFAULT now(),
 last_received_at timestamptz NOT NULL DEFAULT now(),
 selected_at timestamptz,
 PRIMARY KEY (session_id, device_key)
);
CREATE INDEX discovery_observations_business ON discovery_observations(business_id);

ALTER TABLE discovery_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE discovery_sessions FORCE ROW LEVEL SECURITY;
CREATE POLICY discovery_sessions_tenant ON discovery_sessions
 USING (business_id = NULLIF(current_setting('app.current_business_id',true),'')::uuid)
 WITH CHECK (business_id = NULLIF(current_setting('app.current_business_id',true),'')::uuid);
ALTER TABLE discovery_observations ENABLE ROW LEVEL SECURITY;
ALTER TABLE discovery_observations FORCE ROW LEVEL SECURITY;
CREATE POLICY discovery_observations_tenant ON discovery_observations
 USING (business_id = NULLIF(current_setting('app.current_business_id',true),'')::uuid)
 WITH CHECK (business_id = NULLIF(current_setting('app.current_business_id',true),'')::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON discovery_sessions, discovery_observations TO authenticated;
