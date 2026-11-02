# Hub relay for routers whose provider cannot reach the Hub

Some internet providers drop traffic to the Hub's address range (found 2026-11-02 on
103.197.156.x: ping, HTTPS and WireGuard to 156.67.216.128 and its neighbour
156.67.216.1 all failed while the same router reached Cloudflare and Vultr Singapore).
A router there completes the one-paste setup but never handshakes, and the app keeps
saying no recent tunnel connection is verified.

## Confirm it is the provider path first
On the Hub (`ssh mikromagic-hostinger`):
```bash
wg show wg0 latest-handshakes        # the router's peer shows 0 = never
timeout 25 tcpdump -nn -i any "udp port 51820 and src host <router public IP>" -c 3
```
No packets in 25 seconds while the router retries means the packets never leave the
provider. On the router: `/ping 8.8.8.8` works, `:put [:resolve megamagic.tech]`
returns the Hub address, `/ping 156.67.216.128` and `/tool fetch url=https://megamagic.tech/ready`
fail, and `/tool traceroute 156.67.216.128` stops early. If packets do arrive, it is not
this problem.

## The fix: a relay on a provider the router can reach
A small server forwards the router's UDP WireGuard packets to the Hub. It sees only
encrypted packets; the keys stay on the router and the Hub, and nothing in the app
changes. Check first that the router can reach the relay provider
(`/ping <provider test host>`; Vultr Singapore answered in 36 ms, Linode Singapore did not).

1. Create the smallest Ubuntu 24.04 server at that provider, in a nearby region. In its
   firewall allow only SSH and UDP 51820.
2. On it, as root: `bash setup-relay.sh` (from `deploy/hub-relay/`). It turns on IPv4
   forwarding and adds four idempotent iptables rules that forward UDP 51820 to
   `156.67.216.128:51820`, then saves them. Override with `HUB_IP=... PORT=... bash setup-relay.sh`.
3. On the router, point its peer at the relay (use the real interface name from
   `/interface wireguard print`, and the relay's public IP):
   ```
   /interface wireguard peers set [find interface=mmh-XXXXXXXXXXXX] endpoint-address=<RELAY_IP> endpoint-port=51820
   ```
4. Verify on the Hub: `wg show wg0 latest-handshakes` shows a recent time for the router's
   peer, and in the app the Router tab turns Online. On the router, `/ping 10.77.0.1` answers.
5. Roll back by setting `endpoint-address=megamagic.tech` again; destroy the server to stop the cost.

## Notes
- The Hub sees the relay's address as the router's endpoint. That is expected.
- One relay is one more thing that can fail; keep the keepalive (25 s) so the tunnel stays up.
- The relay only helps the tunnel. If customers on the same provider cannot open
  `megamagic.tech` in a browser, put the domain behind Cloudflare's free proxy (DNS is
  currently at Hostinger parking) or have them use a connection that can reach it.
- Longer term, the one-paste script could take an endpoint override per router so no manual
  router change is needed.
