REVOKE ALL ON FUNCTION public.handle_new_user() FROM PUBLIC, anon, authenticated;
REVOKE ALL ON FUNCTION public.expire_stale_clients() FROM PUBLIC, anon, authenticated;
REVOKE ALL ON FUNCTION public.notify_owners_client_expired() FROM PUBLIC, anon, authenticated;
GRANT EXECUTE ON FUNCTION public.expire_stale_clients() TO service_role;