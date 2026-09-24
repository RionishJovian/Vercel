-- Magic Hub rows historically stored connection_mode = 'cloud'.
-- Rename to 'hub' so the DB value matches the product name and ConnMethod.
UPDATE public.router_connections
SET connection_mode = 'hub'
WHERE connection_mode = 'cloud';
