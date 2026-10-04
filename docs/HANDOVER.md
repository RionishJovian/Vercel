# HANDOVER - MikroMagic / Mega Magic

**Read this before changing anything on production.**
Updated 2026-10-04 after the migration to Hostinger. The live system beats this
file; if they disagree, verify against production and fix this file at the same time.

---

## 0. THE ONE RULE

**No production change without a verified rollback.**

---

## 1. The server - ONE box, app + hub

| | |
|---|---|
| Provider | Hostinger KVM |
| IP | **156.67.216.128** |
| SSH alias | `mikromagic-hostinger` (key `~/.ssh/mikromagic-hostinger`) |
| Specs | 4 core, **15 GB RAM**, 193 GB disk |
| OS | Ubuntu 24.04.5 LTS, clean (no control panel, no Traefik) |

App and hub share one instance. Ports never conflicted (hub 51820/51821 UDP;
app 5433, 6379, 4222, 8080, 80, 443). The trade-off is deliberate: one failure
takes down site and router reachability together. They were independent before.

**The previous VPS pair was at Z.com** (app `118.27.148.195`, hub
`118.27.148.183`). Both became unreachable on 2026-10-04 and are treated as
lost. The owner has **no intention of returning to that provider**.

**Why it is lost - stated as owner report, not verified fact.** The owner says
billing had expired. No suspension notice was ever produced, and the evidence
does not cleanly match a billing suspension: SSH attempts **timed out** rather
than being refused. A suspended account usually still answers the port and
rejects the login ("permission denied"); a timeout means packets were not
arriving, which is more like a network-level block. So the true cause is
unconfirmed - non-payment is plausible, but so is a provider-side block.

**Do not build on "it was billing".** Whether the disks were deleted, retained
for reactivation, or destroyed is unknown. The hub WireGuard private key is
therefore treated as unrecoverable, which is why the key below was regenerated.
If Z.com ever reappears in the owner's plans, first ask whether the old instances
can be reactivated - that would restore the previous key and all nine router
connections without re-enrolling anything.

---

## 2. Services

| Service | Port | Notes |
|---|---|---|
| `mega-magic-api` | 8080 | Go binary as `mmapp`, `/home/mmapp/app/server` |
| `mm_db` | 5433 | postgres:16-alpine, volume `mm_pgdata`, trust auth, **bound to 127.0.0.1 only** |
| `mm_cache` | 6379 | redis:7-alpine |
| `mm_nats` | 4222 | nats:2-alpine |
| `nginx` | 80/443 | serves `/opt/mega-magic-web`, proxies `/api` to 8080 |
| `wg-quick@wg0` | 51820/udp | WireGuard hub |
| `mm-backup.timer` | - | daily 03:30 UTC |
| `mm-edge-check.timer` | - | every 30 min |
| `certbot.timer` | - | TLS renewal |

Config: `/home/mmapp/app/config.yaml` (640) and `/etc/mega-magic/*.env` (600).

**Hub public key:** `dJEFC7BD/RRWBRZ9W1l7l+f5dnZtj8OSKssP8YLwcXQ=`
Private key is in `/etc/wireguard/wg0.key` mode 600 - never print it.

---

## 3. CRITICAL: hub.mikromagic.com must be DNS-ONLY

It is an **A record, `proxied=false`**, pointing straight at 156.67.216.128.

**Never put it behind Cloudflare's orange cloud.** Cloudflare does not proxy
arbitrary UDP, so a WireGuard endpoint behind it cannot complete a handshake. It
was proxied on the previous host, which is the likely reason router enrolment was
unreliable.

`mikromagic.com` and `www.mikromagic.com` stay proxied - correct for the website.

---

## 4. CRITICAL: WireGuard peers must be persisted

On the previous hub, 3 of 9 peers existed **only in kernel state** and vanished
on reboot. Always compare:

```bash
grep -c '^\[Peer\]' /etc/wireguard/wg0.conf   # on disk
wg show wg0 peers | wc -l                      # in memory
```

They must match. Persist any peer the app adds at runtime.

---

## 5. Deploying

Source was shipped from the Mac as a **git bundle** - this VPS has no GitHub
deploy key yet (see open work).

```bash
# on the Mac
cd "/Users/aces/Magic Development/mega-magic-project"
git bundle create /tmp/repo.bundle --all
scp /tmp/repo.bundle mikromagic-hostinger:/tmp/

# on the VPS
sudo -u mmapp git -C /home/mmapp/app/src fetch /tmp/repo.bundle main
sudo -u mmapp git -C /home/mmapp/app/src checkout FETCH_HEAD
sudo -u mmapp -H bash -c 'cd /home/mmapp/app/src && \
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /home/mmapp/app/server ./cmd/server'
sudo -n systemctl restart mega-magic-api
```

**Apply any schema change before restarting**, or the API fails on a missing table:

```bash
ENVIRONMENT=production /usr/bin/go run ./cmd/<name>-schema --action apply --config config.yaml
```

`deploy.sh` targets the old `/home/ubuntu/mega-magic-project` layout and **does
not work here**. Use the sequence above.

---

## 6. Backups - the gap that was fixed

`/usr/local/bin/mm-backup.sh` via `mm-backup.timer` (03:30 UTC) writes to
`/var/backups/megamagic/`, keeps 7 dumps plus globals.

**The old backup verified TABLE COUNT ONLY.** That gap let a restore report
"verification passed" on a database missing 42 of its 48 RLS policies - a
cross-tenant exposure, found by accident during migration.

The current script reads live table **and policy** counts, dumps, **restores into
a throwaway container** (`mm_verify`), verifies both counts, and rotates old dumps
**only after** a verified restore. Failure leaves existing backups untouched.

```bash
sudo -n journalctl -u mm-backup.service -o cat -n 20
```

---

## 7. Edge guard

`/usr/local/bin/mm-edge-check`, every 30 min. `sudo -n mm-edge-check; echo $?`

Checks `immutable` absence, nginx validity, real pages 200, unknown paths 404,
unauthenticated API 401, four security headers, nginx/api/wg active, containers
running, cert >14 days, and **RLS policies == 48**.

**The `immutable` check matters most.** `add_header Cache-Control "... immutable"`
makes Cloudflare pin an asset URL and never revalidate, so a correct deploy stays
invisible for up to 7 days. The guard has been **negative-tested**: `immutable`
was injected and the guard correctly failed.

nginx `add_header` does **not** inherit into a location that declares its own
`add_header`. The security headers are repeated inside every such location on
purpose - do not "clean that up".

---

## 8. TLS

certbot runs in a **venv** at `/opt/certbot-venv` (Ubuntu 24.04 has no system
pip), using **DNS-01** via Cloudflare so renewal works without the domain
pointing here. Credentials in `/root/.cloudflared/dns.ini`.

Use the venv binary when running certbot manually.

---

## 9. OPEN WORK

1. **Router re-enrolment not done.** Routers must be re-enrolled against the new
   hub key. Until then none can connect. The server side is now ready - see
   section 11 for what was blocking it.
2. **Router must have internet AND a correct clock before enrolling.** The
   enrolment request is signed with a timestamp; a router whose clock is stale
   (RouterOS falls back to its last-known time when it has no route out) gets
   rejected. Enrollment is not the first thing to fix on a fresh router - get
   the WAN working and NTP syncing first.
3. **Cloudflare token is expired.** Both tokens previously supplied now return
   "Invalid API Token", so cache purges and DNS edits are impossible via API.
   Until regenerated, **a Cloudflare fix means renaming the asset** (see
   section 11) rather than purging. Token: DNS edit only, no tunnel permission.
4. **The GitHub deploy key IS now authorised** (`mm-vps-deploy`, pull-only).
   The repo remote was repointed from the one-shot `/tmp/repo.bundle` back to
   GitHub, and `core.sshCommand` points at the right key, so `git pull` works
   unaided as the `mmapp` user. Run git as `mmapp` - the repo is owned by it and
   git refuses to run as root.
5. **The video media relay (`4fc5d7f`) is in source but NOT enabled.** It streams
   video bytes through the app server; cheap/free plans must not carry that.
   Leave it off until there is dedicated bandwidth.
6. **Two stashes of unpublished work** on the old app VPS were not carried over.
7. **`mega.mikromagic.com`, `ns1/ns2` still point at the dead 118.27.148.x IPs**
   and serve nothing. Remove or repoint.
8. **Root password was weak and was exposed in a screenshot.** Still set; root
   password login still possible. An `mmapp` user exists - disable root login.

---

## 10. Two bugs that cost real time - read before deploying

**A. `sites-enabled/mega-magic` was a regular FILE, not a symlink.**
It silently shadowed `sites-available/mega-magic`, so `ln -sf` "succeeded" while
nginx kept loading a stale copy. Result: `www.mikromagic.com` returned 404
because the loaded config had no `www` block. Worse, nginx picks the *first*
`listen 443` block as the catch-all, and `magic-hub` sorted before `mega-magic` -
so the Hub's `location / { return 404; }` swallowed every unmatched Host header.
Fix: real symlink, apex marked `default_server`, hub renamed `zz-magic-hub` so
the site sorts first. **Always `ls -la sites-enabled` after any vhost change,
and never leave backups inside `sites-enabled` - `nginx -t` will fail on the
duplicate and catch it.**

**B. Enrollment returned 503 "Magic Hub enrollment is not enabled".**
Not a flag problem - `VPS_HUB_ENROLLMENT_ENABLED=true` was set and correct.
`config.yaml` still carried the OLD split topology: `interface: "wg-magichub"`
and `ping_target: "10.99.0.1"`. On this single instance the hub interface is
`wg0` at `10.77.0.1`, and the dead `10.99.0.0/24` entries were still in
`allowed_target_cidrs` / `management_cidrs`. The app could not resolve a
reachable target for the router, so it reported enrollment disabled.
Fixed in `/home/mmapp/app/config.yaml` (backups: `*.bak-topology-*`,
`*.bak-cidr-*`) and in the tracked `config.yaml.example`. **After any topology
change, verify the interface named in config actually exists on the box
(`ip link show <iface>`).**

**C. The stale-cache trap, twice.** Cloudflare serves assets with
`cache-control: immutable, max-age=604800`, so a browser or the edge can keep
stale JS for 7 days and a server-side fix changes nothing. It caused the
2.8MB-image problem and then a **total sign-in failure** - the edge held a
16-hour-old `api.js` that lacked the `X-MM-CSRF: 1` header, so every login got
403 `same-origin request required` while the correct file sat on the origin
untouched. With no valid API token to purge, the fix is to **rename the asset
to a name the edge has never seen** and repoint the HTML at it. Versions used:
images `-20261004b`, `api.js` `-20261004c`.

---

## 11. House rules

- Never `kill`/`nohup` the app - systemd owns it.
- Long work in `systemd-run` units, not foreground SSH.
- Push multi-line scripts with `scp`, never nested `ssh "..."` quoting.
- **Never** restore without checking **both** table and policy counts.
- Verify with real commands and paste output. Do not report work as done because
  a document says it was.
