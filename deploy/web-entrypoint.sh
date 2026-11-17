#!/bin/sh
# The host Caddy reaches this container through the published port, so its source address here is
# the compose network's gateway (the container's default route). Trust only that one address so
# the client's X-Forwarded-For survives to the API; detection failure falls back to loopback, which
# trusts nobody (the old behaviour).
gw=$(ip route 2>/dev/null | awk '$1=="default"{print $3; exit}')
case "$gw" in
  *[!0-9.]*|"") gw=127.0.0.1 ;;
esac
export MM_EDGE_PROXY="$gw/32"
exec "$@"
