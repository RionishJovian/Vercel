# Real-router trial results, 2026-11-07

Continues `HARDWARE-TRIAL-RESULTS-20261106.md`. It covers guest Wi-Fi through a Ruijie access point, repeat guest page installs, and step 5 of Quick setup.

- **Router:** owner's hAP ax² (`C52iG-5HaxD2HaxD`), RouterOS 7.25, through production and Magic Hub. Upstream is still Wi-Fi on `wifi1`.
- **Access point:** Ruijie Reyee EW1300G, its WAN port cabled to `ether2`, which is a member of `mm-guest-br`.
- **Lab router:** CHR, RouterOS 7.24.6, in UTM, in the `dev@magic.com` account.
- **Client:** one iPhone, driven through iPhone Mirroring. The owner entered every password and voucher code.
- **App:** `328308f`, then `dc59a0a` (PR 157) for the hAP installs.

Results: **Pass**, **Fail**, **Not run**. A pass covers this router and these clients only.

## Guest Wi-Fi through the Ruijie

In router mode the phone got `192.168.110.206` from the Ruijie and did not appear on the hAP, which saw only the Ruijie's own `192.168.50.250`. The steps below ran after the Ruijie was switched to wired repeater mode. RG-EW routers show that mode under 更多 > 工作模式 > 有线中继. Its admin address became `10.44.77.254`, which has not been opened yet.

| Step | Result | Evidence |
|---|---|---|
| Phone gets its own guest address | Pass | `192.168.50.249` on `mm-guest-lan`, under the phone's own Wi-Fi address |
| Branded portal appears | Pass | iOS login sheet, served from `192.168.50.1` |
| Code login | Pass | Guests shows the session with the code, IP and MAC |
| Internet | Pass | `example.com` over HTTPS |
| Traffic on the right port | Pass | `ether2` counters rise; `wifi2` stays at 0 |
| Ban | Pass | Binding `type=blocked`, comment `mega-magic:ban`; 0 sessions; a new page stalled, then "the server stopped responding" |
| Remove block | Pass from the app | App reports the block gone and offers Block again. The router's binding list was not re-read |
| Login page after the block is removed | Pass, with a caveat | `http://192.168.50.1/login` loads. iOS said it was not connected for normal sites and showed no login sheet this time |
| Same code again after the block is removed | Not run | |

## Device controls

| Step | Result | Evidence |
|---|---|---|
| Turn port off, press nothing | Pass | The countdown ran out on `ether2`. The app reported that the router put the settings back, and the next read shows the port running |
| Review screen | Pass | Names the router and port and states the 5-minute restore |
| Block effect on a browsing device with no HotSpot session | Not run | On a HotSpot guest, "Block internet" is the guest Ban. This needs a device on a network with no HotSpot |
| Keep, labels, reserve, limit, PoE, reboot | Not run | |

## Repeat guest page installs

| Step | Result | Evidence |
|---|---|---|
| CHR, install from a folder with 31 entries, before the fix | **Fail** | "HotSpot directory copy did not verify", twice. The profile did not move |
| CHR, reinstall test after the fix | Pass | `TestCHRLabPortalReinstallOnAGrownFolder`: two installs with a 10-second limit per command; new folders hold 26 files |
| hAP, two installs on `dc59a0a` | Pass | Verified in about 3.4 seconds each; profile moved each time; phone still gets the page |

Cause: each install cloned the active folder with every older `login-backup-*` file. At 31 entries RouterOS 7.24.6 finished the copy in under 2 seconds, then held `/file copy` for 60 seconds and returned `action timed out`. The app's SSH limit fired first, at about 12 seconds in all, and it discarded a good copy. At 30 entries or fewer the copy returned in about 0.4 seconds.

The hAP run does not prove the fix on RouterOS 7.25: its folder was not large, so the stall was not exercised, and the app cannot read the router's file count.

## Quick setup, step 5 only

Run on the PR 140 preview with the CHR. The account already had a site, a router, a HotSpot and a saved page, so Quick setup opened at step 5. Steps 1 to 4 and a new account were not walked.

| Step | Result | Evidence |
|---|---|---|
| Resume from existing data | Pass | Steps 1 to 4 ticked, step 5 current |
| Review changes nothing | Pass | Page says nothing changes; the profile stayed on its folder |
| Install | **Fail, then Pass** | Failed on the bug above. Passed once the folder was small again; finish screen shown |
| Finish screen wording | Pass | Says the page has not been tested with a guest |

## Open bugs

1. iOS may not show the login sheet for a guest whose block was just removed, and normal sites then report no connection. Seen once. Real guests could get stuck here.
2. Old `mm-portal-*` folders are never removed from the router. This could matter on routers with little storage.
3. Topology kept the Ruijie's old LAN address `192.168.88.248` after it moved to the guest network.
4. `ether2` was missing from the network inventory read about 20 seconds after being turned off. Cause not looked into.
5. "Block internet" on a HotSpot guest performs a guest Ban. The confirmation says so, but the label may mislead. `DEVICE-CONTROLS-TRIAL-20261105.md` says a HotSpot guest gets the guest action instead.
6. Quick setup does not name the router when there is only one, against AGENTS section 4 (keep the target visible).
7. On the Burmese page, the install steps and errors are in English.
8. The Magic Dude button and bottom navigation cover text and buttons on a phone-sized screen, including the install error and the voucher button.
9. The Hotspot step mounts briefly in Quick setup while the router's profiles are still loading.
10. The CHR logs `service malfunction` about every 10 minutes, with `autosupout.rif` files. It is not tied to installs. Cause not looked into.

## Router state after this run

- **hAP:** not factory. As recorded on 2026-11-06, plus the branded portal in `mm-portal-f23157c3fe210dbb` and the Ruijie on `ether2`. Test codes remain on the router.
- **Ruijie:** wired repeater, one owner-set Wi-Fi name and password.
- **CHR:** profile `mm-labnet` on a new portal folder, with older folders kept, including two left by the failed installs. It is off the Hub until its temporary relay is restarted.
