# Relocation from Hostinger to Vultr (2026-11-02)

The app, database and Magic Hub moved from the Hostinger KVM 4 (156.67.216.128) to one
Vultr server, because the Hostinger address range is blocked by some internet providers
and the Hostinger plan cost about $50 a month for a workload that uses about 130 MB of RAM.

## New environment
- Vultr Cloud Compute `vc2-2c-4gb` (2 vCPU, 4 GB, 80 GB), Singapore, Ubuntu 24.04, label `magic-app`.
- Reserved IP **207.148.76.172** (permanent; keep it if the server is rebuilt). Vultr automatic backups on.
- Same layout as before: app in `/opt/mega-magic-go-restored-20261018T100554Z` (the deploy script still depends on that path),
  Docker volumes `mega-magic-go-staging_{db,redis,nats,app}_data`, host Caddy, WireGuard `wg0` (10.77.0.0/24), `mikromagic-hub-peer` service.
- Firewall (ufw): 22/tcp, 80/tcp, 443/tcp+udp, 51820/udp. SSH keys only. fail2ban, unattended-upgrades, chrony.
- Nightly database dump with a restore-list check: `mm-db-backup.timer` (03:10 UTC) into `/var/backups/megamagic`, 7 days kept.
- DNS stays at Hostinger for now (`A @ 207.148.76.172`, TTL 300). Cloudflare is a later, separate step; keep the apex
  DNS-only so WireGuard keeps working.
- Deploys: `VPS_HOST` and `deploy/vps-known-hosts` now point at 207.148.76.172 (new host key pinned).

## What was done, in order
1. Built and hardened the new server; verified the owner's router reaches it.
2. Copied the app directory, secrets, Caddy config and certificates, WireGuard config, hub-peer service and its node binary.
3. Trial run on a hot copy of the data (stack healthy, API ready).
4. Cutover: stopped the old app and database, took a pre-cutover dump (`/root/mm-rollout-backups/pre-vultr-cutover.dump` on the old server),
   brought the old WireGuard down (it saves its peers), copied the volumes cold, started the new stack.
   Verified: all 78 tables have identical row counts; 10 of 10 peers loaded; public readiness OK through the new Caddy.
5. Switched the DNS record, repointed the relay (`magic-relay`, 45.76.153.254) at the new address and cleared its stale
   connection-tracking entry (a forwarding entry keeps the old destination until it is cleared).

## Rollback (while the old server still exists)
Point the DNS `A @` record back to 156.67.216.128, then on the old server:
`cd /opt/mega-magic-go-restored-20261018T100554Z && docker compose -f deploy/compose.go-restore.yaml start && systemctl start wg-quick@wg0`,
repoint the relay's DNAT, restore `VPS_HOST`/known hosts. Data written on the new server after the cutover is not on the old one.

## Still to do
Cancel Hostinger only after a few days of stable running (it expires 2026-11-04 unless renewed). Delete the relay when no router needs it.
Re-enroll routers that need a fresh key. Cloudflare in front of the website.
