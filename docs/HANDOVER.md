# HANDOVER — MikroMagic / Mega Magic

**Read this before changing anything on the production servers.**

**2026-10-04 source-only continuation:** paid-media bank/MMQR receipt approval,
timed-pass and local-stream foundations were reviewed and validated locally.
They are not a production rollout. After this source commit, do not assume
GitHub/main is identical to the deployed version without checking the live SHA.
Checkout sales must remain off until the guest catalog/player relay and local-PC
acceptance are complete. See CURRENT-HANDOFF.md and MEDIA-PAID-ACCESS.md.
No production change or VPS stash operation was part of this continuation.

Written by Hermes (Mac instance) on 2026-10-04. This document exists so any
agent — including a fresh session with no memory of this work — can pick up
safely. The live system beats this file; if they disagree, verify against the
live system and fix this file in the same session.

---

## 0. THE ONE RULE

**No change to production without a verified rollback.**

Every item below was either applied with a tested rollback, or deliberately
left undone. Do not "finish" anything in section 6 without reading this rule.

---

## 1. The two servers

| | App VPS | Hub VPS |
|---|---|---|
| SSH alias | `mikromagic-app` | `mikromagic-hub` |
| IP | 118.27.148.195 | 118.27.148.183 |
| Specs | 4 core / 15 GB / 96 GB | 4 core / 8 GB / 48 GB |
| Serves | `mikromagic.com` (site + API) | `hub.mikromagic.com` (router tunnel) |
| Actual load | ~1 GB RAM, load 0.01 | ~0.9 GB RAM, load 0.19 |

Canonical domain is **mikromagic.com**. `www` 301-redirects to apex.
Both are behind Cloudflare, so both DNS names resolve to Cloudflare IPs.

**GitHub:** `https://github.com/RionishJovian/Mega` (branch `main`, ~774 files).
The repo is the source of truth and is currently **in sync** with the live site
(verified file-by-file, ignoring `deploy.sh`'s `?v=` cache-busting stamps).

---

## 2. What is actually running (app VPS)

- `nginx` → `root /opt/mega-magic-web` (frontend), proxies `/api/*`, `/health`,
  `/ready` to `127.0.0.1:8080`
- `mega-magic-api.service` → `/home/ubuntu/mega-magic-project/server` (Go binary)
- Docker: `mega_magic_db` (postgres, host port 5433), `mega_magic_cache`
  (redis), `mega_magic_nats`
- Env files: `/etc/mega-magic/{openrouter,billing-telegram}.env`,
  `/etc/mega-magic-hub-lookup.env`

**Never hardcode a secret in a script.** Passwords come from
`/etc/mega-backup.env` via `EnvironmentFile`.

---

## 3. Deploy procedure (verified working)

```bash
ssh mikromagic-app
cd /home/ubuntu/mega-magic-project
git pull --ff-only origin main
git log -1 --oneline          # confirm the commit you expect
./deploy.sh all               # NOT `web` — changes may touch the Go backend
```

`deploy.sh all` validates config → checks DB schema → builds the Go binary →
restarts `mega-magic-api` → rsyncs `web/` to `/opt/mega-magic-web` → reloads
nginx. If a schema check fails, apply it first:

```bash
ENVIRONMENT=production /usr/bin/go run ./cmd/<name>-schema --action apply --config config.yaml
```

**Verified:** repo and live site are identical. A deploy from this box is safe.
(A previous claim that they had diverged was WRONG — it came from a bad
`?v=`-stripping regex. The repo, GitHub, and live site are all the same code.)

---

## 4. Completed work — do not undo

### 4.1 nginx hardening (2026-10-04)
`/etc/nginx/sites-enabled/mega-magic-apex.conf`, backup alongside it.
- `www` → apex **301**
- CSP, HSTS, `nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy`
- unknown paths return **real 404s** (was: 200 + landing page for everything)
- **`immutable` removed from `/assets/`** — see 4.2

### 4.2 WHY `immutable` MUST NOT COME BACK
`add_header Cache-Control "... immutable"` makes Cloudflare pin an asset URL and
**never revalidate**. A correct deploy is then invisible for up to 7 days to
anyone who already loaded the old URL. This is the "I deployed and nothing
changed" bug.

`deploy.sh` content-hashes every asset URL, so a 7-day TTL is safe on its own.
Keep `max-age=604800`. **Never re-add `immutable`.**

### 4.3 `mega-edge-check` — mechanical guard
`/usr/local/bin/mega-edge-check`, runs every 30 min
(`mega-edge-check.timer`, `OnCalendar=*-*-* *:00,30:00`).

```bash
sudo -n mega-edge-check
sudo -n journalctl -u mega-edge-check.service -o cat -n 20
```

Fails on: `immutable` anywhere in `sites-enabled/`, origin re-sending it,
`www` not 301-ing, non-404 unknown paths, any of the 4 security headers
missing, a non-200 real page, `/api/v1/me/profile` not 401, invalid nginx.

**This guard is proven** — a negative test injected `immutable` and the guard
correctly failed, then passed again after restore. Do not "simplify" it; if you
change it, re-run a negative test.

**Known gap:** a Cloudflare purge is NOT automated and no API token exists on
either box. Stale edge copies age out on their own. Do not claim a purge ran.

### 4.4 Backup — healthy, do not "fix"
`mega-db-backup.timer` → `/usr/local/bin/mega-backup.sh` daily at 03:30.
Dumps the REAL production DB (`mega_magic` on 127.0.0.1:5433) to
`/home/ubuntu/backups/mega/`, keeps 7, and **verifies the dump's table count
against the live DB**, failing loudly on mismatch.

**`/home/ubuntu/backups/mega-backup.sh` is DEAD and superseded** — it targets
`mega_magic_dev`, a 20-table database, while production is `mega_magic`. That
exact bug once meant production had no working backup. It is fixed. Leave the
dead file alone or delete it; never re-wire it.

### 4.5 Housekeeping done (2026-10-04)
Removed ~5.5 GB from the app VPS, ~0.4 GB from the hub: Go build cache, logs
>7 days, apt caches, abandoned rollout staging dirs, 465 MB of dated
`deploy-backups`, 74 MB of test tarballs, 5 broken symlinks, stray
`node_modules`. Everything removed is preserved at
`/home/ubuntu/PRE-CLEANUP-BACKUP/` (live site snapshot, full git bundle
verified complete, both stashes, binaries, env files).

---

## 5. SAFETY RULES ON THESE BOXES

**A sudo guard is active.** `sudo -n -l` shows `NOPASSWD: ALL` **minus** a deny
list, which includes:
- `rm -rf /home* /opt* /etc* /root* /var/lib/docker* /usr*` and `-fr` variants
- `docker rm -f*`, `docker image prune*`, `docker volume rm/prune*`, `compose down*`
- `systemctl stop|disable nginx|docker|mikromagic-app`, `mkfs*`, `dd*`, `reboot*`

**These refusals are the owner's decision. Do not work around them.** If a
`rm -rf` under `/home` is refused, either delete the path **as the owning user
without sudo** (legitimate when you own the files) or leave it and say so. Never
restructure the command to evade the pattern.

Other standing rules:
- Never `kill`/`nohup` an app process — it is managed by systemd.
- Long builds/tests go in `sudo systemd-run` units, not a foreground SSH call.
- Push multi-line scripts with `scp`, never `ssh "…"` (nested quotes break).
- Verify with `systemctl is-active` and real artifact greps, not exit codes.

---

## 6. OPEN WORK — nothing here is started

### 6.1 URGENT: 3 of 9 WireGuard peers are memory-only (hub)
```
peers in /etc/wireguard/wg0.conf : 6
peers live in wg show wg0        : 9
```
A hub **reboot loses 3 router connections**. Do this FIRST, before any
migration, because a migration involves restarts.

Persist them: take live blocks from `wg showconf wg0`, append the missing
`[Peer]` blocks to `/etc/wireguard/wg0.conf`, validate with
`wg-quick strip wg0` (never apply a conf that fails strip), then `wg syncconf`.
**Back up the conf first.** Never overwrite the conf with `showconf` output
alone — it carries no `Address`/`MTU` lines.

### 6.2 Possible saving: merge app VPS into hub VPS
App VPS costs ~$100/mo and uses ~7% of its RAM; the hub has 7 GB free and would
fit the app's ~1.5 GB. No customers yet, so downtime risk is low. Suggested
sequence (keep the app VPS until the hub has run cleanly for days):

1. Fix 6.1
2. Copy app files, DB dump, and `/etc/mega-magic/*` to the hub
3. Add a SECOND nginx vhost on the hub for `mikromagic.com` (the hub's existing
   vhost is `hub.mikromagic.com`; they coexist by `server_name`)
4. Issue a Let's Encrypt cert on the hub (current one lives on the app box)
5. Verify everything on the hub **while the app box still serves traffic**
6. Only then repoint DNS — rollback = point it back

Port notes: hub has postgres on 5432 and RouterOS VM on 8291/8728/8729/2222;
keep the app's DB on 5433. The app's own `wg-lab` tunnel to the hub becomes a
loopback once co-hosted — repoint to `127.0.0.1` (optional cleanup).

**Blocking risk if ignored:** co-hosting means one failure takes down both the
site and router reachability. Today they are independent — a hub outage does
NOT take the website down. Losing that separation is a real cost.

### 6.3 Undecided: two stashes on the app VPS
- `stash@{0}` "before one-paste Hub deploy" — 55 files, 5,411 insertions.
  Touches every HTML page + CSS/JS + ~47 backend files. **Verified NOT live.**
  Genuine unpublished work (subscription billing, media publishing, one-paste
  Hub enrollment). *Owner has not decided whether to ship or discard it.*
- `stash@{1}` "owner-authorized playground cleanup" — 3 files, backend only.

Both are inside the verified bundle at `PRE-CLEANUP-BACKUP/`.
**Do not pop or drop them without the owner's explicit decision.**

### 6.4 `mega.mikromagic.com` — temporary dev bridge, still live
Returns 200 and proxies to the app API. Its own config comments call it
temporary and not for production. It is a second unlocked front door. Owner has
not decided whether to restrict or remove it. Do not disable unilaterally.

### 6.5 Stale `sites-available/mega-magic-apex.conf`
Dead copy still containing the OLD `immutable` line. Not loaded (nginx includes
only `sites-enabled/`), but a trap for the next editor. Awaiting owner decision.

---

## 7. Housekeeping / audit scripts (on the app VPS, `/tmp`)

`/tmp/mega-edge-check.sh` (installed to `/usr/local/bin`), `/tmp/app-audit.sh`,
`/tmp/repo-vs-live.sh`, `/tmp/exact-diff.sh`, `/tmp/negtest2.sh`. These are
throwaway; the only durable ones are `mega-edge-check` and the systemd units.

---

## 8. If you are an agent without memory of this

Trust the live system over any claim here, including claims in this file.
Verify with a real command, paste the output, and do not report work as done
because a document says it was.
