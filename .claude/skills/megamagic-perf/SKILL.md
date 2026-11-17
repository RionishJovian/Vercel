---
name: megamagic-perf
description: ALWAYS invoke when optimizing Mega Magic queries, pagination, exports, caching, bundle size, or low-bandwidth guest portal loading.
---

# Mega Magic performance

- Financial writes (approvals, sales, refunds) happen in one database transaction.
- Lists are bounded and paginated; keep existing caps (for example the revenue CSV export limit). Look for N+1 queries when filtering by site, date or payment method, and check the index that backs the query.
- Guest portals load on weak mobile networks: keep the first payload small, bundle fonts locally, and respect the router login page size budget (30,000 bytes login, 16,000 status, pair under 45 KiB).
- Jellyfin catalogue data stays metadata only: no media files, stream URLs or server credentials in the portal database.
- Verify with `npm run build`, `npm run test:critical-ui`, `go test -race ./internal/...`. Measure before claiming a speed-up.
