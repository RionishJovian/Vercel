# Board firmware upgrade

The router page shows a router's board firmware (RouterBOARD bootloader) next to the installed RouterOS, for example
`7.6 · Upgrade available: 7.24.5`. The **Firmware upgrade** card upgrades it from the app. It is the one change in the app
that restarts the router and cannot be undone from the app, so it is previewed, confirmed and verified.

## Switch: `MM_FIRMWARE_UPGRADE_ENABLED`
Set in `.env.production`, then recreate the API. Off by default.

| Value | Preview (read the router, show the plan) | Apply | What the card shows |
|---|---|---|---|
| unset / anything else | refused (503) | refused (503) | "switched off for now" |
| `preview` | allowed | **always refused (503)** | the review, no apply button |
| `true` | allowed | allowed | the review with consent checkbox and **Upgrade and restart** |

Only the exact values `preview` and `true` have any effect. Roles: ADMIN and above (the server answers 403 to others, and the card is not shown).

## What happens
1. **Preview** reads the firmware facts fresh from the router (`/system routerboard`, `/system package update`) and refuses anything it cannot prove: unreadable versions, firmware already current, an offered firmware that is not newer, an offered firmware newer than the installed RouterOS, or a network change waiting to be kept or reverted. It shows the versions, the steps and the warnings, and gives a single-use five-minute confirmation.
2. **Apply** re-reads the router and refuses if anything differs from the preview, if the confirmation was used, expired, or belongs to another router or business, or if a script job is running. Then: export and backup on the router, `/system routerboard upgrade`, and a scheduled restart (`:execute { :delay 3s; /system reboot }`). Each step stops the next on failure, and the error says what was and was not done.
3. **Verification** is not claimed by the server: the router is gone while it restarts. The card polls the router and shows success only when the router itself reports the new firmware again; after ten minutes without that it says so and points at power and cable. The retained firmware reading is cleared when the firmware is written so the page cannot keep showing the old version.
4. The audit log records `router.firmware.upgrade_started` or `router.firmware.failed`, with no router output.

## Not proven on real hardware yet
Verified only against a simulated router (order of commands, refusals, single-use confirmation, state changes between preview and apply, failure at each step) and a stand-in UI test. Not yet confirmed on a real RouterBOARD:
- that `/system routerboard upgrade` runs without an interactive prompt over the app's SSH transport;
- that the scheduled restart fires and the router returns on its own;
- what `current-firmware` reports before and after the restart on RouterOS 7.12 and later.

Run the preview on a router first (`MM_FIRMWARE_UPGRADE_ENABLED=preview`), then apply to a router whose owner agreed to the restart, at a quiet time, with someone able to reach its power.
