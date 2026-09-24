DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'sandbox_exec') THEN
    GRANT DELETE, UPDATE ON public.router_connections TO sandbox_exec;
    GRANT DELETE, UPDATE ON public.unifi_controllers TO sandbox_exec;
    GRANT DELETE, UPDATE ON public.sites TO sandbox_exec;
    GRANT DELETE, UPDATE ON public.connectors TO sandbox_exec;
    GRANT DELETE, UPDATE ON public.device_allowances TO sandbox_exec;
    GRANT DELETE ON public.ap_devices TO sandbox_exec;
  END IF;
END $$;