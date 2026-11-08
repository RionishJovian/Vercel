# AP optimization provider integration — current implementation

This branch contains a controller-neutral proposal engine (`internal/apoptimization/policy.go`) and a **read-only** Omada Controller Open API inventory adapter (`internal/apoptimization/omada`). No live radio write endpoints, controller credentials, or production flags are wired.

## Verified TP-Link Open API
A controller administrator creates an app under **Global Settings → Platform Integration → Open API**, assigns the appropriate site and read-only role, and obtains the **Interface Access Address**, Omada ID, client ID and client secret. Official documentation shows client credentials authentication using `POST /openapi/authorize/token?grant_type=client_credentials`, JSON `omadacId`, `client_id`, `client_secret`, and `Authorization: AccessToken=...`. The documented site device listing is `GET /openapi/v1/{omadaId}/sites/{siteId}/devices?page=1&pageSize=100`. The adapter bounds responses to 512 KiB and at most 10 pages, validates device MACs, and never follows redirects in its default transport.

**Do not claim radio optimization support from the inventory listing.** Available Open API inventory fields do not establish RF interference readings, allowed country channels or a verified radio-setting endpoint on a specific Omada controller/firmware. `ReadAPRadios` intentionally fails closed until its exact route and response are confirmed with a device/controller under user control.

## Current capability matrix

| Vendor | Read-only discovered inventory | RF measurement and channel scoring | Live channel apply |
|---|---|---|---|
| TP-Link Omada Controller | Adapter implemented; credential enrollment and in-app route not yet wired | Not yet verified | Disabled |
| Ruijie/Reyee | Not connected | Not verified | Disabled |
| Huawei | Not connected | Not verified | Disabled |
| iKuai | Not connected | Not verified | Disabled |

The independent policy core supports automatic and manual proposals **from fresh, verified provider measurements**. It rejects stale observations, illegal channels, expired proposals and unapproved changes. Its `ApplyApproved` contract is only for future adapters: wiring it requires tenant/owner authentication, durable one-time approval, revision fencing, and verified provider readback with rollback.

## Next implementation gates
1. Enroll controller and encrypted credentials per tenant, enforce host/IP egress allowlist, TLS trust and per-site scoping.
2. Perform read-only inventory on a real controller. Compare API output to the controller web UI; no router or radio changes.
3. Obtain model-specific channel/radio API documentation and controlled RF measurements. Return **unsupported**, not guessed values, when unavailable.
4. Persist reviewed proposals and one-use owner approval tokens, ensuring device- and tenant-bound expiry and replay protection.
5. Implement a reversible vendor adapter for one verified model; test read → preview → approve → apply → readback → rollback in staging.

## References
- https://support.omadanetworks.com/us/document/109315/
- https://use1-omada-northbound.tplinkcloud.com/doc.html
- https://github.com/dfla-me/omada-mcp/blob/main/docs/openapi/endpoint-map.md (independent map; verify all routes against controller's embedded API documentation)
