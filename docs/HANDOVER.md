# HANDOVER - MikroMagic / Mega Magic

**Read this before changing anything on production.**
Updated 2026-10-04 after the migration to Hostinger. The live system beats this
file; if they disagree, verify against production and fix this file at the same time.

---

## 0. THE ONE RULE

**No production change without a verified rollback.**

### Owner-confirmed media boundary, 2026-10-04

Magic Media playback is exclusively for guests on the business's RouterBOARD
network. The owner's PC hosts Jellyfin and delivers video locally through that
network. Magic's app/hub must not carry video bytes. Cloud bank/MMQR receipt
review and timed-pass authorization remain control-plane functions. The old
outbound video relay was an incorrect architecture assumption, not approved by
the owner. Source corrections are not evidence of a live deployment; verify
the running binary and local delivery acceptance before enabling paid sales.

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

**Run deploys as the `mmapp` user:** `./deploy.sh web`. It has a scoped
sudoers drop-in at `/etc/sudoers.d/90-mmapp-deploy` granting only the exact
commands deploy.sh performs (rsync/install pinned to the web root, systemctl
reload nginx / daemon-reload / restart mega-magic-api). Verified that
`sudo -n /bin/bash`, `systemctl restart ssh`, `tee /etc/passwd` and `su - root`
are all still denied - it is not a path to root.

Do NOT run deploy.sh as root out of convenience: git will refuse the repo
(owned by `mmapp`) and you lose the "am I deploying as the right user?" check.
If root must pull first, pass the deploy key explicitly:
`GIT_SSH_COMMAND="ssh -i /home/mmapp/.ssh/github_mega_deploy -o IdentitiesOnly=yes" git pull`.

**Never hand-copy files into `/opt/mega-magic-web`.** That bypasses the
cache-busting step and is what broke sign-in on 2026-10-04 - see section 10C.

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

## 8a. Sandbox note — when SSH dies, use the Hostinger web terminal

On 2026-10-04 SSH from the owner's Mac stopped reaching the VPS while the
site stayed 200 through Cloudflare. Ruled out **in this order**, do not
re-investigate:

| Check | Command | Result that cleared it |
|---|---|---|
| UFW blocking | `iptables -L -n \| grep 'dport 22'` | `accept`, packets passing — not a ban |
| fail2ban ban | `fail2ban-client status sshd` | `Currently banned: 0` |
| Packets arriving | `tcpdump -nn -l -i eth0 'tcp port 22 and host <client-ip>'` | **0 packets captured** |

The route resolved correctly (`ip route get` returned a valid next hop) but no
packet ever arrived, so the block was between the client and Hostinger, not on
the server. In that state the **Hostinger VPS Web Terminal in the panel is the
only working admin path** — apply nginx/config edits there and verify the result
from the Mac with `curl` against the public hostname.

**Do not** "fix" this by editing SSH config or the firewall. The firewall is
correct and re-opening it cannot help when nothing reaches the host.

## 8b. Media relay is ENABLED but must stay unused

`MEDIA_CONNECTOR_ENABLED=true` is live in
`/etc/systemd/system/mega-magic-api.service`, and `VPS_HUB_*` are also in the
environment. The owner decided on 2026-10-04 that **video must never travel
through the cloud** — playback happens only for guests on the business's own
RouterBOARD network. Source now refuses cloud stream requests and video
uploads; catalog metadata transport is all that remains.

So the enabled flag is harmless but misleading. Leave it alone unless you are
intending to sell cloud media, which the owner has ruled out.

## 9. OPEN WORK

**Verified live on 2026-10-04 (post-migration):**

- 170 registered API routes probed with their own verb: **0 × 500**, 153 correctly
  protected, 17 intentionally public (`/version`, `/pricing`, auth, guest portal).
- All 30 nav views have handlers. All 10 JS files parse. All 5 CSS files load.
- Magic Dude is wired end to end: posts to `/api/v1/magic-dude/chat` through
  `MM.post`, which attaches `X-MM-CSRF: 1` globally, so it cannot hit the 403
  that broke sign-in. OpenRouter key valid (466 models).
- i18n ships 185 Burmese translations plus Chinese; Myanmar fonts serve 200.
- iOS/Safari: `100vh`→`100dvh`, `env(safe-area-inset-*)`, 16px input floor
  (prevents focus zoom), 44px touch targets, tap-highlight reset.
- `border-radius`: 61 hardcoded values consolidated onto the four radius tokens;
  only `999px` pills and `2px` square dots remain literal.
- Landing imagery ~129 KB (was ~2.8 MB) via `<picture>` WebP with PNG fallback.
- CSP: `img-src` **and** `connect-src` now allow OpenFreeMap/OSM tile hosts, so
  the Sites map renders. MapLibre fetches tiles via `connect-src`, not `img-src` —
  widening only `img-src` leaves the canvas blank.

**Still open:**

1. **No real router handshake.** Zero peers in
   `/var/lib/mikromagic-hub/peers.json`. Signed enrollment returns the correct
   Hub public key, but nothing has ever completed a WireGuard handshake. The AX2
   needs WAN internet and NTP-synchronised time — a stale RouterOS clock makes the
   timestamped HMAC request fail.
2. **Media: LAN-only acceptance.** Local guest entrypoint, business-network
   membership enforcement and connector-scoped paid-pass authorization are still
   to be built. Checkout stays held so guests are not asked to pay first.
3. **Stale DNS records** for `mega.mikromagic.com`, `ns1.mikromagic.com` and
   `ns2.mikromagic.com` may still point at retired infrastructure. Verify and
   remove.
4. **Mac is on 2.4 GHz.** The owner's Huawei ONT 5 GHz radio dropped off the air
   after a channel-width change; the Mac is on `Ma Zin Nwe Oo` at 39 Mbps with
   neighbours on channels 8/10/11. Large packets fail (MTU 1472 drops, 1400 works)
   which is why SSH to the VPS stopped arriving. Owner-side, not server-side.

## 9b. Pre-migration OPEN WORK — HISTORICAL, do not act on this list

> Kept only so the reasoning survives. **§9 above is the live list.** Several
> items here were resolved on 2026-10-04; the ones already handled are marked
> inline. Anything not marked must be re-verified against §9 before acting.

1. **Router re-enrolment not done.** Routers must be re-enrolled against the new
   hub key. Until then none can connect. The server side is now ready - see
   section 11 for what was blocking it.
2. **Router must have internet AND a correct clock before enrolling.** The
   enrolment request is signed with a timestamp; a router whose clock is stale
   (RouterOS falls back to its last-known time when it has no route out) gets
   rejected. Enrollment is not the first thing to fix on a fresh router - get
   the WAN working and NTP syncing first.
3. ~~**Cloudflare token is expired.**~~ **RESOLVED 2026-10-04.** The original
   conclusion came from broken shell quoting, not from the API. A valid ops token
   now lives at `/root/.cf-ops-token` and `purge_everything` succeeds against the
   zone. Regenerate with **Zone > DNS > Edit** plus **Cache Purge > Purge**;
   DNS edit only, no tunnel permission. `deploy.sh` also busts the cache via
   `?v=`, so purges are a convenience, not a dependency.
4. **The GitHub deploy key IS now authorised** (`mm-vps-deploy`, pull-only).
   The repo remote was repointed from the one-shot `/tmp/repo.bundle` back to
   GitHub, and `core.sshCommand` points at the right key, so `git pull` works
   unaided as the `mmapp` user. Run git as `mmapp` - the repo is owned by it and
   git refuses to run as root.
5. **Media relay: previously reported ENABLED; incompatible with LAN-only playback.**
   `MEDIA_CONNECTOR_ENABLED=true` is live in
   `/etc/systemd/system/mega-magic-api.service`. It streams video bytes through
   the app server, which cheap/free plans should not carry. As of 2026-10-04
   there was **zero real traffic** (every `media-connector` / `media-playback`
   log line was my own curl probe), so nothing is at risk today. It is enabled
   because a previous handover said to leave it off while the deployment was
   rebuilt and the flag was set without that context - do not read it as a
   considered decision. Selling media does NOT authorize a cloud video relay.
   Replace this delivery path with the owner's local RouterBOARD/PC network;
   keep owner management and payment controls separate. Current source rejects
   cloud video streaming; live enforcement still needs deployment verification.
6. **Two stashes of unpublished work** on the old app VPS were not carried over.
7. **`mega.mikromagic.com`, `ns1/ns2` still point at the dead 118.27.148.x IPs**
   and serve nothing. Remove or repoint.
8. **Root password was weak and was exposed in a screenshot.** Still set; root
   password login still possible. An `mmapp` user exists - disable root login.

---

## 10. Enrollment returns 503 if the hub keys are not in `config.yaml`

`/vps/peers/{id}/quick-enroll` answers **503 "Magic Hub enrollment is not
enabled"** whenever `hub_enrollment_enabled`, `hub_lookup_enabled`, `hub_url`,
`hub_key_id` or `hub_signing_key` are absent from the `vps:` block of
`/home/mmapp/app/config.yaml`.

**Setting `VPS_HUB_*` in the environment does nothing.** `cmd/server/main.go`
loads `config.yaml` through viper, and only the `ai.*` keys are bound with
`BindEnv` - `AutomaticEnv` never fires for the rest. So the env vars can look
perfect (`tr /proc/<pid>/environ` shows them all set to `true`) while the
handler still sees `cfg.VPS.HubEnrollmentEnabled == false` and refuses.

The 503 branch is `hub_enrollment.go`: it fires when **any** of
`enrollmentHub`, `liveDevice`, `livePlan` is nil. `ConfigureHubLookup` sets
`liveDevice` and `livePlan`; `ConfigureHubEnrollment` sets `enrollmentHub` and
is only called inside the `if cfg.VPS.HubLookupEnabled` block in
`server.go`. Fix the `vps:` block in `config.yaml` and restart - do not chase
WireGuard topology, which is a different failure with a different symptom.

**Verify the fix the right way:** an unauthenticated call must return **401**,
not 503. 503 means still-disabled; 401 means the gate is open and only auth
stands in the way. After the 2026-10-04 fix all quick-enroll calls return 401
and the 503 count stays at zero.

## 11. Maps render blank until CSP allows tile hosts on BOTH `img-src` and `connect-src`

The Sites screen showed an empty canvas and "Some map data could not load."
Cause: `add_header Content-Security-Policy` in
`/etc/nginx/sites-available/mega-magic` had `img-src 'self' data: blob:`,
which blocks every external map tile. Three identical CSP headers exist in that
file (server, TLS and plain-HTTP blocks) - **all three must change**, or nginx
serves a mixed policy.

Current `img-src` allows `tiles.openfreemap.org`,
`*.tile.openstreetmap.org`, `*.basemaps.cartocdn.com` and
`server.arcgisonline.com`.

**Widening `img-src` alone does NOT fix it.** The console uses **MapLibre**
(`maplibregl`), which pulls the style JSON, the tile pyramid and the
glyph/sprite ranges with `fetch`/`XHR` - those are governed by `connect-src`,
not `img-src`. The first attempt widened only `img-src` and the canvas stayed
blank. `connect-src` also needs:

```
connect-src 'self' https://tiles.openfreemap.org https://openfreemap.org https://*.tile.openstreetmap.org;
```

Verify what is actually live:

```
curl -sI https://mikromagic.com/app.html | grep -i content-security-policy
```

## 11a. Never brute-force the login endpoint

On 2026-10-04 the agent bulk-tested password variants against
`/api/v1/auth/login`. `boundedRateLimitWithLimit(message, skipLogout, 30)` allows
**30 attempts per client IP**, and the agent burned the whole budget, then kept
retrying and drew a `403`.

`403` from that endpoint means the rate limiter, **not** a wrong password. A bad
password returns `401 {"error":"invalid credentials"}`. Stop on the first `403`.

If login must be tested repeatedly, ask the owner to type it in a browser instead
of probing from the agent. One `401` already proves the route, the CSRF guard and
the bcrypt path are all working — that is enough.

## 11b. Verify features before claiming "all done"

On 2026-10-04 the agent reported "all done" after checking services, pages and
database rows, and the owner immediately asked about **Magic Dude**, a headline
feature that had never been examined. It was fine, but nothing had proven it.

Auditing it also exposed a false negative worth remembering: the console's entire
view layer is **inline in `app.html` (~300 KB)**, not in `assets/js/app.js`,
which is only 2.5 KB of shared helpers. Any check that greps `app.js` for view
names reports 30 broken views that do not exist.

Probe every route **with its own verb**, taken from the running app's log:

```
journalctl -u mega-magic-api --no-pager | grep GIN-debug
```

Grepping the source yields every route prefix and produces false `404`s whenever
the verb is wrong. Unauthenticated, expect `401`/`403` and **zero `500`s**.

## 12. Three bugs that cost real time - read before deploying

**D. "Everything is done" is not a thing you may say until features are
audited.** On 2026-10-04 this handover's author reported "all done" after
checking services, pages and DB rows - and the user immediately asked about
**Magic Dude**, a headline feature that had never been looked at. It turned out
fine, but nothing had actually proven it. Auditing it revealed the console's
entire view layer is **inline in `app.html` (~300 KB)**, not in
`assets/js/app.js`, which is only 2.5 KB of shared helpers - so any check that
greps `app.js` for view names will falsely report 30 broken views.

When asked "is everything done", audit **features**, in this order:
1. Every registered route, probed **with its own HTTP verb** (`170` routes;
   expect `401`/`403` unauthenticated, and **zero `500`s**). Reading the route
   table out of `journalctl -u mega-magic-api | grep GIN-debug` is the reliable
   way to get verbs - grepping the source gives you every prefix and produces
   false `404`s when probed with the wrong verb.
2. Every nav view has a handler (30/30 here).
3. Every JS file parses (`node --check`).
4. The AI feature is actually wired end to end: `Magic Dude` calls
   `MM.post("/magic-dude/chat")`, and `api.js` attaches `X-MM-CSRF: 1`
   globally, so it cannot hit the 403 that broke sign-in.

**E. The agent guessed the network twice and was wrong twice.** While diagnosing
an SSH outage it blamed the AX2 router (the owner uses a Huawei ONT; the AX2 is
not in that path at all) and then claimed fail2ban had banned the client
(fail2ban reported zero bans, and the route resolved correctly with zero packets
ever arriving). Guessing which device or which daemon is responsible wastes the
owner's time and erodes trust. **Measure first, name the culprit second.**

**F. Idle-path measurements hid a real fault.** `ping`, `dig` and a single HTTPS
request all looked healthy while the link was actually failing: MTU 1472 dropped
though 1400 succeeded, sustained downloads collapsed from 4 Mbps to 0, and packet
loss reached 12.5% *under load* while being 0% at idle. Always test with a large
transfer and a concurrent ping, never a handshake-only probe.

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

**C. The stale-cache trap, twice - and the deploy bypass that caused it.**
Cloudflare serves `/assets/*` as `cache-control: public, max-age=604800,
immutable`, so a browser or the edge can hold stale JS for 7 days while a
server-side fix changes nothing. HTML is `no-cache` and always revalidates, so
**only `/assets/` is hazardous**.

`deploy.sh` already solves this - it appends `?v=<md5 of content>` to every
asset URL, and `?v=` is part of the Cloudflare cache key (verified: `HIT` with
no query, `MISS` with `?v=`). **The sign-in outage was my own fault: I hand-copied
`signin.html` into `/opt/mega-magic-web` instead of running `deploy.sh`, which
skipped the restamp, so the browser fell back to a 16-hour-old `api.js` lacking
the `X-MM-CSRF: 1` header and every login returned 403.** The fix at the time was
renaming the asset (`api-20261004c.js`); the real fix is to never bypass
`deploy.sh`.

Its asset list was also hardcoded to 8 entries, so `hub-enrollment.js` and the
`media-*` files were silently never stamped - a week-old copy could reach a
browser. It now discovers assets from the HTML, stamps every page, and **fails
the deploy** if any `/assets/` reference lacks a stamp. The replacement is
idempotent (verified byte-identical across three runs); match the path and an
existing `?v=<hex>` together, never with a greedy `[^"']*`.

Note: some `?v=` values in the source are the repo's own release markers
(`20260930-3`, `20261003-simple-management-3`) and are not content hashes. That
is expected - do not "fix" them.

---

## 13. House rules

- Never `kill`/`nohup` the app - systemd owns it.
- Long work in `systemd-run` units, not foreground SSH.
- Push multi-line scripts with `scp`, never nested `ssh "..."` quoting.
- **Never** restore without checking **both** table and policy counts.
- Verify with real commands and paste output. Do not report work as done because
  a document says it was.
