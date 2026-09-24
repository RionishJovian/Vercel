CREATE OR REPLACE FUNCTION public.enforce_device_quota_trigger()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  _kind text := TG_ARGV[0];
  _max integer;
  _used integer;
  _uid uuid := auth.uid();
BEGIN
  IF NEW.owner_id IS NULL THEN
    RETURN NEW;
  END IF;

  -- Owner / admin accounts have no device limits.
  IF _uid IS NOT NULL AND (
       public.has_role(_uid, 'owner'::public.app_role)
    OR public.has_role(_uid, 'admin'::public.app_role)
  ) THEN
    RETURN NEW;
  END IF;

  -- Serialize concurrent inserts for this owner + device kind so that two
  -- simultaneous requests cannot both pass the check.
  PERFORM pg_advisory_xact_lock(hashtext(NEW.owner_id::text || ':' || _kind));

  SELECT CASE _kind
           WHEN 'routers' THEN da.routers
           WHEN 'controllers' THEN da.controllers
           ELSE da.sites
         END
    INTO _max
    FROM public.device_allowances da
   WHERE da.owner_id = NEW.owner_id;

  _max := COALESCE(_max, 1);

  IF _kind = 'routers' THEN
    SELECT count(*) INTO _used FROM public.router_connections WHERE owner_id = NEW.owner_id;
  ELSIF _kind = 'controllers' THEN
    SELECT count(*) INTO _used FROM public.unifi_controllers WHERE owner_id = NEW.owner_id;
  ELSE
    SELECT count(*) INTO _used FROM public.sites WHERE owner_id = NEW.owner_id;
  END IF;

  IF _used >= _max THEN
    RAISE EXCEPTION 'DEVICE_QUOTA_EXCEEDED: this account already uses % of % allowed %.', _used, _max, _kind
      USING ERRCODE = 'check_violation';
  END IF;

  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_quota_router_connections ON public.router_connections;
CREATE TRIGGER trg_quota_router_connections
BEFORE INSERT ON public.router_connections
FOR EACH ROW EXECUTE FUNCTION public.enforce_device_quota_trigger('routers');

DROP TRIGGER IF EXISTS trg_quota_unifi_controllers ON public.unifi_controllers;
CREATE TRIGGER trg_quota_unifi_controllers
BEFORE INSERT ON public.unifi_controllers
FOR EACH ROW EXECUTE FUNCTION public.enforce_device_quota_trigger('controllers');

DROP TRIGGER IF EXISTS trg_quota_sites ON public.sites;
CREATE TRIGGER trg_quota_sites
BEFORE INSERT ON public.sites
FOR EACH ROW EXECUTE FUNCTION public.enforce_device_quota_trigger('sites');