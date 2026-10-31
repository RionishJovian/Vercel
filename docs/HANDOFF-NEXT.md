# Handoff for the next session (2026-10-31, 19:00 Myanmar time)

Start here. Everything below is the current state; nothing else is needed from earlier chats.

## What this is
Mega Magic: multi-tenant cloud control plane for MikroTik RouterBOARD owners (Starlink -> RouterBOARD -> neighbourhood hotspot). The captive portal runs on the router; the cloud (Magic Hub, WireGuard) only manages: issues voucher codes, installs them on the router, reads usage back. Guests redeem codes on the router login page, never in the cloud. Each client: one login, one site, one router by default; extras (router/site, Jellyfin dashboard, future features) are sold through the Store catalog managed by the platform admin.
Jellyfin = media server the owner runs on their own PC. Jellyfin vouchers give media playback from that host only, no internet; separate page, stock, batches and shops from internet vouchers.

## State
- Nothing is merged or deployed except a manual test copy of the new app. Work is on branches in RionishJovian/Vercel.
- PRs: #6 webhook SSRF block, #7 close public cloud guest routes, #8 SSH host-key verification by default, #9 Store add-on catalog (needs migration `migrations/20261031100000_store_addons.sql` + rerun `deploy/runtime-db-role.sql`), #10 new app in `web/v2/` (branch `ui/jade-glass-rebuild`). Suggested merge order: #6, #8, #7, #9, #10.
- New app (`web/v2/`, plain ES modules, phone first, Burmese/English): sign-in, Home, Revenue, Internet vouchers, Router list/map/detail (read-only), Store, Platform, Jellyfin dashboard, More. Look: midnight forest green gradient, wand status light (`js/status.js`, driven by `/fleet/health`), liquid-glass tab bar, keycap buttons, iOS haptic click (`js/feedback.js`, toggle in More).
- Live test copy: https://megamagic.tech/v2/index.html, copied into the running web container with `docker cp` (lost if the container is recreated; PR #10 + web image rebuild makes it permanent). Refresh: in the VPS console clone the branch to /tmp/mm-v2-src, `docker exec mega-magic-go-restored-web-1 rm -rf /srv/v2`, `docker cp /tmp/mm-v2-src/web/v2 mega-magic-go-restored-web-1:/srv/v2`, remove /tmp/mm-v2-src.
- Still on the old app (`/app.html`): router setup/Wi-Fi/protection tools (the server holds network writes until a router test passes), voucher guest-page editor, Jellyfin guest page and vouchers.
- Mock API for local UI tests existed only in the earlier session (not committed).

## VPS (Hostinger KVM 4, srv2032937, 156.67.216.128)
- **Expires 2026-11-04. Renew it.**
- Live compose project `mega-magic-go-restored` (api, web, db, redis, nats), host Caddy, WireGuard hub on the host (wg0 10.77.0.1/24, UDP 51820/51821). Live data volumes are named `mega-magic-go-staging_*` (misleading; never clean up by that name).
- Nightly DB backup: `/usr/local/sbin/mm-db-backup.sh`, timer `mm-db-backup.timer` 20:00 UTC, output `/opt/backups/nightly` (never auto-deleted).
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
1. Renew VPS. 2. Owner reviews `/v2/` on the phone and lists fixes. 3. Router session (short): Wi-Fi diagnostics, then the rule test. 4. Merge and deploy PRs in order. 5. Build the voucher guest-page editor, Jellyfin guest page and vouchers. 6. Native-speaker review of the Burmese text.

## Working rules the owner set
- Remove wrongly built or unneeded things only with care: move to a holding folder instead of deleting on the VPS. Never enter passwords for the owner; the owner signs in and approves production changes. Keep chat replies short; the owner is often busy and the 5-hour usage limit matters.
