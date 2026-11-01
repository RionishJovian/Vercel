# Rotating the login-token secret and the credential key

Use this when either secret may have been seen by someone who should not have
it (for example printed into a terminal log). Both live only in
`/opt/mega-magic-go-restored-20261018T100554Z/.env.production` (root, mode 0600).
The YAML config holds empty values, and the rollout backups do not contain them.

| Secret (env name) | What it protects | Effect of rotating |
|---|---|---|
| `SECURITY_JWT_SECRET` | Signs login tokens and the guest hotspot cookies | Everyone is signed out once (including you); guests lose their "resume" cookie and re-enter their code. No data changes. |
| `PROVISIONER_APPLIER_CREDENTIAL_KEY` | Seals each router's stored SSH login and the short-lived Hub enrollments | Stored values must be re-sealed with the new key in one step (tool below). A mistake makes the app unable to log in to routers until fixed. |

There is no old-key fallback in the app, so each secret changes in one cut-over.
Do both in a single short window (about a minute of API downtime).

## Before you start
1. Deploy a release that contains `rotate-credential-key` (it is built into the API image, dormant unless run).
2. Pick a moment when the API can be down for about a minute. Guest internet already running on the router is not affected; app management and voucher sync pause.
3. Keep one SSH session open for the whole procedure: the new values exist only in shell variables there. Do not paste them anywhere.

## 1. Dry run (API stays up)
```bash
ssh -T mikromagic-hostinger
set -euo pipefail; umask 077
app=/opt/mega-magic-go-restored-20261018T100554Z
compose=$app/deploy/compose.go-restore.yaml
img=$(docker inspect mega-magic-go-restored-api-1 --format '{{.Config.Image}}')
NEW_CRED=$(openssl rand -hex 32)      # never echo this
rotate() {  # usage: rotate [-apply]
  docker run --rm --network mega-magic-go-restored_default -e ENVIRONMENT=production \
    --env-file "$app/.env.production" --env-file <(printf 'MM_NEW_CREDENTIAL_KEY=%s\n' "$NEW_CRED") \
    --mount type=bind,source=$app/config.go-production.yaml,target=/app/config.yaml,readonly \
    --entrypoint /app/rotate-credential-key "$img" -config /app/config.yaml "$@"
}
rotate
```
Expected: `DRY RUN (verified, rolled back; nothing changed)` with the counts of device
logins and pending enrollments. Any error here (for example "cannot be opened with
the current key") means stop and investigate; nothing has been changed.

## 2. The window
```bash
backup=/root/mm-rollout-backups/rotate-$(date -u +%Y%m%dT%H%M%SZ); mkdir -p "$backup"
cp -p "$app/.env.production" "$backup/env.production"        # OLD secrets; root-only, deleted in step 4
docker exec mega-magic-go-restored-db-1 pg_dump -U mega_magic -d mega_magic -Fc > "$backup/database.dump"
test -s "$backup/database.dump"

# Prepare the new environment file first, so nothing can fail between the data change and the key change.
NEW_JWT=$(openssl rand -hex 32)
NEW_CRED="$NEW_CRED" NEW_JWT="$NEW_JWT" python3 - "$app/.env.production" "$app/.env.production.new" <<'PY'
import os, pathlib, re, sys
s = pathlib.Path(sys.argv[1]).read_text()
for name, var in (("PROVISIONER_APPLIER_CREDENTIAL_KEY", "NEW_CRED"), ("SECURITY_JWT_SECRET", "NEW_JWT")):
    s, n = re.subn(r"(?m)^" + name + r"=.*$", lambda m, name=name, var=var: name + "=" + os.environ[var], s)
    assert n == 1, name
out = pathlib.Path(sys.argv[2]); out.write_text(s); os.chmod(out, 0o600)
PY

docker stop mega-magic-go-restored-api-1
rotate -apply                                                  # re-seals and verifies every value, then commits
mv "$app/.env.production.new" "$app/.env.production"
( cd "$app" && docker compose -f "$compose" up -d --no-deps --force-recreate api )
for i in $(seq 1 30); do curl -fsS http://127.0.0.1:18080/ready >/dev/null && break; sleep 2; done; curl -fsS http://127.0.0.1:18080/ready
```
**If anything fails after `rotate -apply` succeeded:** do not run it again. The
stored values are now sealed with `NEW_CRED`; finishing the `mv` and the
`docker compose ... up` steps above is the whole fix. If `rotate -apply` itself
failed, nothing changed: restore with `docker start mega-magic-go-restored-api-1`.

## 3. Verify
- Open https://megamagic.tech and sign in again (expected after a JWT rotation).
- Server log, within two minutes: `docker logs --since 3m mega-magic-go-restored-api-1 | grep "synchronization pending"`. If the router is online the voucher sync reads "Voucher login accounting synchronized". If it is offline the error must be a **dial timeout** (the app decrypted the login and tried to connect). An error saying **"no ssh target configured"** means the app cannot decrypt the stored login: stop and use step 5.
- Optional proof that every stored value opens under the current key: run the dry run again (`NEW_CRED=$(openssl rand -hex 32); rotate`); it must report the same counts and no error.

## 4. Clean up
```bash
shred -u "$backup/env.production"        # the old secrets
unset NEW_CRED NEW_JWT
```
Database dumps taken before the rotation (in `/root/mm-rollout-backups/*/database.dump`) still hold the router login sealed with the **old** key. They are root-only on the server; an attacker would need both a dump and the old key. Delete old dumps on your normal retention schedule.

## 5. Recovery if the app cannot decrypt after step 2
The data is sealed with `NEW_CRED` and the env must hold the same value. If you
still have the shell: put `NEW_CRED` into `.env.production` and recreate the API.
If the shell is gone, restore `$backup/database.dump` (pre-rotation data) together
with `$backup/env.production` (old key); that returns the system to exactly how it
was before step 2.

## What the tool guarantees
Single transaction with both tables locked; any value the current key cannot
open aborts everything; a new key equal to the current one is refused; every
re-sealed value is opened again with the new key and shown unreadable with the
old key before commit; expired pending enrollments are deleted rather than kept;
the output contains counts only. Tested against PostgreSQL 17 in
`TestDBRotateCredentialKeyIsAtomicVerifiedAndReversibleUntilCommit`.
