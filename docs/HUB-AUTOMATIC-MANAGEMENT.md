# Automatic Magic Hub management

**Owner rule:** one script pasted in a WinBox session connects a router for good. It needs connecting again only after the user deletes its record.

## How it works

1. The user authorizes the connection and gets one script. It stays valid for 30 days, encrypted in the database.
2. The user pastes it in WinBox New Terminal. It installs the tunnel, a dedicated management user with an SSH key, and one firewall rule.
3. Every 15 seconds the server checks routers with a script waiting. Once the Hub reports a recent handshake, the server logs in over the tunnel, checks the router's proof and key, pins its SSH host key and saves the login, encrypted. The page can be closed; nothing more is needed.

The page only watches for that result. Only a saved, verified login counts as connected.

## The script can be pasted again at any time

It brings the router to the wanted state from wherever it is:

- Anything carrying this enrollment's marker is corrected.
- Anything an older Mega Magic setup left, for example after a record was deleted, is replaced. Ownership is judged only by Mega Magic's own `mega-magic:` markers.
- Anything belonging to someone else stops it with `STOP:` before the first change. The management user check runs just after the tunnel is installed.
- WAN, LAN, the owner's users and other firewall rules are not touched.

Details that came from testing on a real RouterOS (`TestCHRLabOnePaste`):

- `/ip/service` also lists live SSH connections, so the script uses the first match, which is the service itself.
- Current RouterOS calls the SSH allow-list `available-from`, and `address` is deprecated. The script uses `available-from` and falls back to `address` only on older versions, keeps the existing entries and adds the app's address.
- Some connections have no working DNS while WireGuard works fine, so the Hub's `MM_HUB_ENDPOINT` is its IP address, not a name. Before the Hub moves to a new IP, every connected router must be updated over its tunnel first.
- The script waits up to 30 seconds and says `NOT CONNECTED YET` if the router has not reached the Hub. The setup stays in place and connects by itself once it can.
- A custom SSH port gets a redirect on the tunnel interface, so the app always connects to port 22.

## Deleting a router with sales history

A router with voucher or PPPoE history can't lose that history, so Delete keeps its record and history, clears its saved login and ends its Hub connection. If the Hub can't end the connection, nothing changes. The same router or a replacement can then connect again on that record with a new script. A router with no history is deleted fully.

## Setup that ran out

If a script's key has ended and the router never connected, a new script gets a fresh Hub allocation and key. The new script corrects whatever an earlier paste left. This never happens to a router with a saved login: new scripts are refused for those.

## Router status

`/fleet/health` is the one source of a router's connection state, and every console page reads it. For a gateway it uses the Hub's live handshake, where contact in the last 5 minutes is Online. If the Hub has no peer or cannot answer, it uses the router check, then a successful router read in the last 5 minutes. Each row names its source and, where known, its last contact and check times. A successful read is a separate fact from the tunnel: when the Hub answers, its state wins.

## Connection history

Each monitoring sweep asks the Hub about every gateway and adds a `router.connection.up` or `router.connection.offline` row to `device_change_events` when the state differs from the last row. A row is written only when the Hub answers, so a failed lookup is not a router drop. `/fleet/health` returns `state_since` from these rows. The offline row's reason holds the last contact time, because the row itself is written up to 5 minutes after the drop.

These rows raise no incidents or alerts. Writing `peer_heartbeats` instead would feed `ClassifyPeerState` and start critical incidents for every offline router, so that stays a separate decision.

## Delete

Delete removes the record and ends the Hub connection in one step. If the Hub cannot end it, the record stays. The user then gets a script that removes the setup from the router, and can skip it if the same router will connect again.

A record with voucher or PPPoE history cannot be deleted yet, and its Hub connection stays. What should happen then is an open owner decision.

## The Hub

The Hub's peer list is the source of truth. At startup and every 30 seconds it restores WireGuard to match and saves the WireGuard file, so a crash cannot cut routers off. It never removes a live peer it does not know. The Hub's key and peer list are in the encrypted off-server backup (`docs/BACKUPS.md`). Install with `scripts/deploy-hub-reconcile.sh`.

## Not proven yet

Tested on a RouterOS 7.24.6 CHR without a real tunnel, because that would add a peer to the production Hub. A real router must still prove the handshake, the login over the tunnel, older RouterOS versions, and a paste through WinBox itself.

### When the Hub step succeeds and the database commit fails

The connection is gone and the record is unchanged, so the API returns 503 with `hub_ended_record_unchanged` and the cleanup script, and logs the failure. The console keeps showing the script. Pressing Delete again finishes the job, because the Hub then has nothing left to end. Nothing retries by itself, so until then the card can still say it is connected while Fleet health shows the true state.
