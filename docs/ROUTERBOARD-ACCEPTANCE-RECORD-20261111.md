# RouterBOARD acceptance run sheet — prepared 2026-11-11

**Current verdict: NOT RUN.** This record prepares a future approved physical run. No RouterBOARD was contacted or modified during this stabilization task. Go tests, mocked RouterOS, the local browser walkthrough and CHR results do not satisfy these physical checks.

## Start record — fill before any contact or write

| Required fact | Current value |
|---|---|
| Operator, owner approval and maintenance window | Owner confirmed hAP ax² available now, no paying customers; exact write/rollback scope still requires preflight |
| Owned test gateway, model/board identity and RouterOS version | Owner selected hAP ax²; board identity and RouterOS version not yet read |
| Software full SHA; API image digest; served assets; migration revision | Select reviewed release after separate approval |
| Business/site/router binding and pinned management identity | NOT VERIFIED |
| Cabling, management uplink, AP model/firmware and guest SSID | NOT VERIFIED |
| Fresh accessible encrypted backup and independent recovery path | NOT VERIFIED; failure blocks risky changes |
| Existing configuration/voucher inventory and expected rollback | NOT CAPTURED |
| Non-revenue fixture batch and phone/second test client | NOT SELECTED |
| Live write gates (server-wide) and scoped intended operation | NOT READ for a hardware window |

The owner's hAP ax² is not a factory-reset device. The dated 2026-11-06 record describes upstream `wifi1`, guest `mm-guest-br`, an AP on `ether2`, PPPoE on `ether3`, and management on `ether4/5`; **re-read before relying on any of it**. Do not reset, remove retained vouchers/subscribers, or recreate this topology by assumption. The 2026-11-07 follow-up in draft PR #158 is additional historical evidence, not current state.

## Ordered physical checks

Every row starts **NOT RUN**. Record PASS only with observed evidence for the named hardware/software, including the negative case. On failure stop dependent writes and record the exact reason; no blanket overall PASS.

| ID / current status | Action and required evidence |
|---|---|
| RB01 — NOT RUN | Read exact identity, supported version/model and actual topology using pinned tenant-scoped access. Fresh Hub handshake plus authenticated router read; reject stale/foreign context. A reachable port alone is insufficient. |
| RB02 — NOT RUN | Verify backup creation, off-router encrypted capture, readable archive and a safe recovery path. Recheck the earlier production capture failure. Never restore a whole customer configuration merely to prove a test. |
| RB03 — NOT RUN | On a separately approved test network, review HotSpot setup diff, existing bridge membership/address/DHCP ownership, firewall/DNS order and management preservation. Apply only after scope approval; independent readback. An existing owner's network is not a factory test fixture. |
| RB04 — NOT RUN | Publish/install the selected branded router portal through the approved path. Read back actual router files, selected HotSpot profile/html directory and local asset references. No overwrite of unowned files; failure and rollback must be explicit. Hosted preview is a separate artifact. |
| RB05 — NOT RUN | Issue one small non-revenue Internet Access Voucher batch. Read back each intended RouterOS username/password/profile/limits before ready/print. Retry the same batch ID after an uncertain result; preserve codes and counts. Generic `/api/v1/vouchers` rejects `guest-internet`; test that refusal separately from real router provisioning. |
| RB06 — NOT RUN | With phone cellular data/VPN disabled, open a captive portal and redeem a fixture code using the actual supported CHAP/PAP path. Record success/error/consent/retry and HTTPS Internet evidence. Check through native Wi-Fi and a bridged third-party AP separately. |
| RB07 — NOT RUN | Capture router-origin first-use time and session/accounting evidence; disconnect/reconnect with the same code. Original expiry and accumulated usage continue; no second activation, refreshed allowance or new code. |
| RB08 — NOT RUN | Independently test upload, download and total quota; direction of speed limits; N+1 device denial; hard time/data expiry and no-cookie/MAC/private-address behavior under existing policy. Compare actual client traffic, router counters and app refresh, not only configured values. |
| RB09 — NOT RUN | In an approved maintenance window isolate only the cloud/Hub test path. Prove already-installed local voucher expiry and continued permitted service where supported. Restore connectivity and reconcile once; no duplicate batches/activation/revenue. Keep local recovery available. |
| RB10 — NOT RUN | Kick, Ban and Unban a real browsing fixture client; verify session removal, no silent login where forbidden, blocked browsing and later recovery. Stale/foreign requests must refuse without affecting another guest. Test actual client MAC, not an AP's management MAC. |
| RB11 — NOT RUN | Delete an unused fixture code and verify router absence/revocation plus invalid login. Used-code deletion and unverified printing remain blocked. Preserve history and reconcile accounting separately. |
| RB12 — NOT RUN | Test a supported non-uplink port control only after explicit impact review. Independent link/client evidence, immediate Revert and timed auto-restore; management/uplink refusal. Recheck bridge-name presentation. PoE/reboot/radio writes need their own supported hardware and approval; none is implied here. |
| RB13 — NOT RUN | Disposable production-shaped host Caddy -> web Caddy -> API chain with stable exact peers: two independent source clients and one owned HotSpot client, 60 browsers behind one NAT, abusive browser vs another, missing/forged XFF/cookies, restart and measured Retry-After. Do not load-test a paying hotspot. Cookie-less clients retain aggregate limits. Capture only redacted source distinctions, never guest identifiers. |
| RB14 — NOT RUN | N19 SSID preview on supported MikroTik and third-party AP models: exact capabilities/current readings, monitoring-only/unsupported/offline refusal, before/after preview and stale-context rejection. Current preview-only implementation does not authorize or prove a universal SSID rollout. |
| RB15 — NOT RUN | Dummy payment approvals and receipts: explicit owner action, tenant separation, idempotency and recorded-vs-bank-settled distinction. No real transfer. Jellyfin access needs separate owner-PC playback acceptance and must not be inferred from Internet voucher success. |
| RB16 — NOT RUN | Restore only test-created scoped changes, independently verify baseline and management/guest continuity, identify retained fixtures, and record final RouterOS/app state. If restore is uncertain, mark Recovery Required; do not label the run complete. |

## Per-row evidence form

Copy only for a row actually executed:

- ID and verdict: PASS / FAIL / NOT RUN / BLOCKED.
- Operator; timestamp with timezone; approved operation/window reference.
- Exact software SHA/image, RouterBOARD/AP/phone model and firmware.
- Expected behavior; actual observation; redacted independent readback/client evidence.
- Negative case; rollback action/result; preserved unrelated state.
- Failure/blocker reason and next safe action.

Keep secrets, voucher codes, MACs, full guest IPs, receipts and private keys out of committed evidence. Store sensitive raw diagnostics only in the approved private location; link a redacted summary. [Existing trial](HARDWARE-TRIAL-RESULTS-20261106.md), [backup/controls procedure](DEVICE-CONTROLS-TRIAL-20261105.md), [guest actions](GUEST-ACTIONS-TRIAL-20261105.md), [#192 evidence](https://github.com/RionishJovian/Vercel/pull/197).

## Follow-up access preflight — 2026-11-11

Owner confirmed the hAP ax² is available now and is not serving paying customers. Physical acceptance has not started: this Mac routes the saved `192.168.88.1` management address through `192.168.100.1` on en0; SSH port 22 timed out, ARP had no direct entry, and WinBox's saved connection did not establish. The pending WinBox connection was cancelled. No password was exposed or changed, and no Mac VPN/Wi-Fi/network or RouterOS setting changed. RB01 is **BLOCKED on a verified management path**, not a hardware FAIL or PASS. Owner was asked to connect the Mac to the current management LAN or identify a changed management IP. Earlier ether4/ether5 management notes remain unverified today.

The separate real-Caddy shared-NAT fixture passed on PR #197 commit `986adf4`; it covers two controlled loopback proxy hops and web/API restarts, not the real HotSpot/client or physical-network portion of RB13.
