# AP optimization provider integration — current implementation

This branch contains a controller-neutral proposal engine (`internal/apoptimization/policy.go`) and a **read-only** Omada Controller Open API inventory adapter (`internal/apoptimization/omada`). It now includes separately gated owner-only controller enrollment and browsing using encrypted client secrets (`ap_omada_connections`) with exact operator-approved HTTPS origins, CIDR-pinned outbound transport, TLS validation and no redirects. No live radio write endpoints are wired; both review and inventory are OFF by default.

## Verified TP-Link Open API
A controller administrator creates an app under **Global Settings → Platform Integration → Open API**, assigns the appropriate site and read-only role, and obtains the **Interface Access Address**, Omada ID, client ID and client secret. Official documentation shows client credentials authentication using `POST /openapi/authorize/token?grant_type=client_credentials`, JSON `omadacId`, `client_id`, `client_secret`, and `Authorization: AccessToken=...`. The documented site device listing is `GET /openapi/v1/{omadaId}/sites/{siteId}/devices?page=1&pageSize=100`. The adapter bounds responses to 512 KiB and at most 10 pages, validates device MACs, and never follows redirects in its default transport.

**Do not claim radio optimization support from the inventory listing.** Available Open API inventory fields do not establish RF interference readings, allowed country channels or a verified radio-setting endpoint on a specific Omada controller/firmware. `ReadAPRadios` intentionally fails closed until its exact route and response are confirmed with a device/controller under user control.

## Current capability matrix

| Vendor | Read-only discovered inventory | RF measurement and channel scoring | Live channel apply |
|---|---|---|---|
| TP-Link Omada Controller | Encrypted owner enrollment and server-scoped read-only device inventory implemented (hardware unverified) | Not yet verified | Disabled |
| Ruijie/Reyee | Not connected | Not verified | Disabled |
| Huawei | Not connected | Not verified | Disabled |
| iKuai | Not connected | Not verified | Disabled |

The independent policy core supports automatic and manual proposals **from fresh, verified provider measurements**. It rejects stale observations, illegal channels, expired proposals and unapproved changes. Its `ApplyApproved` contract is only for future adapters: wiring it requires tenant/owner authentication, durable one-time approval, revision fencing, and verified provider readback with rollback.

## Owner review ledger (separately gated)

The migration `db/migrations/20261108190000_ap_radio_owner_approval.sql` creates `ap_radio_proposals` with forced tenant RLS, an immutable reviewed proposal/digest, a five-minute expiry, owner decision identity, and a single-use `approved → consumed` state. The trigger forbids replay or changing a decided proposal into a new proposal.

`ap_optimization.enabled: false` is the default. If an operator explicitly enables the review ledger **after** migrating the database, only an authenticated tenant OWNER can view the review queue and approve/reject a matching proposal hash and radio fingerprint. The Managed Devices page includes a bilingual review-only panel. Neither admins nor the platform developer may approve on behalf of an SME owner.

There is deliberately no HTTP proposal-creation endpoint or AP Apply endpoint. The `RecordTrustedProposal` persistence function must be called by an independently verified model-specific AP observer; a browser must never supply a fake channel scan. `ConsumeOwnerApproval` atomically consumes an existing decision before any future provider write but is not connected to a vendor adapter yet. **Approving a proposal does not change any AP.**

## Next implementation gates
1. Configure `ap_optimization.enabled: true` and `omada_inventory_enabled: true` ONLY after applying both approval and Omada enrollment migrations, injecting a separate 32-byte `omada_credential_key`, and setting exact `omada_allowed_origins` plus narrow `omada_allowed_cidrs`. The backend refuses enabled startup without those protections. The owner can then add a controller with site-restricted Omada Open API client credentials and browse read-only inventory.
2. Perform read-only inventory on a real controller. Compare API output to the controller web UI; no router or radio changes. No live Omada credentials have been configured or tested in this draft branch.
3. Obtain model-specific channel/radio API documentation and controlled RF measurements. Return **unsupported**, not guessed values, when unavailable.
4. Persist reviewed proposals and one-use owner approval tokens, ensuring device- and tenant-bound expiry and replay protection.
5. Implement a reversible vendor adapter for one verified model; test read → preview → approve → apply → readback → rollback in staging.

## References
- https://support.omadanetworks.com/us/document/109315/
- https://use1-omada-northbound.tplinkcloud.com/doc.html
- https://github.com/dfla-me/omada-mcp/blob/main/docs/openapi/endpoint-map.md (independent map; verify all routes against controller's embedded API documentation)
