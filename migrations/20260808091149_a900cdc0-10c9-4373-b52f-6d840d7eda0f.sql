
-- device_allowances: scope writes to the acting tenant
DROP POLICY IF EXISTS "allowances managed by owners" ON public.device_allowances;
CREATE POLICY "allowances managed by owners"
ON public.device_allowances
FOR ALL
TO authenticated
USING (
  owner_id = public.effective_owner(auth.uid())
  AND (public.has_role(auth.uid(), 'owner'::public.app_role) OR public.has_role(auth.uid(), 'admin'::public.app_role))
)
WITH CHECK (
  owner_id = public.effective_owner(auth.uid())
  AND (public.has_role(auth.uid(), 'owner'::public.app_role) OR public.has_role(auth.uid(), 'admin'::public.app_role))
);

-- device_requests: decisions limited to the acting tenant
DROP POLICY IF EXISTS "requests decided by owners" ON public.device_requests;
CREATE POLICY "requests decided by owners"
ON public.device_requests
FOR UPDATE
TO authenticated
USING (
  owner_id = public.effective_owner(auth.uid())
  AND (public.has_role(auth.uid(), 'owner'::public.app_role) OR public.has_role(auth.uid(), 'admin'::public.app_role))
)
WITH CHECK (
  owner_id = public.effective_owner(auth.uid())
  AND (public.has_role(auth.uid(), 'owner'::public.app_role) OR public.has_role(auth.uid(), 'admin'::public.app_role))
);

-- portal-assets: align write policies with the read policy's tenant folder scoping
DROP POLICY IF EXISTS "portal-assets owner insert" ON storage.objects;
DROP POLICY IF EXISTS "portal-assets owner update" ON storage.objects;
DROP POLICY IF EXISTS "portal-assets owner delete" ON storage.objects;

CREATE POLICY "portal-assets owner insert"
ON storage.objects
FOR INSERT
TO authenticated
WITH CHECK (
  bucket_id = 'portal-assets'
  AND (storage.foldername(name))[1] = (public.effective_owner(auth.uid()))::text
);

CREATE POLICY "portal-assets owner update"
ON storage.objects
FOR UPDATE
TO authenticated
USING (
  bucket_id = 'portal-assets'
  AND (storage.foldername(name))[1] = (public.effective_owner(auth.uid()))::text
)
WITH CHECK (
  bucket_id = 'portal-assets'
  AND (storage.foldername(name))[1] = (public.effective_owner(auth.uid()))::text
);

CREATE POLICY "portal-assets owner delete"
ON storage.objects
FOR DELETE
TO authenticated
USING (
  bucket_id = 'portal-assets'
  AND (storage.foldername(name))[1] = (public.effective_owner(auth.uid()))::text
);
