# Existing management connection during one-paste setup

29 October 2026. The platform administrator's `hAP ax2 lab` has an existing,
decryptable management credential record. The client account's
`Neighborhood RouterBOARD — Test` has no management credentials. This difference
in device state, rather than an administrator permission restriction, explains
their different one-paste setup outcomes.

The one-paste preflight intentionally refuses to replace an existing saved
management connection. Previously this refusal was collapsed into HTTP 503
`hub_management_preflight_failed`, displaying an incorrect server-readiness
error. It now returns HTTP 409 `hub_management_already_configured`, after the
same role, plan and tenant ownership checks. The preparation callback uses the
same classification if another request saves credentials after preflight.
Other preflight failures retain their existing unavailable response.

The frontend directs the user to **Finish connection**, which retains its
recent-handshake and authenticated management verification requirements. Saved
credentials alone do not establish present router connectivity. This change
does not add credential replacement, delete an enrollment or enable router
writes. Reinstalling a configured physical router requires a separately scoped
recovery workflow; onboarding must not silently overwrite its saved connection.

Verification:

- All 117 critical UI checks and the production frontend build passed.
- Affected Go handler and server packages passed with race detection.
- New handler regressions cover authorized roles, viewer denial, foreign-device
  refusal, wrapped error classification, zero key preparation/Hub allocation on
  preflight conflict, and a connection saved during preparation.
- An operator-only probe reproduced the old admin HTTP 503, then verified the
  corrected HTTP 409 against the live runtime database role. The client router
  passed preflight in both versions. Database transactions were read-only;
  all Hub operations were stubbed and no router connections were attempted.

The private probe, build logs and deployment/rollback records are retained in
`.local/hub-existing-connection-20261029` and the corresponding protected VPS
release folder. They are excluded from Git and frontend packaging. Deployment
hash verification and service readiness establish the served software release,
not physical RouterBOARD acceptance.

Deployed API and web images: `mega-magic-api:hub-existing-connection-20261029`
and `mega-magic-web:hub-existing-connection-20261029`. All 55 frontend files
matched the local release and HTTP responses, the changed frontend files
matched through public HTTPS, and the API reported ready. Credential metadata,
WireGuard peer allocations, configuration and data-service container identities
were preserved. Earlier images and Compose configuration remain available for
rollback in `/root/mm-hub-existing-connection-20261029`.
