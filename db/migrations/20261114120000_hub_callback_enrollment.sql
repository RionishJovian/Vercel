-- Wave 3 (B2/B4): router-generated WireGuard key enrolment and durable peer state.
--
-- hub_callback_tokens: one live, single-use, short-lived token per router. Only the
--   SHA-256 of the token is stored. The router presents the token with its PUBLIC key;
--   the private key is created on the router and never reaches the server.
-- hub_peer_state: what the app knows about a router's Hub allocation (public key,
--   /32 address, endpoint, last verified handshake). No private key material.
--   The unique indexes are the database backstop that a /32 or a router key is never
--   assigned to two routers on the same Hub, whatever the application does.
--
-- Additive only: existing releases ignore these tables, so a rollback keeps working.
-- Like hub_pending_enrollments, both are service-only: the tenant SQL role has no
-- access, and the runtime service role gets its policy from deploy/runtime-db-role.sql.

CREATE UNIQUE INDEX IF NOT EXISTS devices_migration_tenant_identity ON devices(device_id,business_id);

CREATE TABLE hub_callback_tokens (
 business_id uuid NOT NULL,
 device_id uuid NOT NULL,
 token_hash bytea NOT NULL,
 requested_by uuid NOT NULL,
 expires_at timestamptz NOT NULL,
 consumed_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(business_id,device_id),
 CONSTRAINT hub_callback_tokens_hash_unique UNIQUE(token_hash),
 CONSTRAINT hub_callback_tokens_hash_size CHECK(octet_length(token_hash)=32),
 FOREIGN KEY(device_id,business_id) REFERENCES devices(device_id,business_id) ON DELETE CASCADE
);

CREATE TABLE hub_peer_state (
 business_id uuid NOT NULL,
 device_id uuid NOT NULL,
 peer_id text NOT NULL,
 router_public_key text NOT NULL,
 tunnel_address text NOT NULL,
 endpoint text NOT NULL,
 enrolled_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 last_handshake_at timestamptz,
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(business_id,device_id),
 CONSTRAINT hub_peer_state_key_shape CHECK(length(router_public_key)=44),
 CONSTRAINT hub_peer_state_address_shape CHECK(tunnel_address ~ '^[0-9]{1,3}(\.[0-9]{1,3}){3}/32$'),
 CONSTRAINT hub_peer_state_address_unique UNIQUE(endpoint,tunnel_address),
 CONSTRAINT hub_peer_state_key_unique UNIQUE(endpoint,router_public_key),
 FOREIGN KEY(device_id,business_id) REFERENCES devices(device_id,business_id) ON DELETE CASCADE
);

ALTER TABLE hub_callback_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE hub_callback_tokens FORCE ROW LEVEL SECURITY;
ALTER TABLE hub_peer_state ENABLE ROW LEVEL SECURITY;
ALTER TABLE hub_peer_state FORCE ROW LEVEL SECURITY;
REVOKE ALL ON hub_callback_tokens, hub_peer_state FROM PUBLIC, authenticated;
