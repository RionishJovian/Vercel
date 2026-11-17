# Device deletion blocked by retained guest history

29 October 2026. A read-only production check found that the platform admin's
`hAP ax2 lab` has 11 vouchers, 11 guest-voucher grants and one issuance batch.
The grant and batch foreign keys intentionally prevent permanent deletion of
their parent device. There is no pending management-address migration on this
device. The observed DELETE request reached the API and returned HTTP 500;
this was not a disabled confirmation button or an account permission denial.

Two error-reporting defects are corrected:

- The router deletion handler maps a linked-record foreign-key conflict to HTTP
  409 with a message explaining that retained voucher/recovery history prevents
  deletion. Database constraint names and internal errors remain private. Other
  failures remain HTTP 500, and the existing migration-recovery conflict remains
  HTTP 409.
- The global removal helper previously referenced `showErr`, which exists only
  inside the screen renderer. That scope error hid the API failure. The renderer
  now passes its error callback explicitly. An accessible alert beside the
  selected device displays the failure and scrolls into view. The record stays
  visible and its button becomes usable again after a failed request. Successful
  deletion followed by a failed refresh still asks for reload rather than
  offering a second deletion.

The explicit permanent-deletion confirmation, viewer restrictions, tenant
isolation and existing foreign-key guards remain. No cascade deletion of guest
history or credential reset was added. Retiring a device while retaining its
history needs a separate archive workflow; permanent deletion is currently the
only device-removal operation.

Verification:

- All 118 critical UI checks and the production frontend build passed.
- Disposable PostgreSQL tests with race detection verified successful deletion
  of an unlinked device and HTTP 409 for grant, issuance-batch and recovery
  links. Failed deletion preserved device identity, voucher ownership,
  allowance snapshots and linked history. Foreign-tenant and viewer requests
  were denied without disclosing linked history.
- The UI regression runs without a global `showErr`, verifies the injected
  callback, confirms escaped inline conflict text, and retains confirmation,
  cancellation, duplicate-request and detached-view guards.

Private test/build/deployment evidence is excluded from Git and retained in
`.local/device-delete-20261029` and the corresponding protected VPS release
folder. Production diagnosis used read-only queries. No production DELETE was
issued by the agent.

Deployed images: `mega-magic-api:device-delete-20261029` and
`mega-magic-web:device-delete-20261029`. All 55 frontend files matched the
release source and HTTP responses, the changed app document matched through
public HTTPS, and the API reported ready. Device and linked-history counts,
management credentials, WireGuard allocations, configuration and data-service
container identities were preserved. Prior images, binary and Compose
configuration remain available for rollback in `/root/mm-device-delete-20261029`.
