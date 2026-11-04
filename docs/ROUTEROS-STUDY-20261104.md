# RouterOS study continuation — 2026-11-04

Continues the four manual bookmarks left in the November 2 Claude checklist.
The earlier hardening work is retained. This is an integration reference, not
an instruction to install features or change a router.

The official pages reviewed currently identify themselves as RouterOS 7.26.
Use the connected router's version, packages, model and actual read responses
before using any property. In particular, do not assume these pages describe
every capability of the previously reported 7.12.2 router.

## 1. Management tools

Sources: [section](https://manual.mikrotik.com/docs/management-tools/),
[SSH](https://manual.mikrotik.com/docs/management-tools/ssh/),
[console and Safe Mode](https://manual.mikrotik.com/docs/management-tools/console/).

RouterOS supports remote command execution through SSH. Its console Safe Mode
is session-based, has a finite undo history, and does not cover reboot-required
operations such as restoring a backup. It is not a general transaction wrapper
for independent web requests. The SSH manual also identifies version-dependent
multiline-command behavior; do not introduce new multiline assumptions for
older routers.

Application mapping:

- Keep the existing Go transport in `internal/provisioner/routeros/transport.go`
  and saved Magic Hub management access. The transport supports fingerprint
  checking; a device label or neighbor announcement cannot replace it.
- Keep browser input limited to typed operations, not arbitrary console text.
  `network_control.go` owns command menus, single-use review tokens, per-router
  locks, fresh-plan comparisons, snapshots, router-side restoration and readback.
- Preserve that restoration mechanism. Do not replace it with a claim that SSH
  commands automatically execute in Safe Mode.
- Keep WinBox as a local troubleshooting tool, rather than replacing Mega
  Magic's screens with an embedded third-party management interface.

## 2. System information and utilities

Sources: [section](https://manual.mikrotik.com/docs/system-information-and-utilities/),
[services](https://manual.mikrotik.com/docs/system-information-and-utilities/services/),
[device mode](https://manual.mikrotik.com/docs/system-information-and-utilities/device-mode/),
[scheduler](https://manual.mikrotik.com/docs/system-information-and-utilities/scheduler/),
[neighbor discovery](https://manual.mikrotik.com/docs/system-information-and-utilities/neighbor-discovery/).

Management-service restrictions must retain the current management source.
Service address restrictions are not a replacement for firewall protection.
Device-mode changes require physical confirmation and reboot; they cannot be
silently completed as a remote settings update. Scheduler startup execution can
precede network availability, and startup plus a recurring interval has different
timing from a one-time startup entry.

Discovery uses MNDP/CDP/LLDP. Including a bridge or bond in its interface list
also includes its members. Removing participation stops both sending and
receiving; receive-only behavior is a separate setting where supported.

Application mapping:

- Reuse `telemetry.go`, `telemetry_extra.go` and `routertelemetry/service.go` for
  observations. Preserve source timestamps, partial failures and unknown values.
- `observations.go` currently collects neighbor identity, board, platform,
  addresses and interface, plus bridge forwarding records. Discovery can enrich
  topology labels; neither it nor a forwarding port proves a direct cable hop
  or an authenticated device identity.
- Future discovery-setting reads must resolve interface-list membership before
  identifying WAN exposure. A bridge name alone does not describe participation.
- Review service, clock and scheduler changes against Hub reachability and
  existing voucher expiry behavior. Do not reset first use or recreate existing
  voucher jobs as part of discovery or monitoring.

## 3. Bridging and switching

Sources: [bridging and switching](https://manual.mikrotik.com/docs/bridging-and-switching/),
[VLAN interfaces](https://manual.mikrotik.com/docs/bridging-and-switching/vlan/).

A VLAN interface and bridge VLAN filtering are different configuration layers.
Management/routing through a VLAN-filtering bridge needs appropriate bridge
(CPU-port) membership. The manual dates automatic membership when adding a VLAN
interface to a filtering bridge to 7.16; do not assume it on 7.12.2. Some bridge
and Ethernet changes can reset a switch chip and interrupt ports, depending on
the hardware. Offloading behavior is model-specific.

Application mapping:

- Preserve the scope in `network_addresses.go` and `docs/NETWORK-ADDRESS-VLAN.md`:
  constrained IP/VLAN operations, not an arbitrary bridge/trunk editor.
- Do not infer working VLAN forwarding from successful VLAN-interface creation.
  Future bridge editing needs explicit port roles, tagged/untagged membership,
  PVID, ingress/frame handling, management reachability and hardware capability.
- Keep the protections for Hub addresses, uplinks, bridge-member addressing,
  serving interfaces and dynamic records. Display observed forwarding
  associations without inventing intermediate switches.
- Validate any future bridge-changing plan in an isolated environment and retain
  independent recovery before offering it for a physical client router.

## 4. Firewall and quality of service

Sources: [section](https://manual.mikrotik.com/docs/firewall-and-quality-of-service/),
[firewall](https://manual.mikrotik.com/docs/firewall-and-quality-of-service/firewall/),
[packet flow](https://manual.mikrotik.com/docs/firewall-and-quality-of-service/packet-flow-in-routeros/),
[connection tracking](https://manual.mikrotik.com/docs/firewall-and-quality-of-service/connection-tracking/),
[queues](https://manual.mikrotik.com/docs/firewall-and-quality-of-service/queues/).

Firewall rule order and the traffic path matter: input protects router services,
forward handles routed client traffic, and bridge forwarding follows its own
path. IPv4 FastTrack bypasses simple queues, global-parent queue trees and other
facilities, including the Hotspot universal client. Configuration readback alone
therefore does not prove bandwidth enforcement.

Simple queues match in order and define upload/download relative to their
target. PCQ splits traffic by a classifier; a zero PCQ rate shares the available
bandwidth among its substreams. Selecting source/destination classification and
the target is essential to the intended fairness policy.

Application mapping:

- Keep Protection, Fairshare QoS and Hotspot speed profiles as distinct tasks.
  `network_plan.go` already prevents the generic queue editor from modifying
  Fairshare-owned policies.
- Preserve the upload-first RouterOS rate string in `guest_speed.go`, the
  ownership marker, and exact-profile adoption checks. It is not an instruction
  to introduce a second competing queue for each guest.
- Before extending shaping, inspect enabled FastTrack rules, queue ownership,
  order and traffic paths. Do not disable FastTrack automatically. A conflict
  needs an explicit reviewed change and a performance/guest-traffic test.
- Keep management traffic reachable when planning protection rules. Queue or
  firewall field readback, internet reachability and measured shaping throughput
  remain separate results.

## Checklist handoff

- Login/server hardening, CodeQL/lint and earlier tests: retained as completed
  work from Claude's checklist; not reopened by this study.
- CSP: changes are already in commit `3f49e59`. The continuation built the React
  app and loaded landing, sign-in, pricing, portal preview and the signed-out
  console under the repository's CSP with no observed policy violations or
  JavaScript exceptions. The local browser fixture returned unauthenticated API
  responses; this was not a signed-in production or router test.
- Bookmarked RouterOS study: the four sections and relevant linked pages above
  have been reviewed and mapped to the existing source. This does not certify
  every feature in the manual or add a new feature backlog.
- Merge/deployment still follows the owner's explicit merge/deploy instruction.
  Physical-router tests and the firmware-upgrade switch remain under their
  existing gates. No router, live flag, account or production data changed here.
