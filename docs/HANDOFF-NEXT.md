# Handoff for the next session (2026-11-01)

Start here. Everything below is the current state; nothing else is needed from earlier chats.

## What this is
Mega Magic: multi-tenant cloud control plane for MikroTik RouterBOARD owners (Starlink -> RouterBOARD -> neighbourhood hotspot). The captive portal runs on the router; the cloud (Magic Hub, WireGuard) only manages: issues voucher codes, installs them on the router, reads usage back. Guests redeem codes on the router login page, never in the cloud. Each client: one login, one site, one router by default; extras (router/site, Jellyfin dashboard, future features) are sold through the Store catalog managed by the platform admin.
Jellyfin = media server the owner runs on their own PC. Jellyfin vouchers give media playback from that host only, no internet; separate page, stock, batches and shops from internet vouchers.

## State (2026-11-01)
- Everything below is merged to `main` and deployed to https://megamagic.tech (GitHub one-click deploy works). PRs #1-#19 are merged; #7 was reverted by #11 because hosted bank-transfer payment needs the public guest routes.
- **Console:** the React app in `app/` (Vite, React 19, TypeScript, SWR), served at `/v2/` with hash routes, Burmese/English, Day/Night, Jade Glass look. It replaced the old `web/app.html` and the vanilla `web/v2`, which were removed (PR #19 and the asset cleanup). `/app.html` redirects to `/v2/`. All former features are ported, including the voucher guest-page editor, Jellyfin guest page, Hub, Internet setup, topology and Magic Dude.
- **Static pages in `web/`:** landing (`index.html`, animated logo, phone recording of the guest page), `pricing.html` (prices only from `GET /api/v1/pricing`), `signin.html` (sign in and create account), `portal.html` (hosted guest page). Shared: `assets/js/api.js`, `site.js`, `assets/css/landing.css`, `site-pages.css`, `media-guest.*`.
- **Self registration:** `POST /auth/register` creates a business, owner and authorized login. With individual passes enforced the new login gets one 7-day trial (`app_account_trials`, migration `20261101090000_account_trials.sql` applied to production 2026-10-31). No email verification or CAPTCHA (accepted by the owner). `billing.self_registration_enabled=false` closes sign-ups. No real test account has been created on production.
- **Build:** `Dockerfile.web.prod` builds `app/` in a Node stage into `/srv/v2`; the Vercel build (`scripts/prepare-vercel-build.mjs`) also builds it into `vercel-public/v2`. CI validate runs the app tests and build, the Go tests, `test:deployment`, `test:vercel` and `test:critical-ui`.
- Migrations are never run by the deploy; apply them by hand with a backup first (see the VPS section).

## VPS (Hostinger KVM 4, srv2032937, 156.67.216.128)
- **Expires 2026-11-04. Renew it.**
- Live compose project `mega-magic-go-restored` (api, web, db, redis, nats), host Caddy, WireGuard hub on the host (wg0 10.77.0.1/24, UDP 51820/51821). Live data volumes are named `mega-magic-go-staging_*` (misleading; never clean up by that name).
- Backups: take one before any migration with `/usr/local/sbin/mm-db-backup.sh`. Nightly DB backup: `/usr/local/sbin/mm-db-backup.sh`, timer `mm-db-backup.timer` 20:00 UTC, output `/opt/backups/nightly` (never auto-deleted).
- Moved (not deleted) to `/root/_to_delete/20261031` with MANIFEST.txt. Stage B cleanup not done (`build21` volumes, `mm-*` source copies, `/root/go`).
- Console access: Hostinger "Web console" opens a new tab and must be clicked by the owner. The owner's VPS console is reachable only from an AU IP (known issue, fix later).

## Jellyfin test (owner's MacBook + hAP ax2)
- Jellyfin Server 12.4 (arm64) on the MacBook, server `Nishs-MacBook-Pro-6`, library "Magic Media - disposable acceptance", port 8096. Mac is on router `ether2`, static lease 192.168.88.254 (MAC 00:E0:4C:36:08:D9). macOS firewall allow not done yet.
- Router `hAP ax2 lab`, device id `683a4cee-97fb-4a88-a4b4-f142fdbdc78c`, hub address 10.77.0.8, RouterOS 7.24.2. No USB, cannot host Jellyfin. Solar powered: keep power-ons short and planned.
- WAN: `wifi1` is a station on the home Wi-Fi (192.168.100.210). LAN bridge 192.168.88.1/24. Hotspot `mm-lab-hotspot` on `mm-cvp-guest` (SSID "Magic Cafe", 10.5.50.1/24), profile `mm-lab-hsprof`.
- Done on the router by the owner: static lease; `wifi2` set to AP (SSID mm-lab-admin, passphrase set, channel 2437/20 MHz); `mm-cvp-guest` master moved to `wifi2`.
- **Open problem:** `wifi2` and `mm-cvp-guest` still did not show the running flag (R) after the change; cause unknown. Next power-on: `/interface/wifi print`, then `/log print where topics~"wifi"` (a country setting may be needed; that is the owner's choice). The app's Wi-Fi writes are held server-side (`wifi_write_enabled=false`).
- Read-only router data is available while signed in: `/api/v1/routers/<id>/telemetry` and `/network/inventory`.
- Proposed router rules (not applied): hotspot user profile `mega-magic-media-v1` with `address-list=mm-media-guests`; forward accept `src-address-list=mm-media-guests dst-address=192.168.88.254 protocol=tcp dst-port=8096`, then forward drop for that list; dst-nat from hub 10.77.0.1 on the WireGuard interface to 192.168.88.254:8096. Test steps: media code reaches only Jellyfin; internet code on another phone unaffected; reboot persistence; remove all `mm-` items to end.
- Cloud side still to build: `product=media` vouchers (`plan_key='guest-media'`), fail-closed readiness check, review/apply step, separate reports, per-site Jellyfin host settings. Only after the router test passes.

## Next steps
1. Renew the VPS (expires 2026-11-04). 2. Router session (short; owner plans it for 2026-11-02): Wi-Fi diagnostics, then the Jellyfin rule test, grace window and router-page link checks. 3. Native-speaker review of all Burmese text (app, landing, pricing, sign-in; marked draft). 4. After the router test passes: cloud side of `product=media` vouchers and per-site Jellyfin host settings. 5. Optional: create one real trial account on production to confirm the trial end to end, then remove it.

## Working rules the owner set
- Remove wrongly built or unneeded things only with care: move to a holding folder instead of deleting on the VPS. Never enter passwords for the owner; the owner signs in and approves production changes. Keep chat replies short; the owner is often busy and the 5-hour usage limit matters.
