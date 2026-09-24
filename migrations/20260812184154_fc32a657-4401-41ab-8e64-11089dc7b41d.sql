DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM cron.job WHERE jobname = 'fleet-ai-scan-hourly') THEN
    PERFORM cron.unschedule('fleet-ai-scan-hourly');
  END IF;
END $$;

SELECT cron.schedule(
  'fleet-ai-scan-hourly',
  '0 * * * *',
  format($cmd$
  select net.http_post(
    url:='https://project--01520fe2-2926-48eb-af86-61b334047e63.lovable.app/api/public/hooks/fleet-ai-scan',
    headers:=jsonb_build_object('Content-Type','application/json','x-cron-secret',%L),
    body:='{}'::jsonb
  ) as request_id;
  $cmd$, (SELECT secret FROM private.cron_secrets WHERE name = 'default'))
);