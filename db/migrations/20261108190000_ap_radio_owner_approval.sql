-- AP optimization review ledger. This records owner approval only; it
-- DOES NOT configure radios. No vendor writer is wired to these records.
CREATE TABLE IF NOT EXISTS ap_radio_proposals (
  id uuid NOT NULL DEFAULT gen_random_uuid(),
  business_id uuid NOT NULL REFERENCES businesses(business_id),
  device_id uuid NOT NULL,
  proposal jsonb NOT NULL CHECK (jsonb_typeof(proposal) = 'object'),
  proposal_hash text NOT NULL CHECK (proposal_hash ~ '^[0-9a-f]{64}$'),
  fingerprint text NOT NULL CHECK (fingerprint ~ '^[0-9a-f]{64}$'),
  status text NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending', 'approved', 'rejected', 'consumed', 'expired')),
  created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  expires_at timestamptz NOT NULL,
  decided_by uuid,
  decided_at timestamptz,
  consumed_at timestamptz,
  PRIMARY KEY(business_id,id),
  FOREIGN KEY(device_id,business_id) REFERENCES devices(device_id,business_id),
  CHECK(expires_at > created_at AND expires_at <= created_at + interval '5 minutes'),
  CHECK (
    (status IN ('pending','expired') AND decided_by IS NULL AND decided_at IS NULL)
    OR (status IN ('approved','rejected','consumed') AND decided_by IS NOT NULL AND decided_at IS NOT NULL)
  ),
  CHECK ((status='consumed')=(consumed_at IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS ap_radio_proposals_open
  ON ap_radio_proposals(business_id,expires_at DESC)
  WHERE status IN ('pending','approved');

-- Reject changes to the owner-visible proposal, its expiry or its identity.
-- A reviewed approval can be consumed just once, never set back to pending.
CREATE OR REPLACE FUNCTION guard_ap_radio_proposal() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF ROW(NEW.business_id,NEW.id,NEW.device_id,NEW.proposal,NEW.proposal_hash,
      NEW.fingerprint,NEW.created_at,NEW.expires_at)
     IS DISTINCT FROM
     ROW(OLD.business_id,OLD.id,OLD.device_id,OLD.proposal,OLD.proposal_hash,
      OLD.fingerprint,OLD.created_at,OLD.expires_at) THEN
    RAISE EXCEPTION 'AP proposal identity/review evidence is immutable';
  END IF;
  IF NOT (
     (OLD.status='pending' AND NEW.status IN ('approved','rejected','expired'))
     OR (OLD.status='approved' AND NEW.status IN ('consumed','expired'))
  ) THEN
    RAISE EXCEPTION 'invalid or replayed AP proposal state transition';
  END IF;
  IF OLD.status='approved' AND
     (NEW.decided_by IS DISTINCT FROM OLD.decided_by OR NEW.decided_at IS DISTINCT FROM OLD.decided_at) THEN
    RAISE EXCEPTION 'owner approval identity is immutable';
  END IF;
  IF NEW.status IN ('approved','rejected') AND
     (NEW.decided_by IS NULL OR NEW.decided_at IS NULL OR OLD.status<>'pending') THEN
    RAISE EXCEPTION 'explicit owner decision is required';
  END IF;
  IF NEW.status='consumed' AND NEW.consumed_at IS NULL THEN
    RAISE EXCEPTION 'consumption must be recorded';
  END IF;
  RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS ap_radio_proposal_guard ON ap_radio_proposals;
CREATE TRIGGER ap_radio_proposal_guard BEFORE UPDATE ON ap_radio_proposals
 FOR EACH ROW EXECUTE FUNCTION guard_ap_radio_proposal();

ALTER TABLE ap_radio_proposals ENABLE ROW LEVEL SECURITY;
ALTER TABLE ap_radio_proposals FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS ap_radio_proposals_tenant ON ap_radio_proposals;
CREATE POLICY ap_radio_proposals_tenant ON ap_radio_proposals FOR ALL TO authenticated
 USING(business_id=NULLIF(current_setting('app.current_business_id',true),'')::uuid)
 WITH CHECK(business_id=NULLIF(current_setting('app.current_business_id',true),'')::uuid);
REVOKE ALL ON ap_radio_proposals FROM PUBLIC;
GRANT SELECT,INSERT,UPDATE ON ap_radio_proposals TO authenticated;
