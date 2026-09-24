-- Device Fingerprints for Target Identity Verification (Step A)
-- Stores serial, board-name, model, RouterOS version captured at onboarding
-- and verified before every mutating operation.

CREATE TABLE public.device_fingerprints (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL,
  device_type text NOT NULL CHECK (device_type IN ('router', 'switch', 'ap_controller')),
  device_id uuid NOT NULL,  -- router_connections.id or managed_devices.id
  serial text NOT NULL,
  board_name text NOT NULL,
  model text NOT NULL,
  routeros_version text NOT NULL,
  captured_at timestamptz NOT NULL DEFAULT now(),
  verified_at timestamptz,
  UNIQUE (owner_id, device_type, device_id)
);

-- RLS
ALTER TABLE public.device_fingerprints ENABLE ROW LEVEL SECURITY;

CREATE POLICY "device_fingerprints tenant read"
  ON public.device_fingerprints
  FOR SELECT
  TO authenticated
  USING (owner_id = public.effective_owner(auth.uid()));

CREATE POLICY "device_fingerprints tenant insert"
  ON public.device_fingerprints
  FOR INSERT
  TO authenticated
  WITH CHECK (owner_id = public.effective_owner(auth.uid()));

CREATE POLICY "device_fingerprints tenant update"
  ON public.device_fingerprints
  FOR UPDATE
  TO authenticated
  USING (owner_id = public.effective_owner(auth.uid()))
  WITH CHECK (owner_id = public.effective_owner(auth.uid()));

-- Index for lookup
CREATE INDEX device_fingerprints_lookup_idx
  ON public.device_fingerprints (owner_id, device_type, device_id);

-- Service role full access
GRANT ALL ON public.device_fingerprints TO service_role;
GRANT SELECT ON public.device_fingerprints TO authenticated;

-- Audit enhancement: add resolved target identity columns to existing audit tables
ALTER TABLE public.router_save_audit
  ADD COLUMN IF NOT EXISTS resolved_serial text,
  ADD COLUMN IF NOT EXISTS resolved_board_name text,
  ADD COLUMN IF NOT EXISTS fingerprint_match boolean;

ALTER TABLE public.ap_actions_audit
  ADD COLUMN IF NOT EXISTS resolved_serial text,
  ADD COLUMN IF NOT EXISTS resolved_board_name text,
  ADD COLUMN IF NOT EXISTS fingerprint_match boolean;

ALTER TABLE public.deployment_history
  ADD COLUMN IF NOT EXISTS resolved_serial text,
  ADD COLUMN IF NOT EXISTS resolved_board_name text,
  ADD COLUMN IF NOT EXISTS fingerprint_match boolean;

-- Index for audit queries by fingerprint
CREATE INDEX IF NOT EXISTS router_save_audit_fingerprint_idx
  ON public.router_save_audit (owner_id, resolved_serial, created_at DESC);

CREATE INDEX IF NOT EXISTS ap_actions_audit_fingerprint_idx
  ON public.ap_actions_audit (owner_id, resolved_serial, created_at DESC);