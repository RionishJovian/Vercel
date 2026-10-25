#!/usr/bin/env bash
# Executed only by an explicitly requested production workflow. No schema auto-migration.
set -Eeuo pipefail
umask 077
release_dir=${1:?release directory required}
registry_user=${2:?registry user required}
cd "$release_dir"
# One token arrives on encrypted stdin, never in command arguments or logs.
IFS= read -r registry_token
api_image=$(python3 -c 'import json,re;v=json.load(open("release.json"))["api"];assert re.fullmatch(r"ghcr\.io/[a-z0-9_./-]+@sha256:[a-f0-9]{64}",v);print(v)')
web_image=$(python3 -c 'import json,re;v=json.load(open("release.json"))["web"];assert re.fullmatch(r"ghcr\.io/[a-z0-9_./-]+@sha256:[a-f0-9]{64}",v);print(v)')
release_sha=$(python3 -c 'import json,re;v=json.load(open("release.json"))["sha"];assert re.fullmatch(r"[a-f0-9]{40}",v);print(v)')
app=/opt/mega-magic-go-restored-20261018T100554Z
compose="$app/deploy/compose.go-restore.yaml"
test -f "$compose"
# This is the only production app; serialize separate workflow runs too.
exec 9>/var/lock/mega-magic-deploy.lock
flock -n 9 || { echo 'Another deployment holds the production lock'; exit 1; }
export DOCKER_CONFIG
DOCKER_CONFIG=$(mktemp -d)
trap 'rm -rf "$DOCKER_CONFIG"; unset registry_token' EXIT
printf '%s' "$registry_token" | docker login ghcr.io -u "$registry_user" --password-stdin >/dev/null
unset registry_token
docker pull "$api_image"
docker pull "$web_image"
for image in "$api_image" "$web_image"; do
 test "$(docker image inspect --format '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$image")" = "$release_sha"
done
# Refuse an incompatible production config before stopping the old API.
docker run --rm --network mega-magic-go-restored_default -e ENVIRONMENT=production --env-file "$app/.env.production" --mount "type=bind,source=$app/config.go-production.yaml,target=/app/config.yaml,readonly" --entrypoint /app/config-check "$api_image" -config /app/config.yaml
backup="/root/mm-rollout-backups/ci-$release_sha-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "$backup"
cp -p "$compose" "$backup/compose.yaml"
cp -p "$app/config.go-production.yaml" "$backup/config.yaml"
cp -p "$app/restore-bin/mega-magic" "$backup/restore-bin"
docker inspect --format '{{.Image}}' mega-magic-go-restored-api-1 mega-magic-go-restored-web-1 > "$backup/previous-image-ids"
docker exec mega-magic-go-restored-db-1 pg_dump -U mega_magic -d mega_magic -Fc > "$backup/database.dump"
docker exec -i mega-magic-go-restored-db-1 pg_restore -l < "$backup/database.dump" > "$backup/database-list.txt"
test -s "$backup/database-list.txt"
rollback(){
 status=$?;trap - ERR;set +e
 cp -p "$backup/compose.yaml" "$compose"
 cp -p "$backup/restore-bin" "$app/restore-bin/mega-magic"
 (cd "$app" && docker compose -f "$compose" up -d --no-deps --no-build --pull never api web)
 echo "Deployment failed; previous API/web restored. Backup: $backup";exit "$status"
}
trap rollback ERR
# Change only the two selected image references. No unrelated services, keys or flags.
python3 - "$compose" "$api_image" "$web_image" <<'PY'
import pathlib,sys,re,os
p=pathlib.Path(sys.argv[1]);s=p.read_text()
for name,image in [('api',sys.argv[2]),('web',sys.argv[3])]:
 pattern=r'(?ms)(^  '+name+r':\n.*?^    image: )[^\n]+'
 s,n=re.subn(pattern,lambda m:m.group(1)+image,s,count=1);assert n==1
st=p.stat();temp=p.with_suffix(".release.tmp");temp.write_text(s);os.chown(temp,st.st_uid,st.st_gid);os.chmod(temp,st.st_mode&0o777);os.replace(temp,p)
PY
(cd "$app" && docker compose -f "$compose" up -d --no-deps --no-build --pull never api web)
ready=false
for attempt in $(seq 1 30);do
 if curl -fsS http://127.0.0.1:18080/ready -o "$backup/ready.json" && python3 - "$backup/ready.json" <<'PY'
import json,sys
x=json.load(open(sys.argv[1]));assert x['status']=='ready';assert all(x['checks'][k]=='ok' for k in ['database','redis','nats'])
PY
 then ready=true;break;fi
 sleep 2
done
test "$ready" = true
for service in api web;do
 image=$api_image;test "$service" != web || image=$web_image
 expected_id=$(docker image inspect --format '{{.Id}}' "$image")
 test "$(docker inspect --format '{{.Image}}' "mega-magic-go-restored-$service-1")" = "$expected_id"
done
# Public readiness must be JSON too; a website fallback page cannot pass.
curl -fsS https://megamagic.tech/ready -o "$backup/public-ready.json"
python3 - "$backup/public-ready.json" <<'PY'
import json,sys
x=json.load(open(sys.argv[1]));assert x['status']=='ready';assert all(v=='ok' for v in x['checks'].values())
PY
# Keep the staged binary consistent with the selected image for later recovery.
docker cp mega-magic-go-restored-api-1:/app/mega-magic "$app/restore-bin/mega-magic"
printf 'Verified release %s; rollback retained at %s\n' "$release_sha" "$backup"
trap - ERR
