# Network controls — 2026-10-03

## Scope and rollout

Dedicated navigation exposes WAN and failover, PPPoE subscribers, Wi-Fi networks,
and existing-network management. Gateways remains registration, Hub connection
and diagnostics. These are typed operations, not a free-form RouterOS terminal.

Code `7f432ad` is implemented, locally/lab tested and deployed to mikromagic.com
with `./deploy.sh all`. Landing, app and health returned 200; anonymous inventory
returned 401. The signed-in live workspaces loaded actual CHR interfaces, checked
PPPoE address suggestions and existing-service task choices. The live Wi-Fi page
displayed its physical-radio acceptance hold with no SSID form. No live router
preview/apply was performed. Protected API/web backup:
`/home/ubuntu/mm-network-rollout.n96C5f`. No migration is needed. Production controls require
`MM_NETWORK_CONTROLS_ENABLED=true`; absent/false holds preview and apply before
any target lookup. Wi-Fi additionally requires
`MM_NETWORK_WIFI_CONTROLS_ENABLED=true`, which must stay false until approved
physical-radio acceptance. Inventory reports both capabilities and the browser
does not offer a held write form. Owner/admin routing and tenant-scoped target
lookup remain mandatory; a feature flag does not grant a role or device access.
The production drop-in enables network controls and explicitly holds Wi-Fi
writes false. This is a partial tested rollout, not whole-app certification.

## Implemented operations

- IPv4 two-WAN recursive-probe failover or equal-cost balancing in the main
  routing table. Explicit port/gateway selection; detected DHCP gateways prefill
  on selection. Overlapping ISP subnets, competing automatic uplinks and probe
  conflicts fail closed. Existing static defaults are retained but disabled.
  Managed routes/NAT are updated by exact owned selectors; active management
  endpoints receive primary/backup return routes outside ECMP.
- PPPoE ISP clients; dedicated subscriber services with checked private address
  pools; subscriber accounts, service state/default plan, and profile speeds,
  DNS/address allocation and session limit.
- Existing DHCP services, shrinking address pools, client gateway/DNS, Hotspot
  service/profile settings, static reservations and simple bandwidth queues.
- Modern/legacy local-radio SSID/WPA2 editors, held for hardware acceptance.
  A radio-free CHR is not an AP; choose the actual registered radio device.

Existing address and non-default route prefixes and even unrouted IP pools
reserve suggested private networks. Port roles are never guessed. The initial
LAN/DHCP/Hotspot workflow now also verifies scalar fields rather than RouterOS
human-oriented print output, which prints comments separately and some menus
as columns. Duration readback accepts canonical HH:MM:SS/week/day values;
lease input is strictly parsed and bounded to one minute through seven days.

## Review, backup and recovery

Review is read-only and returns a five-minute, random, single-use confirmation
bound to business, device, exact inputs and the before-state. Changing a field,
device or task invalidates the browser review; late responses are discarded.
Apply rechecks state under the shared per-device lock, creates an export and
binary backup, then installs a three-minute router-side reverse-operation timer.
The timer survives the initiating SSH session. WAN steps run as a coherent
router block to avoid losing the return route between separate SSH writes.

Every changed field is read back. WAN probes use one labelled scalar because
RouterOS still emits its ping table inside a scripting expression. Both ISP
probe destinations must reply before the guard is cancelled. Cancellation is
verified. Errors attempt reverse-order restoration and report whether it was
verified, rather than reporting success from a transport timeout. An uncertain
timer blocks subsequent changes; active router jobs also block writes after an
API restart. Initial LAN setup respects this same cooldown/job boundary.

Secrets are blank in forms, excluded from inventory, reviews and browser
storage, and not logged. Approval storage contains hashes, not command strings
or credentials. Audit results distinguish verified router changes from an
unrecorded audit row. Router backups remain on the device, not in browser drafts.

## Acceptance evidence

Local: full Go suite, focused router/handler/server race tests, Go vet, six-page
UI audit, network-workspace regressions, and focused synthetic Chromium tests
passed. Browser coverage includes four workspaces, detected values, stale review
protection, explicit consent, single verified apply, no stored passwords and
390px layout. Database integration is not newly certified by these runs.

Real RouterOS 7.15.3: two newly created, loopback-only, disposable KVM CHRs on
the lab host, not the registered production CHR or any customer router:

- PPPoE server/subscriber and queue configuration with verified field readback.
- Failover policy, simulated primary probe-route loss, backup default activation,
  internet replies through backup and primary recovery.
- Equal-cost policy: both WAN defaults active. This is not a throughput benchmark
  or proof that one download combines the two ISP capacities.
- Isolated LAN/DHCP/Hotspot setup and live-record edits to lease duration, pool,
  DHCP DNS/gateway, Hotspot profile and subscriber-plan speed.
- A separate client authenticated to the app-created PPPoE account; the server
  confirmed its session and internet probes replied through the subscriber link.
- A real DHCP client received the app-created static reservation, 192.168.75.99.
- A router-side background job survived SSH disconnect. Earlier failure trials
  confirmed immediate rollback and independent-timer restoration; the invalid
  overlapping-uplink fixture was replaced and such inputs now reject in preview.

Opt-in tests: `TestNetworkCHRIsolatedAcceptance` and
`TestNetworkCHRPeerAuthenticationAndDHCP`. They require exact disposable-launch
confirmations, loopback ports 3222/4322 and separate pinned SSH identities.
The prepared server has independent NAT uplinks 10.0.2.0/24 and 198.18.2.0/24;
its ether3/ether4 connect to the client's ether2/ether3 over private QEMU socket
Ethernet. The test-account password is deliberately disposable, not a customer
credential. No production configuration, account, router or public test port
was changed by these acceptance runs.
After acceptance, the two disposable guests were stopped, not deleted; their
containers and disks remain recoverable. Start both before rerunning peer tests.

## Remaining boundaries

Physical Wi-Fi (modern/legacy packages, country restrictions, profile
inheritance, reconnect and rollback), CAPsMAN-managed radios, hardware link-cut
and cold-restart WAN trials, IPv6 routing and shaping throughput are not
certified. This slice uses explicitly reviewed IPv4 ISP gateways: DHCP gateway
changes require re-review; it does not install a DHCP lease-script updater.
Two PPPoE uplinks are not accepted as the static two-WAN topology when their
peer gateways do not belong to discovered interface subnets. ISP authentication
against a customer's provider is separate from the successful lab account.

Hotspot login/voucher/payment and actual guest browsing are separate acceptance
boundaries; DHCP/Hotspot settings readback does not certify them. Pool expansion,
arbitrary bridge/VLAN restructuring, RADIUS provisioning and cloud WebFig proxy
are not offered by these editors. Do not call this the whole app completed.
