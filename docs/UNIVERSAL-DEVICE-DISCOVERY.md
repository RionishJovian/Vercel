# Universal Device Discovery — mixed-brand network acceptance contract

## Purpose
A site does **not** require a predetermined list of exact iKuai, Ruijie/Reyee, Huawei or TP-Link models. MikroMagic identifies equipment from verifiable observations and enables only controls that the individual device and its management interface actually support.

## Phase 1 in PR #170
The Devices page includes an on-demand **Universal device discovery** section. It invokes `GET /api/v1/routers/:id/discovery`, which first proves the requested gateway belongs to the active business and then reuses the existing read-only, per-device authenticated MikroTik observation connection.

Already available source menus:
- Gateway interface MAC table: removes the gateway's own MACs.
- ARP and DHCP lease tables: IP/MAC evidence, **not proof of reachability**.
- Router MNDP/CDP/LLDP neighbor table: reported identity/platform/board and reported gateway port; **not a verified cable**.
- Bridge host forwarding: MACs learned through a port, potentially multiple switch hops downstream.
- Wi-Fi client registration / HotSpot active: client presence at collection time, **not device management**.

The backend normalizes and merges only valid unicast MAC addresses, bounds each result to **512 MACs / 2500 rows**, preserves per-source success/failure, and never gives partial data a green/complete status. Locally administered MACs are flagged as potentially randomized. Router-observed vendor/model strings are *reported hints*, not proof of authentication, exact device model or supported configuration endpoints. Owner-authored device labels are display annotations only.

Each discovered identity returns a conservative capability matrix: `gateway_observation=observed` with its source evidence; third-party management, AP client info, SSID control, RF optimization and physical topology are **unknown** until directly verified. No discovered device is automatically registered as managed, configured, rebooted, scanned or authenticated against. Unavailable source menus remain unavailable, not empty-success.

## Limits
- A silent switch or AP invisible to gateway neighbor/bridge/DHCP/ARP tables may not be discovered.
- DHCP clients behind NAT or a second home router may appear only as the router's WAN-facing MAC, not all clients behind it.
- SSIDs, client accounting, transmit power, actual physical links and management credentials cannot safely be inferred from MAC/vendor labels.
- SNMP, controller APIs, and management protocols need additional **opt-in, tenant-bound** source adapters. Passive gateway observation does not authorize querying arbitrary LAN IPs.
- LLDP remote neighbor data can help identify the immediate hop only after protocol support, collection provenance, interface identity and MAC relationships are verified. A RouterOS neighbor entry alone does not prove the physical parent for a downstream AP.

## Next phase
1. Add an explicit managed-source registration and approval model (no open LAN scanner): gateway's existing observations, a known controller inventory, and tenant-credentialed SNMP/LLDP adapters when configured.
2. Validate source schemas and preserve observed-at time, tenant/device site, partial failures and lease expiry.
3. Correlate only verified same-device identities, without merging hosts simply because they share an IP, name or manufacturer.
4. Add model-independent capability probes with safe read-only adapters, per-vendor/mode support, and no inferred write access.
5. Merge graph changes from draft PR #162 only after reconciliation with this discovery contract and CI. Physical links should remain marked unverified until independent evidence supports the hop.
6. Keep owner-confirmed reversible configuration changes behind separate verified backup and per-adapter write gates.

**No hardware acceptance or production activation is claimed by CI.** Hardware testing is deferred by owner decision until the four feature areas are ready to test together.
