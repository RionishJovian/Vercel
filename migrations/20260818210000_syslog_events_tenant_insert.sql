-- Magic Hub / Local Connector log sync inserts via the user JWT client.
-- syslog_events had SELECT/UPDATE/DELETE policies only; HTTPS ingest used
-- service_role, so paired /log pulls failed with RLS on insert.

GRANT SELECT, INSERT, UPDATE, DELETE ON public.syslog_events TO authenticated;

DROP POLICY IF EXISTS "tenant insert syslog" ON public.syslog_events;
CREATE POLICY "tenant insert syslog" ON public.syslog_events
  FOR INSERT TO authenticated
  WITH CHECK (
    owner_id = public.effective_owner(auth.uid())
    AND NOT public.has_role(auth.uid(), 'read_only')
    AND NOT public.has_role(auth.uid(), 'expired')
  );
