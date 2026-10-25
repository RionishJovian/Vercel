# Magic Hub RouterBOARD management — software candidate

Owner direction: build task-based remote RouterBOARD controls inside the existing Magic Hub page, preserving Mega Magic's silver/emerald Jade Glass theme. Router availability is not a prerequisite for implementation. Local test results and real-device certification remain separate.

## Implemented scope

Magic Hub now contains nine task groups: overview, Internet connections, IP/DHCP, HotSpot/LAN, protection, Fairshare QoS, onboard Wi-Fi, subscriber service, and maintenance. Existing WAN/PPPoE, DHCP, HotSpot, Wi-Fi, backups, portal, voucher and guest workflows are reused rather than replaced. The task panel renders without probing the router; reads start when a task is selected. Inner editors preserve the selected router. An outer category cannot discard a pending apply.

Overview uses a dedicated tenant-bound `GET /api/v1/routers/:id/network/overview` action with two bounded SSH reads: interfaces and a fixed system-resource projection. It does not load subscriber credentials or the full configuration inventory. Missing data stays unavailable, not fabricated. Existing authenticated router routes, administrator requirement, tenant lookup and individual-pass gates apply unchanged.

Protection has an explicit scope: create/enable/disable one Mega Magic-owned invalid IPv4 input drop for a selected port. No global firewall-off operation is exposed. Installation verifies the rule's content and first position. Existing policies are preserved. An altered or colliding owned marker is rejected. Asymmetric routing risk is described in review.

Portal bypass shield lists HotSpot IP-binding exemptions and can disable one explicitly selected active unmanaged bypass. It cannot create or re-enable bypasses or change `mm-` managed exemptions. The review binds the client's MAC, address, server and comment to the confirmation so changing the target after preview invalidates the operation. Existing authenticated sessions can require reauthentication before the effect is visible. This is not a claim to eliminate MAC spoofing or every form of portal bypass.

Fairshare QoS creates two dedicated PCQ types (upload `src-address`, download `dst-address`, zero PCQ rate for equal sharing) and an owned simple queue with total upload/download capacity. The exact connected IPv4 customer subnet is required. Hub addresses, broad/malformed targets, active FastTrack, conflicting/unreadable queue targets and reserved-name collisions are rejected. Changes to existing policies validate their identity, scope and both classifiers; the generic bandwidth editor cannot bypass those ownership checks. Sharing is per client IP, not necessarily per person.

IP/DHCP offers current IP and route inspection plus the existing DHCP pool/network/reservation editors, new customer network workflow and router-name/DNS Quick Config. Arbitrary replacement of management IPs and arbitrary route edits are not offered. Maintenance links existing backups/restore/events/fleet workflows and retains the verified-connection requirement for reboot. Automated RouterOS/firmware updates are not implemented by this candidate.

## Guarded writes

New typed operations use the existing preview/confirmation/apply engine, tenant/device-bound one-use five-minute tokens, stale-state rejection, per-device lock, pre-change export and binary backup, router-side restoration timer, configuration readback and rollback reporting. Reviews now show field-level before/after details; credential values remain hidden.

The existing `MM_NETWORK_CONTROLS_ENABLED` and separate Wi-Fi rollout switches are unchanged. This candidate does not enable network writes, change a physical router or claim connectivity. The default held state continues to show actual read-only settings. No migration, pass approval, payment or provider credential is required.

## Documentation basis and transport choice

- [Official REST API](https://help.mikrotik.com/docs/spaces/ROS/pages/47579162/REST+API): HTTPS `www-ssl`, Basic Auth, string-valued results, GET/PATCH/PUT/DELETE semantics.
- [Official MikroTik community REST discussion](https://forum.mikrotik.com/t/how-can-i-use-rest-api-to-run-a-script/174279/7): REST is served by www/www-ssl, not the binary API service port.
- [Official PCQ example](https://help.mikrotik.com/docs/spaces/ROS/pages/137986099/PCQ+example): separate upload/download classifiers and simple-queue attachment.
- [Official connection tracking](https://help.mikrotik.com/docs/spaces/ROS/pages/130220087/Connection+tracking): FastTrack bypasses simple queues; invalid-state dropping needs care with asymmetric routing.
- [Official HotSpot documentation](https://help.mikrotik.com/docs/spaces/ROS/pages/56459266/HotSpot+-+Captive+portal): bypassed IP bindings exempt clients from login; walled garden is a separate mechanism.

The implemented engine continues to use the existing saved, tenant-bound SSH transport through Magic Hub. This is not a REST transport rollout. Enabling www-ssl, provisioning trusted certificates and least-privilege REST users, and storing a new transport binding would be a separate explicit connection capability; the frontend must not enable public WebFig or bypass certificate validation just to obtain REST.

## Verification

- 89 critical UI tests passed, zero failures/skips (suite run with localhost listener permission).
- Seven new DOM tests cover immediate rendering, typed requests, explicit review, read-only IP data, escaping, offline retry, late-response isolation, apply-navigation lock and rollout holds.
- `go test -race ./internal/provisioner/routeros ./internal/handler ./internal/server -count=1` passed. Integration tests requiring an external disposable PostgreSQL URL were not exercised in this run; no database code or route authorization policy changed.
- Stateful command-runner tests cover first-position firewall protection, foreign-rule preservation, altered-marker rejection, bypass target drift, managed exemption rejection, PCQ creation/edit, conflicts, unsafe targets and rollback after a write commits but the transport reports failure. These are simulated command executions, not RouterOS hardware execution.
- Production Go API builds; production inline JavaScript and new assets parse; diff whitespace check passes.
- Local browser preview is explicitly sample data; phone-width Day/Night styling inspected. It is not connected to a customer router or production account.

## Release status

Implemented and locally verified. Not deployed by this change. Preserve the existing live individual-pass release and its deadlines. A deployment must verify the current running image and asset identities, back up Compose/images/assets, build an immutable compatible API/web pair, preserve configuration ownership and all current flags, and verify readiness/public asset hashes. No database restore is needed for rollback. Physical internet, first-use, QoS throughput, bypass enforcement, expiry and reconnect certification remain pending router availability.
