CREATE TABLE IF NOT EXISTS private.cron_secrets (
  name text PRIMARY KEY,
  secret text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

REVOKE ALL ON private.cron_secrets FROM anon, authenticated;
GRANT SELECT ON private.cron_secrets TO service_role;

INSERT INTO private.cron_secrets (name, secret)
VALUES ('default', encode(gen_random_bytes(32), 'hex'))
ON CONFLICT (name) DO NOTHING;

SELECT cron.alter_job(
  5,
  command := format($cmd$
  select net.http_post(
    url:='https://project--01520fe2-2926-48eb-af86-61b334047e63.lovable.app/api/public/hooks/voucher-maintenance',
    headers:=jsonb_build_object('Content-Type','application/json','x-cron-secret',%L),
    body:='{}'::jsonb
  ) as request_id;
  $cmd$, (SELECT secret FROM private.cron_secrets WHERE name = 'default'))
);