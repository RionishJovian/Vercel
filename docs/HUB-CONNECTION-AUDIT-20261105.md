# Magic Hub connection audit: 2026-11-05

**Owner rule:** one one-paste script in a WinBox session. Once connected it keeps working and never needs connecting again, unless the user deleted the device record.

**Verdict:** not met yet. The script itself works, but several ordinary events can force a second paste or leave a router unable to connect.

**How this was checked:** code reading, read-only inspection of the production Hub and database, and the real generated script run on the CHR lab (RouterOS 7.24.6).

**Not tested:**
- A real tunnel handshake from the CHR, because that would add a peer to the production Hub.
- A paste into WinBox itself. The lab used `/import`.
- Whether RouterOS picks up a new address when the Hub's DNS name changes.

## What already works

| Check | Evidence |
|---|---|
| One script does everything | Lab: tunnel, user, SSH key, SSH access and firewall rule all installed from one run with no SSH session open. |
| Router settings survive a reboot | Lab: WireGuard peer and SSH key were still present after a reboot. Route and address output was not captured. |
| A Hub server move needs no paste | The peer endpoint is a DNS name, not an IP address. |
| Saved login survives restarts and deploys | Stored encrypted in the database. Key rotation re-seals it. |
| A lapsed pass does not disconnect | The pass check blocks app actions but does not touch the saved login. |
| Editing a record cannot change a tunnel address | Refused by the update handler, and the field is read-only. |

## Findings

### One paste

| # | Severity | Finding | Evidence |
|---|---|---|---|
| 1 | High | Pasting is not enough. The user must also press Check. If Check does not succeed within 30 minutes, the setup material expires and a second paste is needed. | `hub_automatic_management.go`: 30-minute lifetime and "pending enrollment unavailable". |
| 2 | High | A second paste of a good script always ends in `STOP: management firewall rule changed`, although the router is fine. The script compares `10.77.0.1/32`, but RouterOS reports `10.77.0.1`. The wizard says STOP means setup is incomplete. | Lab, repeat run. |
| 3 | Medium | An open SSH connection to the router makes the paste fail partway with `invalid internal item number`, leaving the tunnel installed but no management login. `/ip/service` also lists live SSH connections, so `find where name="ssh"` matches two entries. | Lab: failed with SSH open, passed with none. |
| 4 | Medium | A script not pasted within 24 hours has no way forward. The router key is gone, and the repair script stops with "run step 1 first", a step the console no longer has. | Code reading. |
| 5 | Medium | A router with a custom SSH port stops with "use advanced management setup", which the console does not offer. | Code reading. |
| 6 | Low | The form asks for a "Router HTTPS management port", although the connection uses SSH. | `Enrollment.tsx`. |

Production has 5 gateway records. 3 have a Hub tunnel allocated but no saved connection. The cause is not proven: they may be abandoned tests or these failures.

### Stays connected

| # | Severity | Finding | Evidence |
|---|---|---|---|
| 7 | High | 3 of the 13 live Hub peers are missing from the saved WireGuard file, and the Hub does not re-apply its peer list at startup. A crash or power loss would drop those routers until someone repairs the Hub. A clean reboot should save them, and a reboot is currently pending. | Production comparison. A manual, unscheduled persist tool already notes this bug "lost 3 peers before". |
| 8 | High | Off-site backups hold only the database, not the Hub private key or peer list. If the server were lost without a server-level restore, every router would need a new paste. Whether Vultr's server backups restore these was not verified. | Off-site backup script. |
| 9 | High | A connected router that breaks has no way back on its record. New scripts are refused ("already has a saved connection"), so the only route is Delete. Delete is itself refused when the record has vouchers or paid PPPoE history. A factory-reset or replaced router with sales history can never connect again. | `QuickEnrollGateway`, `DeleteRouter`. |
| 10 | Medium | "Connect Magic Hub" stays on connected routers and leads to a refusal. A failed check says "generate a new script", which the app then refuses. | `hub/index.tsx`, `hub_management.go`. |
| 11 | Low | One peer marked removed in the Hub store is still live in WireGuard, and 5 test peers are still live. | Production comparison. |

### Delete

| # | Severity | Finding | Evidence |
|---|---|---|---|
| 12 | High | Delete does not end the connection. The Hub peer stays active, because the app never calls the Hub's remove route. 6 active real-router peers have no gateway record, and one of those handshook about 58 hours ago. | Production comparison. |
| 13 | High | Connecting the same router after a delete fails with `STOP: conflicting Hub route` and leaves a second WireGuard interface behind. The old login, SSH key and firewall rule also stay on the router, and the console shows no cleanup script. | Lab, new record on the same router. |

## Proposed fix order

1. **Hub durability (7, 8, 11):** re-apply the peer list at Hub startup, save after each change, add the Hub key and peer list to encrypted off-site backups, and remove stale peers. Do this before the pending server reboot.
2. **Script (2, 3, 5, 13):** make repeat pastes succeed, ignore live SSH connection entries, use the router's real SSH port, and safely replace leftovers carrying Mega Magic's own markers.
3. **Connect by itself (1, 4, 6):** keep setup material until the router connects, let the server finish on its own once it sees the tunnel, and remove the Check step and port field.
4. **Delete and recovery (9, 10, 12, 13):** Delete removes the Hub peer and shows a cleanup script. Connected routers show status instead of Connect. A record with sales history needs an owner decision.
5. **Prove it:** repeat on the CHR, then on the owner's own router.
