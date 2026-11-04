# Testing on a disposable RouterOS CHR

The tests in `internal/provisioner/routeros/chr_lab_test.go` run the app's real router code against a real RouterOS. They change the router, so they only run when `MM_CHR_SSH` names one. Never point them at a customer router.

## Setup used (UTM on the Mac, RouterOS 7.24.6 x86_64)

- Log in with an SSH key. Set `MM_CHR_SSH=admin@<ip>` and `MM_CHR_KEY=<private key path>`.
- **Time:** the scripts need `/system ntp client` to report `synchronized`. If NTP cannot leave the VM's network, run a small SNTP responder on the Mac and set the CHR's server to the bridge address.
- **PPPoE on one NIC:** two macvlan interfaces in bridge mode, `m1` and `m2`, on `ether1`. Run the PPPoE server on `m2` and the clients on `m1`, so sessions are real, with a plan named `lab-plan` and an unmanaged control user `unmanaged1` connected through client `c-un`.
- **Hotspot guest:** macvlan `m3` is the customer LAN and `m4` is the guest in its own VRF `guest`, with a default route through `10.60.0.1@guest`. `/ping 1.1.1.1 vrf=guest` crosses the wire as a real guest would. The test creates the rest.

```bash
MM_CHR_SSH=admin@192.168.64.4 MM_CHR_KEY=$HOME/.ssh/id_ed25519 go test -count=1 -v -timeout 30m -run 'TestCHRLab' ./internal/provisioner/routeros
```

## What it shows

Results below are from 2026-11-05 on RouterOS 7.24.6.

| Area | Result |
|---|---|
| PPPoE lifecycle | Install, first login, early, late and unused renewal, retry, suspend, resume, expiry. Only the affected account disconnects. An unmanaged session stays up. |
| Reboot with no time source | Managed PPPoE accounts are disabled, unmanaged ones stay up, and everything recovers by itself once NTP returns. |
| Flash writes | The 15-second guard adds none: 40 sectors a minute with it on or off. |
| Voucher deadline | Access ended 3 s after the deadline, and the guest could not return. |
| Grace of 1 minute, paid plan of 1 minute | Ended on time. |
| 1 MB data limit | Went offline at about 1 MB. |
| Speed tier | The guest's dynamic queue has `max-limit=5M/20M`, upload first. |
| Customer network with Hotspot, voucher login and guard, guest portal deployment | Pass. `api.json` is carried into the deployed folder. |
| 11 network forms | Pass. |
| Firmware upgrade | Refuses safely, because a CHR has no board firmware. |

## What it cannot show

- Measured throughput through a speed queue.
- A phone's browser, including CHAP login in a real page and captive portal detection.
- Wi-Fi: a CHR has no radios.
- Board firmware upgrades: a CHR has none.
- RouterOS 7.25 and newer, until tested on them.
- Device mode `home` or `basic`: a CHR ships as `advanced`. The check is tested with the real output format, but a blocked feature has not been seen on hardware.

## RouterOS facts found here that the manual does not state

- A comment prints as `0  ;;; text` on the record's first line, or as `;;; text` above its fields. Long comments wrap across several `;;;` lines.
- `disabled` is omitted from many `print value-list detail` outputs. A disabled record shows only `Flags: X - DISABLED`.
- `/ip hotspot user profile` has no `comment` property, and the whole `add` is refused.
- `/ppp active print stats` is refused on 7.24.6. Counters are on the session's `<pppoe-NAME>` interface.
- MAC login is tried only when a host is first seen or after a session reset, so a guest already waiting stays locked out until their host entry goes.
- `place-before=0` is refused when rule 0 is dynamic, which is the case once Hotspot runs.
- `:toarray` on a string gives text elements on 7.24.6, and `0 != "0"` is false.
- A multi-line SSH command runs on 7.24.6, but a local variable is empty on the next line.
- `:return` outside a function stops the script and prints its value.
- `/system health print terse` is refused on a CHR, so health stays empty there.
