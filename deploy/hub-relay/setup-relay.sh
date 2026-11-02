#!/usr/bin/env bash
# Forwards UDP WireGuard traffic from this relay server to the Magic Hub, for routers
# whose internet provider cannot reach the Hub's own address. The relay sees only
# encrypted WireGuard packets; keys stay on the router and the Hub. Run once as root
# on a fresh Ubuntu server. Safe to run again.
set -euo pipefail

HUB_IP="${HUB_IP:-156.67.216.128}"
PORT="${PORT:-51820}"

[ "$(id -u)" -eq 0 ] || { echo "run as root"; exit 1; }
[[ "$HUB_IP" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "HUB_IP must be an IPv4 address"; exit 1; }
[[ "$PORT" =~ ^[0-9]+$ ]] && [ "$PORT" -ge 1 ] && [ "$PORT" -le 65535 ] || { echo "PORT must be 1-65535"; exit 1; }

IFACE="$(ip -4 route show default | awk '{print $5; exit}')"
[ -n "$IFACE" ] || { echo "no default network interface found"; exit 1; }

export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
echo iptables-persistent iptables-persistent/autosave_v4 boolean false | debconf-set-selections
echo iptables-persistent iptables-persistent/autosave_v6 boolean false | debconf-set-selections
apt-get install -y -qq iptables iptables-persistent >/dev/null

# Forwarding on, now and after a reboot.
cat > /etc/sysctl.d/99-hub-relay.conf <<'EOF'
net.ipv4.ip_forward=1
EOF
sysctl -q --system

rule() { # rule <table> <chain> <spec...>: add only if missing
  local table="$1" chain="$2"; shift 2
  iptables -t "$table" -C "$chain" "$@" 2>/dev/null || iptables -t "$table" -A "$chain" "$@"
}

# Rewrite the destination to the Hub and the source to this relay, so the Hub's
# replies come back through it. Only this one UDP port is forwarded.
rule nat PREROUTING  -i "$IFACE" -p udp --dport "$PORT" -j DNAT --to-destination "$HUB_IP:$PORT"
rule nat POSTROUTING -o "$IFACE" -p udp -d "$HUB_IP" --dport "$PORT" -j MASQUERADE
rule filter FORWARD -p udp -d "$HUB_IP" --dport "$PORT" -j ACCEPT
rule filter FORWARD -p udp -s "$HUB_IP" --sport "$PORT" -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT

netfilter-persistent save >/dev/null

echo "Relay ready: UDP $PORT on $(hostname -I | awk '{print $1}') -> $HUB_IP:$PORT (interface $IFACE)."
echo "Allow UDP $PORT (and SSH) in the provider's firewall; nothing else needs to be open."
