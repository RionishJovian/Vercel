# Shared RouterBOARD context — 29 October 2026

The owner rejected repeating device selection inside each router action.
Magic Hub now opens Router overview, network editors and management-address
migration for the current router directly. A client with one router sees its
name once above the task categories, without a device dropdown. Legacy
multi-router workspaces have one remembered context selector above the settings.
Unrecognized or foreign remembered IDs do not choose an arbitrary router.

Each editor receives only the current target. Changing shared context replaces
that editor and invalidates its pending UI review; late reads cannot replace the
new target's view. Context and category changes are locked during network apply
and migration operations. Router names remain visible in consequential reviews,
with normal escaping. Existing roles, rollout holds, tenant checks and explicit
preview/consent/apply behavior are preserved.

The standalone shared network and migration editors also omit the device
selector when only one router exists, showing a read-only name instead.
Connection failures remain visible and do not become fabricated inventory.

Validation: all 117 critical UI checks passed, including seven new tests for
single-router auto-targeting across all Hub categories, one shared multi-router
context, invalid remembered IDs, stale responses, apply/migration locks and
read-only roles. Production frontend build, six-surface audit, device/topology,
guest-device and network-layout checks passed. No backend changes were needed.

The web-only release is `mega-magic-web:router-context-20261029`, image
`sha256:a22deefa9f5e6d27cdce19a0511b0e329981bead549b74b78ac349733a647485`.
All 55 production frontend files match the container and served HTTP bytes;
HTML and changed scripts also match through HTTPS. The API and data containers,
runtime configuration and security headers are unchanged. The prior web image
and Compose file are retained for rollback. Physical router configuration was
not modified or certified by this UI change.

Fresh authenticated live checks confirmed that Protection and management-address
migration open for the saved router with zero visible device selectors. The
web container was healthy with zero restarts; no temporary verification
container remained. A final mobile-emulation check stalled and is not counted
as a successful layout check. Its viewport override was cleared and its test
tab closed; user-owned tabs were preserved.
