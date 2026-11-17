# Real-router trial results, 2026-11-06

Partial run of `GUEST-ACTIONS-TRIAL-20261105.md` and `DEVICE-CONTROLS-TRIAL-20261105.md`, plus a first PPPoE and iKuai AP check. The Wi-Fi controls trial did not run, and `BROADBAND-PHYSICAL-TRIAL-20261104.md` remains pending.

- **Router:** owner's hAP ax² (`C52iG-5HaxD2HaxD`), RouterOS 7.25, through production and Magic Hub.
- **Clients:** one iPhone on iOS 18.7, on the router's Wi-Fi and through an iKuai IK-X9 AP on `ether2`; one Mac with a USB Ethernet adapter as PPPoE client.
- **App:** `a483570`, then `a90fa7c` after PR 150.
- **WinBox:** unavailable for most of the run. Checks that name WinBox used the app's router readings instead, unless noted.

Results: **Pass**, **Fail**, **Not run**. A pass covers this router and these clients only.

## Guest actions

| Step | Result | Evidence |
|---|---|---|
| Live users loads | Pass | Session listed; Idle and Banned MAC show numbers |
| Idle after 16 minutes | Not run | First try inconclusive: MikroTik's status page refreshes each minute and traffic kept growing. The retry was abandoned when the phone moved to the AP |
| Left against WinBox | Not run | Left fits the voucher, but was not compared with `session-time-left` |
| Kick | Pass | App reports router confirmation; session gone |
| Browse after Kick | Pass | Login page after reload, no silent login, no session a minute later. Profile is `http-chap,mac`; iPhone Safari |
| Log in again with the same code | Pass | Left and running totals carry on |
| Stale second page Kick | Pass | "guest session changed on the router"; nothing else removed |
| Ban | Pass | `type=blocked`, comment `mega-magic:ban`, session ended |
| Browse while banned | Pass | No internet, no login page |
| Remove block from the chip | Pass | Binding gone; login page back within about a minute |
| Refuse an owner's own binding | Not run | |
| Changed private Wi-Fi address | Not run | The phone used a private address throughout, one per network |
| Delete an unused code | Pass | Shows Revoked |
| Try the deleted code | Pass | Branded portal reports it invalid |
| Delete a used code | Pass | Button disabled with the reason |
| Refresh usage after delete | Pass | "Usage read from the router." |
| Print the batch after delete | Pass from data | Print filter leaves only the unused code; print dialog not opened |

## Device controls

| Step | Result | Evidence |
|---|---|---|
| Ports match the router | Pass | App readings; not compared with WinBox |
| Names have no bridge suffix | **Fail** | Topology shows `ether2;bridge / ether2` |
| Fresh backup off the router | **Fail** | Capture backup says unavailable for the owner on a connected router. On-router snapshots exist |
| Block internet rule | Pass | First static forward rule, above FastTrack, comment `mm-block-<MAC>` |
| Block effect on a browsing device | Not run | The blocked MAC was the AP, which has no browsing traffic of its own |
| Remove block | **Fail, then Pass** | Page crashed on `warnings: null`; passes after PR 150 |
| Turn port off | Pass | `ether2` off, AP link gone, countdown shown |
| Revert now | Pass | Back at 1 Gbps within seconds |
| Auto-restore, Keep, uplink refusal | Not run | |
| Port label, Reserve, Limit | Not run | |
| PoE restart, Reboot | Not run | Needs a powered device and Telegram approval |

## HotSpot, vouchers and portal

| Step | Result | Evidence |
|---|---|---|
| Wizard on a factory-style router | **Fail** | See open bugs. Needed `wifi2` removed from the bridge by hand and a client connected |
| Voucher batch stuck in `unknown` | Pass after HotSpot | Router had no HotSpot profile. Finished on first retry with no new code |
| Create vouchers | Pass | "Codes are installed and verified on the router." |
| Install branded portal | Pass | Verified; phone shows it in place of the stock page |
| Code login on the router's Wi-Fi | Pass | One code entry; internet works |
| Portal and code login through the iKuai AP | Pass | Session from the phone's AP address on `192.168.50.x`; traffic on `ether2`, none on `wifi2` |

The AP test used a bridge made by hand (`mm-guest-br`, members `wifi2` and `ether2`). The app cannot create this.

## PPPoE, not paid

`MM_BROADBAND_WRITES_ENABLED` stayed off, so paid accounts were not exercised.

| Step | Result | Evidence |
|---|---|---|
| Create subscriber service | Pass | `ether3` removed from the bridge by hand first; readback verified |
| Create subscriber | Pass, with a caveat | The form's default name `customer` went in unnoticed, and the owner did not knowingly set its password |
| Mac finds the service and logs in | Pass | Mac PPP log; `192.168.51.250`; DNS from the plan |
| Internet through PPPoE | Pass | With the VPN off: route via `ppp0`, first hop `192.168.51.1`, 4/4 pings, HTTPS 200 from the PPPoE address |
| 10M/20M limit | Not run | The download test had a script error |

## Later checks

- **Wizard fixes:** deployed (PR 152) and proven on a CHR, RouterOS 7.24.6; see `CHR-LAB.md`. The new job query also runs on this hAP's 7.25 by hand. A run on the hAP itself is still owed.
- **Bugs 1, 2 and 3 below** are fixed by PR 152; **5, 7 and 8** by PR 155, except the password autofill and the bottom bar overlap in 7.
- **Bug 6 was only partly right:** Capture backup is now enabled, but see the correction in `CHR-LAB.md`: the app doesn't keep a full copy.

## Open bugs

1. Wizard refuses every interface on a factory router: ports are bridge members and the bridge already has an address.
2. Wizard rolls back on a Wi-Fi AP with no client: RouterOS marked the DHCP server invalid. It applied once a client joined.
3. Any open terminal counts as an "active script job" (`type=login`), which blocks wizard and network applies, and the error also says rollback was not confirmed. A likely cause of the failed apply at 06:38 UTC, not proven.
4. Voucher Create locks with no discard when a saved batch can't finish in that browser.
5. Guests has no refresh timer, and a failed SWR read stops polling elsewhere until focus returns.
6. Capture backup unavailable in production.
7. PPPoE form: the `customer` default and possible password autofill, form buttons far above the form on a narrow screen, "0 records" while the router has 1, and no list of active non-paid sessions.
8. Topology shows `ether2;bridge`.
9. The wizard leaves guests on MikroTik's stock login until the branded page is installed separately.
10. Since the Voucher page preview was removed at the owner's request, the editor's Guest Portal preview differs from the router's page: "Buy WiFi" against "Wi-Fi Access". Decide whether owners should see the router page before installing it.

## Router state after this run

Not factory. Upstream Wi-Fi on `wifi1`; guest network on `mm-guest-br`; PPPoE on `ether3`; LAN `192.168.88.1` on `ether4` and `ether5` only, so WinBox and WebFig need a cable there. Test vouchers and the `customer` subscriber are still on the router.
