# CHR Lab Router — breakable RouterOS test target

Stand up a disposable RouterOS 7.24.4 (Containerized RouterOS, MikroTik) for
testing the real Provisioner Applier WITHOUT touching production.

## Coordinates (current)
- Container: chr-lab (image mikrotik/chr:stable), on mega_magic_dev_network
- RouterOS 7.24.4; SSH enabled, admin/empty, mgmt IP 172.20.0.250
- SSH banner: SSH-2.0-ROSSSH  →  reachable from host at 172.20.0.250:22
- WG tunnel iface: wg-magichub (listen-port 13231), IP 10.99.0.3/30
  (SANDBOX range, isolated from prod 10.77.0.0/24 — never 10.77.0.x),
  peer `peer2` allowed-address 10.99.0.1/32 (sandbox hub), keepalive 25s
  (state VERIFIED live 2026-09-25 via SSH reprint: peer=10.99.0.1/32, iface=10.99.0.3/30)
- Lab keypair: priv /tmp/lab_priv, pub /tmp/lab_pub
- Live CCR2004 reference (Kyaw Caffee, PROD — never point the lab at it):
  iface=magic-cloud, pubkey=<<in private KB, not here>>, endpoint hub.mikromagic.com:51820,
  allowed-address 10.77.0.1/32, device IP 10.77.0.4/24 gw 10.77.0.0, keepalive 25s.
- Snapshot: /system backup saved as lab-initial; export file lab-default.rsc

## Pitfalls (learned)
- CHR needs --privileged + a bridge network; it QEMU-boots a real RouterOS VM.
- First login forces a password; skip with Ctrl-C to keep default admin/empty.
- Management NIC inside RouterOS is `ether1` (NOT eth0).
- Serial console via docker attach / -it PTY echoes chars-mangled; drive with
  `process write data="<cmd>\r"` and verify from the host, not the console.
- `docker rm -f` is blocked by sudo policy; use `docker stop` + `docker rm`.
- /ip address add address=172.20.0.250/16 interface=ether1  → then host can ping.
- Verify reachability from HOST (docker exec ping may fail asymmetrically).
