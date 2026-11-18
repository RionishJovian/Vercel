# Router test kit: Wi-Fi WAN resume and voucher load (hAP ax²)

Written 2026-11-18. Covers what PR 250 (Wi-Fi WAN resume) and the voucher scheduler question
(`guest_voucher.go`, one 10 s scheduler per voucher) still need from a real router. Everything
else in the one-paste was proven earlier (`docs/ROUTER-TEST-BUCKETLIST-20261113.md`,
`docs/HARDWARE-TRIAL-RESULTS-20261106.md`). Nothing below is verified yet.

## Before the router is powered (no router time)
- Router: the hAP ax², no paying customer on it, powered from its own adapter or a PoE injector
  that meets its input (a weak USB or laptop port cannot power it).
- Mac: Ethernet adapter in a router LAN port (ether2 to ether5), set to DHCP. Wi-Fi may stay on
  the home network. Winbox by MAC address if the IP is unknown.
- Production sign-in on https://megamagic.tech/app/ with the dev test account; the router must be
  a gateway in a site you can delete afterwards.
- Backup first: `/export file=pre-wifi-wan` and `/system backup save name=pre-wifi-wan`, then
  download both from Files. Never `show-sensitive`.
- Wi-Fi uplink details ready: the SSID and passphrase of the Wi-Fi the router will use as WAN.

## Test A: board check (about 5 min)
1. In the app, Magic Hub, add a gateway, pick **hAP ax²** in the model list.
2. Choose Ethernet WAN, copy the script, paste in Winbox New Terminal.
   Pass: no `STOP`, ends with the Magic Hub tunnel steps.
3. Repeat on a **different chosen model** (for example hAP ac²) without changing the router.
   Pass: stops at the first check with the board-string message, router unchanged
   (`/export` diff is empty).

## Test B: Wi-Fi WAN and resume (about 20 min)
1. Pick hAP ax², Wi-Fi WAN, the 5 GHz or 2.4 GHz radio, enter the SSID and passphrase,
   copy the script.
2. **Cut the paste:** paste only the first half (up to the line `# APPLY (Wi-Fi WAN)`), then stop.
   Pass: the router shows the three marked objects (`/interface bridge port`, `/interface list member`,
   `/ip dhcp-client`, comment `Magic Hub Wi-Fi WAN`) or nothing yet; nothing unmarked changed.
3. Paste the **whole** script again.
   Pass: it continues, does not duplicate anything (`print` each list once), prints
   `WAN DHCP and default route are ready. Continuing with Magic Hub.`, then the tunnel steps.
4. Collision check: add an unrelated entry by hand (for example a DHCP client on ether1 with no
   comment), paste again. Pass: it stops with "was changed outside this setup", nothing changed.
5. Record: RouterOS version, the exact STOP text if any, and `/ip dhcp-client print`,
   `/interface list member print`, `/interface wifi print` (or `wireless`) before and after.

## Test C: voucher scheduler load (about 20 min)
Only needed to decide whether to replace the per-voucher schedulers with one shared guard.
1. Issue 10, then 50 test guest vouchers on the gateway and redeem them (or run the router-side
   add commands for the same count).
2. Read `/system resource print` (cpu-load) and `/system scheduler print count-only` at 0, 10 and 50.
3. Decision rule: if cpu-load stays under about 20 percent at 50, leave it; if it climbs, open the
   shared-guard work (expiry must move onto the voucher user; `guest_resume.go` and two test files
   depend on the current event).

## After
- Delete the test site and gateway in the app (hard delete), then `/system reset-configuration`
  only if the router goes back to spare use. Keep the export files with the results.
- Add the results to `docs/` with the date; update `AGENTS.md` (Wi-Fi WAN is "verified on hardware"
  only for the model and RouterOS version actually run).
