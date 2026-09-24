-- Emerald / Sapphire base pass: 1 router, 3 sites, 15 AP controllers when no
-- device_allowances row exists yet. Plus add-on raises these via app activation.

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
  _connection_mode text;
BEGIN
  IF NEW.owner_id IS NULL THEN
    RETURN NEW;
  END IF;

  IF _kind = 'routers' THEN
    _connection_mode := to_jsonb(NEW)->>'connection_mode';
    IF _connection_mode = 'sandbox' THEN
      RETURN NEW;
    END IF;
  END IF;

  IF _uid IS NOT NULL AND (
       public.has_role(_uid, 'owner'::public.app_role)
    OR public.has_role(_uid, 'admin'::public.app_role)
  ) THEN
    RETURN NEW;
  END IF;

  PERFORM pg_advisory_xact_lock(hashtext(NEW.owner_id::text || ':' || _kind));

  SELECT CASE _kind
           WHEN 'routers' THEN da.routers
           WHEN 'controllers' THEN da.controllers
           ELSE da.sites
         END
    INTO _max
    FROM public.device_allowances da
   WHERE da.owner_id = NEW.owner_id;

  _max := COALESCE(
    _max,
    CASE _kind
      WHEN 'routers' THEN 1
      WHEN 'sites' THEN 3
      ELSE 15
    END
  );

  IF _kind = 'routers' THEN
    SELECT count(*) INTO _used FROM public.router_connections
     WHERE owner_id = NEW.owner_id
       AND connection_mode IS DISTINCT FROM 'sandbox';
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
