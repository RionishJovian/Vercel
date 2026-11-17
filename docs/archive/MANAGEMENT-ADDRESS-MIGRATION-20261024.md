# Guided Magic Hub management-address migration

Implemented as an additive extension to the existing app and Hub. This moves the router's owned IPv4 Magic Hub /32 address, not its guest LAN, WAN or DHCP subnet. The Hub selects the new address; the browser never accepts an arbitrary destination, credential or RouterOS command.

## User flow

Magic Hub → Management address → select router → check router and prepare review → review old/new addresses → explicitly consent → back up and migrate. The existing router record editor makes tunnel management IP read-only and links to this wizard. The API and database also reject direct rewrites and transport downgrades of configured Hub connections. Initial verified authentication remains supported.

A GET reads availability and the latest durable checkpoint. Preview reads the current Hub peer, requires a recent handshake, checks the saved SSH pin and WireGuard public key, validates ownership of the static address/interface, checks the outbound address policy, and stores a five-minute review. It does not reserve Hub addresses or change the router. A pending preview can be cancelled.

## Safety sequence

1. A session-level PostgreSQL lease and shared per-device network mutex exclude concurrent moves. Every API action reloads the tenant/device-bound journal and expected revision.
2. A durable `starting` checkpoint precedes remote side effects. The Hub reserves the selected address, retains the old /32 and adds the new /32 to the same peer. Both kernel conflicts and durable reservations are checked.
3. The router captures export/binary backups, retains the original address and adds an operation-tagged alias. A router-side three-minute timer removes only this operation's alias if confirmation is lost.
4. A connection to the new address must authenticate using the existing pinned SSH host key and report the same WireGuard public key and owned interface/address. No trust-on-first-use, key replacement, relaxed certificate verification or fallback enrollment is used.
5. The Hub durably commits the new allocation while retaining both paths. This dual routing survives Hub restart and does not expire after commitment.
6. The router timer is cancelled and cancellation/new-address readback are verified before saving the connection.
7. The management address, `connection_saved` checkpoint and one device-change audit event commit in one PostgreSQL transaction, with expected old address, SSH fingerprint, phase and revision. An uncertain commit result is resolved by reloading the journal, never by assuming it failed.
8. The new path is verified again. The exact original address record is removed through the new path; unrelated records are preserved. The new address receives the normal enrollment ownership marker so subsequent bootstrap and migrations remain compatible.
9. The Hub removes the old allocation only after router cleanup. The journal reaches `completed` after Hub readback verification.

A failure before the saved connection switches leaves the old path in place. Recovery verifies that path, restores old-only Hub routing and removes only this operation's alias. A failure after the atomic switch recovers forward through the new path and finishes cleanup. It never blindly restores the old database target after a lost commit response. If identity, ownership or reachability cannot be verified, the job remains pending for inspection.

## Implementation

- `internal/managementmigration`: durable-phase coordinator and fault-injection tests.
- `internal/database/management_migration.go`: tenant transactions, revision CAS, atomic target/audit update and PostgreSQL advisory lease. Lease release failure discards the session rather than returning a locked connection to the pool.
- `migrations/20261024070000_management_address_migration.sql`: additive journal, one active move per device, composite tenant foreign keys, forced RLS and configured-tunnel write guards. Active jobs block deletion of the device; completed/cancelled history follows the existing confirmed device-record deletion workflow.
- `internal/provisioner/routeros/management_migration.go`: frozen pinned target, outbound policy, dual-address staging, timer, identity readback and exact cleanup.
- `internal/hub/management_migration.go`: signed `peers:migrate` protocol and response identity/address binding.
- `internal/handler/management_migration.go`: authenticated administrator routes shared by production and tests. Individual-pass and tenant middleware remain in the existing parent group.
- `scripts/hub-management-migration.mjs`: synchronous single-writer Hub state machine, durable allocation reservations, expiry/restart reconciliation.
- `scripts/stage-hub-management-migration.mjs`: produces a candidate from the exact inspected Hub baseline. Never edits or restarts a live service.
- `web/assets/js/management-migration.js`: status/review/apply/recovery wizard. It disables duplicate actions, reloads durable state after uncertain responses, escapes returned values and ignores detached reads.

## Hub source provenance

The existing authorized SSH key was matched read-only against the VPS's authorized public-key fingerprint. No key or permission was added. The running service `/opt/mikromagic-hub/service.mjs` on `srv2032937` was copied read-only and verified as SHA256:

`760beadcd1cba2ceb95741aa8b60ec5ed82ee8adb9059332121a53dc06adb246`

Staging rejects source drift. The generated service retains the existing HMAC authorization, readonly lookup and create-only enrollment. It blocks legacy peer mutations during an active move, reserves both addresses, verifies kernel ownership before synchronization, and reconciles migration records on startup and every 30 seconds. Private signing values and peer-store contents were not copied into the repository.

## Validation

- 94 critical UI checks passed, no skips or failures.
- Eight Hub state-machine tests and the signed real staged-service route integration passed using in-memory filesystem/WireGuard dependencies; no network listener or real WireGuard mutation.
- Affected backend packages (`managementmigration`, `hub`, `routeros`, `handler`, `server`) passed under the race detector with disposable PostgreSQL integration enabled.
- Real PostgreSQL tests cover atomic target/checkpoint/audit, stale/replayed switches, forced tenant isolation, composite actor ownership, one active job, concurrent leases, direct address/transport rejection, first-authentication compatibility and disabled-trigger schema rejection.
- Stateful RouterOS command-runner tests cover dual addresses, timer cancellation, cleanup retry, preservation of foreign addresses, wrong identity, changed ownership, reused record IDs and outbound-policy rejection. They simulate commands, not RouterOS execution.
- Production Go build and JavaScript syntax checks passed. HTTP tests use real signed JWT middleware and the shared production role/route registration; denied requests do not invoke the controller.

## Activation and rollback boundary

Not deployed or enabled by this implementation. Application and Hub flags are separate: `MM_MANAGEMENT_MIGRATION_ENABLED=true` and `MM_HUB_MANAGEMENT_MIGRATION_ENABLED=true`. Enabling the app requires the additive schema and its startup verification. Missing/older Hub protocol fails before router changes; no reenrollment fallback exists.

Release order: verify current source/image identities; back up the database, Hub service/module and peer store, Compose/config and app/web images; apply the additive migration; stage and test the exact Hub source; deploy the compatible Hub/module and API/web artifacts; preserve existing ownership/credentials/flags; verify readiness, public assets and signed readonly availability before enabling the capability. Existing individual-pass deadlines must not be changed.

Do not restore the old Hub implementation while any Hub migration is staged/committed or any app job is nonterminal. First recover/finish those jobs with the new coordinator. Rolling back the app while retaining the compatible Hub extension and additive journal preserves the known paths; do not restore an old database dump over newer customer history. A committed Hub move intentionally retains both routes until cleanup, so disabling the UI alone is not cleanup.

## CHR and hardware status

Read-only checks found no registered Lima VM, CHR disk or installed QEMU executable in the inspected Mac locations; only historical UTM data was present. Docker Desktop's socket was absent. The current VPS had the five app containers and no discovered QEMU executable/CHR image in the inspected locations. No VM, customer router or Hub peer was modified.

Actual RouterOS/CHR execution, kernel WireGuard migration, power-loss behavior and physical RouterBOARD acceptance remain unverified. The opt-in certification must use an explicitly disposable isolated CHR and prove both successful migration and forced failures at each boundary before claiming hardware certification. Existing physical-router/phone voucher acceptance remains a separate pending task.

Official references: [multiple IP addresses per interface](https://help.mikrotik.com/docs/spaces/ROS/pages/328247/IP+Addressing), [WireGuard allowed-address routing and uniqueness](https://help.mikrotik.com/docs/spaces/ROS/pages/69664792/WireGuard), [CHR virtualization requirements](https://help.mikrotik.com/docs/spaces/ROS/pages/18350234/Cloud+Hosted+Router+CHR).
