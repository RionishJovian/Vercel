# Local frontend synchronized to VPS — 29 October 2026

The live VPS frontend now matches all 55 production files in the local `web/`
directory, byte for byte. This deploys the consolidated topology continuity
changes and local homepage. No API or database deployment was performed.

## Verified identity

- Web image: `mega-magic-web:frontend-sync-20261029-1`.
- Image ID:
  `sha256:9e8a790dd05149c8067f168b454237a9f559694804b33e73a11004ec875e70e4`.
- Owner HTML SHA-256:
  `85cd0c492c0147d882549ee863351b37b65ea850e04ebc26916b09fea8be7312`.
- Homepage SHA-256:
  `c8d0a9ffc4260630602eff9b797d7c1348467e77c0fffa8e5a0032b34e18bd8d`.
- API remains
  `sha256:c078966d7dbe63ed6ffe2f9a1e32cc70b961f5354663e3f561e2eba70107f9e8`.

Every file was checked against the local manifest both inside the new container
and over HTTP, first on a private verification port and again after deployment.
The public HTTPS owner page also matches the local hash. Web health is healthy,
its restart count is zero, and API/database/Redis/NATS readiness is OK. The API,
PostgreSQL, Redis and NATS container identities were preserved.

The source comparison found two differing HTML files and one local-only module,
`voucher-workspace.js`. Unchanged public assets were reused only after their
hashes matched local. The new image contains exactly the local production
bundle; older public preview/prototype copies and unused wallpaper files are
excluded. Private `.local/` records and historical `src/` material were not
packaged. No database was copied or synchronized.

## Delivery and checks

HTML responses now require cache revalidation. A versioned entry URL was used
for browser verification. The existing security policy is retained; deferred
header assignment prevents duplicate headers from the two proxy layers.

Passed: production frontend build, six-surface static audit, all 107 critical UI
tests, focused device/topology and guest-device workflow tests, Caddy validation,
the 55-file container/HTTP comparisons and public HTTPS verification. Signed-in
browser checks rendered topology, voucher stock, the router guest preview and
the public homepage with its signed-in redirect, without browser console errors. No voucher, payment, saved
configuration or intentional router operation was performed. Physical router
connectivity and guest enforcement remain separately unverified.

The first live switch encountered inherited Compose labels from the committed
parent image, causing Compose to misidentify the private verification container.
Automatic rollback restored the original frontend. The final image clears those
historical labels, the private server uses a separate verification identity, and
that server is stopped before live Compose reconciliation. The final switch
passed; the temporary container is removed.

## Recovery and source

The persistent Compose file selects the new web image. Prior web image
`mega-magic-web:telemetry-20261028` and its verified ID
`sha256:0d2edb41667c3a89b16f2236c5b49342801d2c82eac52b0d3923e6478550e5d4`
are retained. Protected release context, manifests, configuration backups and
verification records are under `/root/mm-frontend-sync-20261029`; private local
evidence is under `.local/frontend-sync-20261029`.

A future manual frontend rollback should change only the web image and its
associated proxy settings after checking current state. Do not overwrite newer
API/configuration changes with an older whole-Compose backup. No image pruning,
GitHub push or separate Vercel deployment was performed.
