BEGIN;
ALTER TABLE public.device_discovery_labels ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.device_discovery_labels FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS device_discovery_labels_tenant_isolation ON public.device_discovery_labels;
CREATE POLICY device_discovery_labels_tenant_isolation ON public.device_discovery_labels
 TO authenticated
 USING (business_id = NULLIF(current_setting('app.current_business_id',true),'')::uuid)
 WITH CHECK (business_id = NULLIF(current_setting('app.current_business_id',true),'')::uuid);
GRANT SELECT,INSERT,UPDATE,DELETE ON public.device_discovery_labels TO authenticated;
CREATE UNIQUE INDEX IF NOT EXISTS devices_id_business_key ON public.devices(device_id,business_id);
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='public.device_discovery_labels'::regclass AND conname='discovery_label_device_tenant') THEN
  ALTER TABLE public.device_discovery_labels ADD CONSTRAINT discovery_label_device_tenant
   FOREIGN KEY(device_id,business_id) REFERENCES public.devices(device_id,business_id) ON DELETE CASCADE NOT VALID;
 END IF;
END $$;
ALTER TABLE public.device_discovery_labels VALIDATE CONSTRAINT discovery_label_device_tenant;
COMMIT;
