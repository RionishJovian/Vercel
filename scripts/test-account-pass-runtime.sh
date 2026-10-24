#!/usr/bin/env bash
# Exercise the actual expiry tests inside the exact minimal deployment image.
# Uses no database, network, credentials or running application container.
set -euo pipefail
image=${1:?Supply the existing Linux AMD64 API image to verify}
scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -c -o "$scratch/accountpass.test" ./internal/accountpass
chmod 755 "$scratch/accountpass.test"
docker run --rm --network none --read-only --cap-drop ALL --security-opt no-new-privileges \
  --mount "type=bind,source=$scratch/accountpass.test,target=/tmp/accountpass.test,readonly" \
  --entrypoint /tmp/accountpass.test "$image" -test.v
