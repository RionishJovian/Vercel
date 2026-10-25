#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
release=/root/mm-individual-0291edc
app=/opt/mega-magic-go-restored-20261018T100554Z
api=mega-magic-go-restored-api-1
web=mega-magic-go-restored-web-1
db=mega-magic-go-restored-db-1
compose="$app/deploy/compose.go-restore.yaml"
config="$app/config.go-production.yaml"
expected=ae4c24a835fac5d05be0dc81c46b1293401fd359f1de7e11d18ae2e6283a175c
old_api=mega-magic-api:066f46a
old_web=mega-magic-web:af4282c
new_api=mega-magic-api:0291edc
new_web=mega-magic-web:0291edc
cd "$release"
# Existing access and runtime identities must still match the reviewed base.
test "$(docker inspect "$api" --format '{{.Config.Image}}')" = "$old_api"
test "$(docker inspect "$web" --format '{{.Config.Image}}')" = "$old_web"
test "$(docker exec "$api" sha256sum /app/mega-magic | cut -d' ' -f1)" = 91f5a783a9b547f0018f55d53f8df75a875e322d85d6563ed37b3be538383076
test "$(docker exec "$db" psql -U mega_magic -d mega_magic -Atc "SELECT to_regclass('public.app_account_pass_rollouts') IS NOT NULL")" = t
python3 - <<'PY'
import json,hashlib,pathlib
m=json.load(open('release.json'))
for f,expected in m['files'].items():
 assert hashlib.sha256(pathlib.Path(f).read_bytes()).hexdigest()==expected,f
print('Source manifest verified:',m['commit'])
PY
# Independent VPS compilation must exactly match the local Linux build.
(cd source && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOTOOLCHAIN=go1.26.5 go build -trimpath -buildvcs=false -o "$release/mega-magic" ./cmd/server)
echo "$expected  mega-magic" | sha256sum -c -
# Build immutable images before touching the running app or database.
cat > Dockerfile.api <<'DOCKER'
FROM mega-magic-api:066f46a
COPY --chmod=755 mega-magic /app/mega-magic
LABEL org.opencontainers.image.revision="0291edc"
DOCKER
cat > Dockerfile.web <<'DOCKER'
FROM mega-magic-web:af4282c
COPY source/web/app.html /srv/app.html
COPY source/web/pricing.html /srv/pricing.html
LABEL org.opencontainers.image.revision="0291edc"
DOCKER
docker build --pull=false -f Dockerfile.api -t "$new_api" .
docker build --pull=false -f Dockerfile.web -t "$new_web" .
stamp=$(date -u +%Y%m%dT%H%M%SZ)
backup="/root/mm-rollout-backups/individual-0291edc-$stamp"
mkdir -p "$backup"
printf '%s\n' "$backup" > backup-path.txt
cp -p "$compose" "$backup/compose.yaml"
cp -p "$config" "$backup/config.yaml"
cp -p "$app/restore-bin/mega-magic" "$backup/restore-bin"
docker cp "$api:/app/mega-magic" "$backup/runtime-bin"
docker cp "$web:/srv/app.html" "$backup/app.html"
docker cp "$web:/srv/pricing.html" "$backup/pricing.html"
docker inspect "$api" --format '{{.Image}}' > "$backup/api-image-id"
docker inspect "$web" --format '{{.Image}}' > "$backup/web-image-id"
docker exec "$db" pg_dump -U mega_magic -d mega_magic -Fc > "$backup/database.dump"
test -s "$backup/database.dump"
docker exec -i "$db" pg_restore -l < "$backup/database.dump" > "$backup/database-list.txt"
test -s "$backup/database-list.txt"
sha256sum "$backup/database.dump" > "$backup/database.dump.sha256"
docker tag "$old_api" "mega-magic-api:before-individual-$stamp"
docker tag "$old_web" "mega-magic-web:before-individual-$stamp"
printf 'Validated backup: %s\n' "$backup"
rollback(){
 status=$?
 docker logs --tail 150 "$api" > "$backup/failed-api.log" 2>&1 || true
 docker inspect "$api" --format '{{json .State}}' > "$backup/failed-api-state.json" 2>&1 || true
 trap - ERR
 set +e
 cp -p "$backup/compose.yaml" "$compose"
 cp -p "$backup/config.yaml" "$config"
 cp -p "$backup/restore-bin" "$app/restore-bin/mega-magic"
 cp -p "$backup/app.html" "$app/web/app.html"
 cp -p "$backup/pricing.html" "$app/web/pricing.html"
 docker exec "$db" psql -U mega_magic -d mega_magic -c "UPDATE app_account_pass_rollouts SET enforcement_enabled=false WHERE rollout_key='individual-v1'" >/dev/null 2>&1
 (cd "$app" && docker compose -f deploy/compose.go-restore.yaml up -d --no-deps --no-build --pull never api web)
 printf 'ROLLED BACK application; additive history retained. Backup %s\n' "$backup"
 exit "$status"
}
trap rollback ERR
# Additive schema and once-only transition; no pass or payment is approved.
test -s source/migrations/20261024040000_individual_pass_rollout.sql # Already applied and verified after the first rollback.
docker exec -i "$db" psql -v ON_ERROR_STOP=1 -U mega_magic -d mega_magic < source/deploy/activate-individual-passes.sql
# Preserve every other configuration byte when adding/replacing the billing map.
python3 - "$config" "$compose" <<'PY'
import os,pathlib,sys,yaml
p=pathlib.Path(sys.argv[1]);text=p.read_text();before=yaml.safe_load(text);root=yaml.compose(text)
match=[(k,v) for k,v in root.value if k.value.lower()=='billing'];assert len(match)<=1
key=match[0][0].value if match else 'billing'
b=dict(before.get(key) or {})
for k in list(b):
 if k.lower() in ['individual_passes_enabled','subscription_only']:del b[k]
b.update(individual_passes_enabled=True,subscription_only=False)
block=yaml.safe_dump({key:b},sort_keys=False)
if match:
 k,v=match[0];updated=text[:k.start_mark.index]+block+text[v.end_mark.index:]
else:updated=text.rstrip()+'\n'+block
expected=dict(before);expected[key]=b
assert yaml.safe_load(updated)==expected,'unexpected configuration change'
original=p.stat();tmp=p.with_suffix('.individual.tmp');tmp.write_text(updated);os.chown(tmp,original.st_uid,original.st_gid);os.chmod(tmp,original.st_mode&0o777);os.replace(tmp,p);assert (p.stat().st_uid,p.stat().st_gid,p.stat().st_mode&0o777)==(original.st_uid,original.st_gid,original.st_mode&0o777)
c=pathlib.Path(sys.argv[2]);s=c.read_text()
for old,new in [('image: mega-magic-api:066f46a','image: mega-magic-api:0291edc'),('image: mega-magic-web:af4282c','image: mega-magic-web:0291edc')]:
 assert s.count(old)==1,old;s=s.replace(old,new)
c.write_text(s)
PY
docker run --rm --network none --mount "type=bind,source=$config,target=/app/config.yaml,readonly" --entrypoint sh "$new_api" -c 'test -r /app/config.yaml && echo CONFIG_READABLE_AS_API_USER'
cp mega-magic "$app/restore-bin/mega-magic"
cp source/web/app.html "$app/web/app.html"
cp source/web/pricing.html "$app/web/pricing.html"
(cd "$app" && docker compose -f deploy/compose.go-restore.yaml up -d --no-deps --no-build --pull never api web)
ready=false
for attempt in $(seq 1 15); do
 if curl -fsS http://127.0.0.1:18080/ready > ready.json; then ready=true;break;fi
 docker logs --tail 50 "$api" > "$backup/attempt-api.log" 2>&1 || true
 if grep -qE 'FATAL|panic:' "$backup/attempt-api.log"; then break; fi
 sleep 2
done
test "$ready" = true
test "$(docker exec "$api" sha256sum /app/mega-magic | cut -d' ' -f1)" = "$expected"
curl -fsS https://megamagic.tech/app.html -o public-app.html
curl -fsS https://megamagic.tech/pricing.html -o public-pricing.html
cmp source/web/app.html public-app.html
cmp source/web/pricing.html public-pricing.html
curl -fsS https://megamagic.tech/api/v1/pricing -o public-policy.json
python3 - <<'PY'
import json
p=json.load(open('public-policy.json'))['account_policy']
assert p['mode']=='individual_pass' and p['routers']==1 and p['sites']==1 and p['same_features']
print('Live public policy matches individual client enforcement')
PY
docker exec "$db" psql -U mega_magic -d mega_magic -c "SELECT starts_at,transition_days,enforcement_enabled FROM app_account_pass_rollouts; SELECT count(*),min(expires_at),max(expires_at) FROM app_account_pass_transitions; SELECT count(*) AS passes FROM app_account_passes; SELECT count(*) AS renewal_requests FROM app_account_pass_renewals;"
docker ps --format '{{.Names}} {{.Status}} {{.Image}}'
printf 'DEPLOYED 0291edc with validated rollback %s\n' "$backup"
trap - ERR
