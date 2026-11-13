# RouterBOARD test plan (one powered session, about 30 minutes)

Purpose: prove on real RouterOS the router scripts from PR #207 (paid additional gateways, safe one-paste) that have only run in unit tests so far. Everything that can be tested without a router is already done. The session is **one automated command** plus a backup before and a comparison after, so the router is powered only while it is actually being tested.

Branch `claude/magichub-additional-gateways`, commit `3fb3c36` or later.

## What this session proves, and what it does not
| Proves (Session 1) | Does **not** prove (later sessions) |
|---|---|
| RouterOS accepts and runs the new scripts on **your** model and version, including RouterOS 7.12 differences (`/user group comment`, the SSH allow-list name, the WireGuard `rx` counter) | A real Magic Hub tunnel, handshake or management login (the lab uses throwaway keys) |
| All checks run before the first change; each STOP leaves the router unchanged | The app's Connected state, Safe Mode **commit** by a person in WinBox |
| Resume, Start over, group adoption, other `mm-…` accounts kept, cleanup script | Two physical gateways, reboot persistence, guest Hotspot (Sessions 2 and 3) |

## 1. Before you power the router on (no router time)
1. **Router.** Use a spare or test RouterBOARD with no paying customers.
   - It must **not** be enrolled in Magic Hub: the test creates and finally removes every `mm-…` user/group and `mmh-…` tunnel on it.
   - Note the model and RouterOS version.
   - **Do not upgrade it.** Your hAP ax3 on 7.12.2 is a useful case as it is.
   - Its internet connection may stay as it is (Wi-Fi or Ethernet). Nothing in this session changes WAN, LAN, Wi-Fi, HotSpot, DHCP or DNS.
2. **Computer** on the router's LAN by **Ethernet** (not the router's Wi-Fi, so an SSH change can't cut you off). It needs:
   - `ssh` and `scp`;
   - Go 1.26.11;
   - this repository with the branch checked out:
     ```bash
     git fetch origin claude/magichub-additional-gateways
     ```
     ```bash
     git checkout claude/magichub-additional-gateways
     ```
   - a compile check that it builds (no router needed):
     ```bash
     go vet ./internal/server/
     ```
3. **Test SSH key.** Create a dedicated key with no passphrase, used only for this test:
   ```bash
   ssh-keygen -t rsa -b 4096 -N "" -f ~/.ssh/mm_lab
   ```
   You'll import its public half into the router in step 2.
4. **Optional: review the exact scripts first, without a router.** This writes the four scripts the session pastes (one-paste, resume, start over, disconnect) into a folder:
   ```bash
   MM_LAB_RENDER=$HOME/mm-lab-scripts go test ./internal/server -run TestRenderLabScripts -count=1
   ```
5. **Know your recovery path.** If IP access is lost, WinBox can still connect by **MAC address** from the same LAN. The automated run restores the SSH service at the end, but if it stops halfway, this command restores it:
   ```
   /ip service set ssh port=22 address=""
   ```

## 2. Router session (powered on)
| Step | Time | What you do | Pass |
|---|---|---|---|
| 1. Backup | 3 min | In WinBox Terminal: `/export file=pre-mm-lab` and `/system backup save name=pre-mm-lab`. Download both files (Files menu) to the computer. **Do not** use `show-sensitive`. | Both files saved |
| 2. Lab access | 2 min | Upload `~/.ssh/mm_lab.pub` in WinBox Files, then run `/user ssh-keys import public-key-file=mm_lab.pub user=admin`. Check from the computer: `ssh -i ~/.ssh/mm_lab admin@192.168.88.1 "/system resource print"`. | Prints board and version |
| 3. Identity (read only) | 1 min | The read-only command after this table, which saves the router's current state. | File saved |
| 4. **The test** | ~15 min | The test command after this table. Leave the router and terminal alone until it finishes. | `PASS` at the end; every case listed below |
| 5. After | 3 min | The after-export command after this table, then download `post-mm-lab.rsc`. | No `mm-`/`mmh-` items remain; SSH back on 22 with an empty allow-list |
| 6. Power off | – | Only if every case passed and the diff in §4 is empty. Otherwise keep the router as it is and send me the files first. | – |

Step 3, from the computer (read only):
```bash
ssh -i ~/.ssh/mm_lab admin@192.168.88.1 '/system resource print; /system routerboard print; /ip service print detail where name=ssh; /user group print; /user print; /interface wireguard print; /ip firewall filter print count-only' > mm-lab-before.txt
```

Step 4, the test (from the repository folder; replace the address and LAN with yours):
```bash
MM_LAB_HARDWARE=1 MM_CHR_SSH=admin@192.168.88.1 MM_CHR_KEY=$HOME/.ssh/mm_lab MM_LAB_LAN=192.168.88.0/24 go test ./internal/server -run TestCHRLabOnePaste -v -count=1 -timeout 40m 2>&1 | tee mm-lab-$(date +%F).log
```

Step 5, from the computer:
```bash
ssh -i ~/.ssh/mm_lab admin@192.168.88.1 '/export file=post-mm-lab'
```

### Cases in step 4, in run order
About 25 pastes at roughly 31 seconds each. A failing case stops only itself; the run continues and records everything.

| # | Case | Proves |
|---|---|---|
| 0 | Preflight: router sees this computer inside `MM_LAB_LAN` | **Refuses to start** otherwise, so no lockout |
| 1–3 | First paste, same paste again, paste with an SSH session open | One-paste installs; repeat pastes change nothing; `TUNNEL ACTIVE`/`NOT CONNECTED YET` plus the **Safe Mode** line, never "close this window" |
| 4 | New key on the same gateway | STOP "holds a key from an earlier setup … choose Start over"; address unchanged |
| 5 | Restricted SSH allow-list | Existing entries kept and the Hub address added (7.12 `address` / 7.13+ `available-from`) |
| 6 | Custom SSH port | Redirect and firewall port are correct and are removed again on port 22 |
| 7 | Static drop rules | Management rule placed first |
| 8 | Another gateway's older tunnel | STOP; nothing replaced |
| 9 | Foreign route to the Hub address | STOP before any tunnel exists |
| 10 | User with our name that isn't ours | STOP **before the tunnel** is written (new order) |
| 11 | Group with our name that isn't ours | STOP before any change; its policy is not overwritten (**`/user group comment` on 7.12**) |
| 12 | Older setup's group whose only member is ours | Adopted and marked |
| 13 | Another Magic Hub account (`mm-aaaa…`) and its rule | **Left in place** (the old setup deleted them) |
| 14 | Resume | Works with the key on the router; with no key it STOPs and creates nothing |
| 15 | Start over | Foreign peer → STOP with the key unchanged; then replaces only this gateway's marked tunnel (new key, new address); the old script then refuses |
| 16 | Disconnect script, pasted twice | Everything removed; a second paste is safe |

During the run the router sends WireGuard handshakes to `megamagic.tech:51820` with throwaway keys. The Hub ignores unknown keys, so this is harmless, and it's why the run ends with `NOT CONNECTED YET`.

## 3. What to send me
- `mm-lab-YYYY-MM-DD.log`, `mm-lab-before.txt`, `pre-mm-lab.rsc` and `post-mm-lab.rsc`. Exports made without `show-sensitive` contain no passwords.
- Not needed, and please don't send: the `.backup` file and the `mm_lab` private key.

I then fix anything that failed **offline**. A follow-up session re-runs only the failed cases, for example `-run 'TestCHRLabOnePaste/start_over'`, so it is minutes, not a full run.

## 4. Pass/fail and restore
- **Pass:** the log ends in `PASS`, and a diff of `pre-mm-lab.rsc` against `post-mm-lab.rsc`, ignoring the date line, shows no changes.
- **Any failure:** don't change the router by hand. Send the files. If the router misbehaves, restore with `/system backup load name=pre-mm-lab` (it reboots), or import `pre-mm-lab.rsc` on a reset router.

## Later sessions (each needs your approval of a window; not part of this one)
- **Session 2: real one-paste through Magic Hub** (~30–45 min, after #207 is merged or on a decided build). Gateway A pasted in WinBox with Safe Mode, then Ctrl+X to commit, then Connected in the app. Then gateway B (a second router, same site, after a slot is fulfilled), with A staying Connected. Then an expired-setup resume and Start over, and a reboot of both.
- **Session 3: Hotspot acceptance** (~60–90 min with a phone): Phase 3 steps 1–15 (plan, voucher, guest page, phone login, session, single-device, expiry, reconnect).
