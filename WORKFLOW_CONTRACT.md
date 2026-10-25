# Mega Magic — Workflow / Capability Contract

**Generated from live audit (commit 8b50ebb) + reconciled source**
**Date: 2026-10-23**

---

## Legend

| Symbol | Meaning |
|--------|---------|
| ✅ | Implemented, tested, backend route exists |
| ⚠️ | Backend route exists but readiness gate may block (feature flag, config, missing connector) |
| 🔴 | Frontend calls route that **does not exist** in backend (contract drift) |
| 🟡 | Legacy/old commerce path — retained for history, not current authority |
| ❓ | Unknown / not audited |

---

## Role Hierarchy (source: `internal/model/user.go`)

```
DEVELOPER (100)  — system only, never grantable
OWNER     (60)   — business owner, full access
ADMIN     (50)   — staff with admin permissions
VIEWER    (10)   — read-only tenant member
```

**Tenant roles**: OWNER, ADMIN, VIEWER (grantable by OWNER)
**System role**: DEVELOPER (platform admin only)

---

## Feature Gates

| Gate | Source | Controls |
|------|--------|----------|
| `subscriptionOnly` (config) | `SubscriptionAccess` middleware | Bypasses subscription check when `false` |
| `SubscriptionAccess` middleware | `internal/handler/subscription_handler.go` | Enforces `subscription.Status.Access` on all protected routes |
| `RecoveryRoute` allowlist | `internal/subscription/policy.go:30-37` | Allows `/me/*`, `/subscription`, `/store/*`, `/pricing` even when expired |
| `provisioner.inventory_enabled` | config | Gates read-only inventory; never enables applier/writes |
| `Media.ConnectorEnabled` | `internal/server/media_routes.go` | Gates Jellyfin publication & playback routes |
| `publishing.PurchasesEnabled` | `internal/server/media_routes.go` | Hardcoded `false` — blocks paid media sales |

---

## Navigation → Renderer → API Contract

### 1. Overview (`data-view="overview"`)
| Aspect | Detail |
|--------|--------|
| **Renderer** | `renderOverview()` |
| **API** | `/api/v1/me`, `/api/v1/subscription`, `/api/v1/fleet/health` (aggregate) |
| **Auth** | `VIEWER+` + `SubscriptionAccess` (recovery allowed) |
| **DB Authority** | `users`, `businesses`, `devices` (peer keys), `telemetry_samples` |
| **Readiness** | ✅ Always loads; fleet health shows `UNKNOWN` when no telemetry |

---

### 2. Revenue Dashboard (`data-view="revenue"`)
| Aspect | Detail |
|--------|--------|
| **Renderer** | `renderRevenue()` |
| **API** | `/api/v1/gw/revenue`, `/api/v1/voucher-report`, `/api/v1/billing/requests` |
| **Auth** | `VIEWER+` + `SubscriptionAccess` |
| **DB Authority** | `guest_purchase_requests`, `vouchers` (guest_voucher_grants), `store_purchases` (legacy) |
| **Readiness** | ⚠️ **Mixes legacy Store approval inbox** (audit #10) — legacy passes excluded from hotspot totals but remain actionable in payment inbox |
| **Known Issue** | Legacy business-pass decisions mixed with guest revenue workflow |

---

### 3. Internet Voucher Dashboard (`data-view="vouchers"`)
| Aspect | Detail |
|--------|--------|
| **Renderer** | `renderVouchers()` |
| **API** | `/api/v1/guest-vouchers/issue`, `/api/v1/guest-vouchers/batches`, `/api/v1/vouchers` (list), `/api/v1/guest-vouchers/sync` |
| **Auth** | `ADMIN+` for issue/sync; `VIEWER+` for list |
| **DB Authority** | `vouchers` (guest path uses `guest_voucher_grants`), `guest_voucher_issue_batches` |
| **Readiness** | ⚠️ **Per-code lifecycle hidden** (audit #12) — main table shows only first 50 rows, no first-use/expiry columns |
| **Known Issue** | Two voucher families share one table (audit #14): old generic (sovereign/Hub expiry) vs guest internet (grants, first-use, router install) |

---

### 4. Jellyfin Dashboard (`data-view="media"`)
| Aspect | Detail |
|--------|--------|
| **Renderer** | `renderJellyfinPortal()` (also used for Jellyfin Guest Portal) |
| **API** | `/api/v1/media/pair`, `/api/v1/media/connection`, `/api/v1/media/publication/available`, `/api/v1/media/publication`, `/api/v1/media/publication/publish` |
| **Auth** | `ADMIN+` for pair/publish; `VIEWER+` for status |
| **Feature Gate** | `Media.ConnectorEnabled` + `publishing.PurchasesEnabled=false` (hardcoded) |
| **DB Authority** | `media_connections`, `magic_media_publications`, `media_passes` |
| **Readiness** | 🔴 **Publication returns 404** (audit #7) — connector not enabled in live deployment |
| **Known Issue** | Sales UI (pricing, duration, MMQR) exposed but `PlaybackReady` always `false` (audit #8) |

---

### 5. Magic Hub (`data-view="gateways"`)
| Aspect | Detail |
|--------|--------|
| **Renderer** | Part of `renderSites()` + Hub cards |
| **API** | `/api/v1/vps/peers`, `/api/v1/vps/peers/:id/site`, `/api/v1/vps/peers/:id/enroll`, `/api/v1/vps/hub-config` |
| **Auth** | `ADMIN+` for enrollment; `VIEWER+` for list |
| **DB Authority** | `vps_peers`, `devices` (site_id), `router_connections` |
| **Readiness** | ⚠️ **UNKNOWN state** (audit #8) — no live enrollment/test run; capability probes deferred |

---

### 6. Devices (`data-view="routers"`)
| Aspect | Detail |
|--------|--------|
| **Renderer** | `renderObservedWorkspace()` → `MMRevamp.mountObserved()` (kind="devices") |
| **API** | `/api/v1/routers` (list), `/api/v1/routers/:id/observations` **🔴 MISSING**, `/api/v1/routers/:id/discovery-labels` **🔴 MISSING**, `/api/v1/routers/:id/discovery-labels` PUT **🔴 MISSING** |
| **Auth** | `VIEWER+` for list/observations; `ADMIN+` for discovery-labels PUT |
| **DB Authority** | `devices`, `device_discovery_labels` (new table) |
| **Readiness** | 🔴 **Live 404 on `/observations`** (audit #5) — `workflow-revamp.js` calls endpoints not in prior backend |
| **Fixed in Reconciliation** | Added 4 endpoints: `GetRouterObservations`, `GetDiscoveryLabels`, `UpsertDiscoveryLabel`, `DisconnectGuest` |

---

### 7. Site Topology (`data-view="topology"`)
| Aspect | Detail |
|--------|--------|
| **Renderer** | `renderTopology()` |
| **API** | `/api/v1/topology/map`, `/api/v1/topology/:deviceId/reboot` |
| **Auth** | `VIEWER+` for map; `ADMIN+` for reboot |
| **DB Authority** | `devices` (parent_gateway_id, deployment_layer), `sites` |
| **Readiness** | ✅ Loads; explicitly **not discovered physical cabling** (audit #17) |

---

### 8. Guests (`data-view="guests"`)
| Aspect | Detail |
|--------|--------|
| **Renderer** | `renderObservedWorkspace()` → `MMRevamp.mountGuests()` |
| **API** | `/api/v1/routers/:id/observations` **🔴 MISSING**, `/api/v1/routers/:id/guests/disconnect` **🔴 MISSING** |
| **Auth** | `VIEWER+` for sessions; `ADMIN+` for disconnect |
| **DB Authority** | None directly — reads from router via connector (hotspot_sessions inventory) |
| **Readiness** | 🔴 **0/1 gateway sources available** (audit #6) — correctly shows unavailable, not zero |
| **Fixed in Reconciliation** | Same 4 endpoints as Devices |

---

### 9. Fleet Health (`data-view="fleet"`)
| Aspect | Detail |
|--------|--------|
| **Renderer** | `renderFleet()` |
| **API** | `/api/v1/fleet/health`, `/api/v1/fleet/health/offline`, `/api/v1/peers/:peerId/state` |
| **Auth** | `VIEWER+` |
| **DB Authority** | `devices`, `peer_heartbeats`, `telemetry_samples` |
| **Readiness** | ✅ Real stored gateway reads; current state `UNKNOWN` (audit #16) |

---

### 10. Internet Setup (`data-view="internet"`)
| Aspect | Detail |
|--------|--------|
| **Renderer** | `renderInternetWorkspace()` + task renderers (WAN, LAN, WiFi, PPPoE) |
| **API** | `/api/v1/routers/:id/quick-config`, `/api/v1/routers/:id/network/:action`, `/api/v1/routers/:id/gateway/:action` |
| **Auth** | `ADMIN+` (all write) |
| **DB Authority** | `devices`, `router_config_backups` (implicit) |
| **Readiness** | ⚠️ **Task navigation loads; changes not executed** (audit #10) — requires live router |

---

### 11. Internet Hotspot Portal (`data-view="portal"`)
| Aspect | Detail |
|--------|--------|
| **Renderer** | `renderPortal()` |
| **API** | `/api/v1/sites/:id/portal` (GET/PUT), `/api/v1/sites/:id/portal/publish`, `/api/v1/portal/staff/config`, `/api/v1/portal/staff/router-preview` |
| **Auth** | `ADMIN+` |
| **DB Authority** | `sites` (portal_config), `portal_config` (business-level fallback) |
| **Readiness** | ⚠️ **Authority split** (audit #13) — business `/portal/config` vs site `/sites/:id/portal/*` can diverge |

---

### 12. Jellyfin Guest Portal (`data-view="jellyfin-portal"`)
| Aspect | Detail |
|--------|--------|
| **Renderer** | `renderJellyfinPortal()` (shared with dashboard) |
| **API** | `/api/v1/media/publication/public`, `/api/v1/media/catalogue`, `/api/v1/media/purchase/guest` |
| **Auth** | Public (device-anchored, no user auth) |
| **DB Authority** | `magic_media_publications`, `media_passes`, `guest_purchase_requests` |
| **Readiness** | 🔴 **Publication 404** (audit #7) — same as dashboard |

---

### 13. Store (`data-view="store"`)
| Aspect | Detail |
|--------|--------|
| **Renderer** | `renderStore()` |
| **API** | `/api/v1/store/checkout-options`, `/api/v1/store/purchases` (list), `/api/v1/store/purchases/:id/approve` |
| **Auth** | `ADMIN+` for approve; `VIEWER+` for list |
| **DB Authority** | `store_purchases`, `store_items`, `account_entitlements` |
| **Readiness** | 🟡 **Legacy commerce** (audit #1, #3) — not current subscription authority; tenant ADMIN can approve own purchase (trust boundary issue) |

---

### 14. Staff & Permissions (`data-view="team"`)
| Aspect | Detail |
|--------|--------|
| **Renderer** | `renderTeam()` |
| **API** | `/api/v1/team` (GET), `/api/v1/team/invitations`, `/api/v1/team/invitations` (POST/DELETE), `/api/v1/team/:userId` (PUT/DELETE) |
| **Auth** | Backend requires `OWNER+`; frontend `canOpenView` admits `ADMIN` (audit #16 mismatch) |
| **DB Authority** | `business_memberships`, `invitations`, `users` |
| **Readiness** | ⚠️ **Invitation workflows overlap** (audit #17) — screen creates users with owner-entered password; backend has separate invitation routes unused by renderer |

---

### 15. Incidents (`data-view="incidents"`)
| Aspect | Detail |
|--------|--------|
| **Renderer** | `renderIncidents()` |
| **API** | `/api/v1/incidents`, `/api/v1/incidents/device/:deviceId`, `/api/v1/incidents/:id/acknowledge`, `/api/v1/incidents/:id/resolve`, `/api/v1/incidents/:id/action` |
| **Auth** | `VIEWER+` for list; `ADMIN+` for ack/resolve/action |
| **DB Authority** | `incidents`, `alert_policies` |
| **Readiness** | ✅ Loads; suggested_action application is Stage 1A (high-risk → ADMIN) |

---

### 16. Security (`data-view="security"`)
| Aspect | Detail |
|--------|--------|
| **Renderer** | `renderSecurity()` |
| **API** | `/api/v1/security/posture`, `/api/v1/devices/:id/security`, `/api/v1/devices/:id/security/verify`, `/api/v1/devices/:id/security/harden`, `/api/v1/routers/:id/capabilities`, `/api/v1/routers/:id/quick-config`, `/api/v1/routers/:id/webfig/secure` |
| **Auth** | `VIEWER+` for posture/capabilities; `ADMIN+` for verify/harden/quick-config/webfig |
| **DB Authority** | `devices`, `router_config_backups`, `device_ssh_keys` |
| **Readiness** | ⚠️ **One unknown device** (audit #18) — no mitigation applied |

---

### 17. Backups (`data-view="backups"`)
| Aspect | Detail |
|--------|--------|
| **Renderer** | `renderBackups()` |
| **API** | `/api/v1/devices/:id/backups`, `/api/v1/devices/:id/backups` (POST), `/api/v1/devices/:id/backups/:backupId` (DELETE), `/api/v1/devices/:id/changes`, `/api/v1/devices/:id/restore/:backupId` |
| **Auth** | `VIEWER+` for list; `ADMIN+` for create/delete/restore |
| **DB Authority** | `router_config_backups` |
| **Readiness** | ✅ Selection/history screen captured; create/restore not exercised (audit #19) |

---

### 18. Fleet Scans (`data-view="scans"`)
| Aspect | Detail |
|--------|--------|
| **Renderer** | `renderScans()` |
| **API** | `/api/v1/scans/:id` |
| **Auth** | `VIEWER+` |
| **DB Authority** | `fleet_scan_runs` |
| **Readiness** | ✅ Loads |

---

### 19. Magic Coins (`data-view="coins"`)
| Aspect | Detail |
|--------|--------|
| **Renderer** | `renderMagicCoins()` |
| **API** | `/api/v1/coins/wallet`, `/api/v1/coins/transactions`, `/api/v1/coins/adjust` |
| **Auth** | `VIEWER+` for wallet/transactions; `ADMIN+` for adjust |
| **DB Authority** | `magic_coin_wallet`, `magic_coin_ledger` |
| **Readiness** | ⚠️ **Ownership mismatch** (audit #15) — wallet reads calling user; transactions read entire business ledger |

---

### 20. Unlock Keys (`data-view="unlockkeys"`)
| Aspect | Detail |
|--------|--------|
| **Renderer** | `renderUnlockKeys()` |
| **API** | `/api/v1/unlock-keys` |
| **Auth** | `VIEWER+` |
| **DB Authority** | `router_unlock_keys`, `webfig_unlock_keys`, `magic_switch_unlock_keys` |
| **Readiness** | ✅ Loads |

---

### 21. Event Log (`data-view="events"`)
| Aspect | Detail |
|--------|--------|
| **Renderer** | `renderEvents()` |
| **API** | `/api/v1/syslog-events`, `/api/v1/ai-usage-events`, `/api/v1/terminal-history` |
| **Auth** | `VIEWER+` |
| **DB Authority** | `syslog_events`, `ai_usage_events`, `terminal_history` |
| **Readiness** | ✅ Loads |

---

### 22. Product Guide (`data-view="guide"`)
| Aspect | Detail |
|--------|--------|
| **Renderer** | `renderGuide()` |
| **API** | None (static content) |
| **Auth** | `VIEWER+` |
| **Readiness** | ✅ |

---

## Public / Captive Portal Routes (no user auth)

| Route | Handler | Tenant Resolution | Readiness |
|-------|---------|-------------------|-----------|
| `GET /api/v1/portal/:deviceId/config` | `PortalHandler.PublicConfig` | From device_id | ✅ |
| `POST /api/v1/portal/:deviceId/redeem` | `PortalHandler.PublicRedeem` | From device_id | ✅ |
| `GET /api/v1/portal/status` | `PortalHandler.PublicStatus` | From device_id | ✅ |
| `GET /api/v1/portal/:deviceId/media` | `MediaCatalogueHandler.PublicCatalogue` | From device_id | ⚠️ Requires `magic_media_publications` table + RLS |
| `POST /api/v1/portal/:deviceId/purchase` | `BillingHandler.PublicCreatePurchase` | From device_id | ✅ |
| `POST /api/v1/portal/:deviceId/guest-purchase` | `GuestWifiHandler.PublicGuestPurchase` | From device_id | ✅ |
| `POST /api/v1/portal/guest-purchase/:requestId/receipt` | `GuestWifiHandler.PublicAttachReceipt` | From request_id | ✅ |

---

## Database Authority Map

| Domain | Primary Tables | RLS Enforced | Notes |
|--------|----------------|--------------|-------|
| **Account/Identity** | `users`, `businesses`, `business_memberships` | ✅ | `app.current_business_id` |
| **Subscription** | `subscription_status` (view), `account_entitlements` | ✅ | Legacy `service_purchases` still read by Platform Accounts |
| **Routers/Devices** | `devices`, `router_config_backups`, `device_ssh_keys` | ✅ | `device_discovery_labels` (new) |
| **Sites/Portals** | `sites`, `portal_config` | ✅ | Site config preferred over business fallback |
| **Guest Internet** | `vouchers` (guest_voucher_grants), `guest_voucher_issue_batches`, `guest_purchase_requests`, `portal_sessions` | ✅ | Two voucher families share `vouchers` table |
| **Jellyfin Media** | `media_connections`, `magic_media_publications`, `media_passes`, `media_purchases` | ✅ | `Media.ConnectorEnabled` gate |
| **Legacy Store** | `store_purchases`, `store_items`, `account_entitlements`, `business_metered_quotas`, `device_allowances` | ✅ | 🟡 Not current subscription authority |
| **Magic Coins** | `magic_coin_wallet`, `magic_coin_ledger` | ✅ | Ownership mismatch (user vs business) |
| **VPS/Hub** | `vps_peers`, `router_connections` | ✅ | |
| **Telemetry/Incidents** | `telemetry_samples`, `peer_heartbeats`, `incidents`, `alert_policies` | ✅ | |
| **Audit/History** | `audit_events`, `syslog_events`, `terminal_history`, `ai_usage_events` | ✅ | |

---

## Readiness Conditions Summary

| Workflow | Ready | Blocker |
|----------|-------|---------|
| Account login/profile | ✅ | — |
| Subscription status | ✅ (legacy mode) | Subscription-only mode needs product decision |
| Router registration (Magic Hub) | ⚠️ | No live enrollment test |
| Device observations/discovery | ✅ (reconciled) | Was 🔴 — endpoints added |
| Guest sessions/disconnect | ✅ (reconciled) | Was 🔴 — endpoints added; needs live router test |
| Voucher issuance/sync | ✅ | Per-code lifecycle UI incomplete |
| Voucher first-use tracking | ⚠️ | Router install + first-use not tested |
| Jellyfin pairing | ⚠️ | Connector not enabled live |
| Jellyfin publication | 🔴 | 404 — connector disabled |
| Jellyfin paid playback | 🔴 | `PurchasesEnabled=false` hardcoded |
| Site portal publish/deploy | ⚠️ | Authority split (business vs site) |
| Store purchase approval | 🟡 | Legacy; trust boundary (tenant ADMIN approves own) |
| Fleet health | ✅ | State = UNKNOWN without live telemetry |
| Internet setup (WAN/LAN/WiFi) | ⚠️ | Requires live router |
| Team invitations | ⚠️ | Frontend/backend workflow mismatch |
| Security hardening | ⚠️ | Requires live router |
| Backup create/restore | ⚠️ | Requires live router |

---

## Legacy Dependencies to Isolate (Not Erase)

| Component | Tables | Current Coupling | Action |
|-----------|--------|------------------|--------|
| Store purchases → subscription | `store_purchases`, `account_entitlements` | Platform Accounts reads as activation evidence | Quarantine activation path; retain history |
| Device allowances | `device_allowances` | Router/site creation checks in legacy mode | Choose one capacity policy |
| Magic Coins | `magic_coin_wallet`, `magic_coin_ledger` | Wallet (user) vs Ledger (business) mismatch | Pick account or business ownership |
| Old voucher path | `vouchers` (sovereign, Hub expiry) | Shares table with guest internet vouchers | Restructure around explicit purpose |
| Service purchases | `service_purchases` | Old Store functionality | Do not migrate/reconnect to new workflows |

---

## Remaining Product Decisions (Blockers)

1. **Subscription model**: Per-account (individual login) vs per-business (staff seats)? Which features/limits governed?
2. **Capacity policy**: One authoritative limit (base quota vs purchased slots vs router/site/controller allowances)?
3. **Jellyfin connector**: Enable `Media.ConnectorEnabled` and `publishing.PurchasesEnabled`? Provider certification required.
4. **Store approval trust boundary**: Platform operator approval vs tenant self-approval?
5. **Coin ownership**: Account-scoped or business-scoped?
6. **Magic Hub enrollment**: Test with live router; define offboarding/re-enrollment (audit #20).

---

## Verification Checklist (Step 4)

| Test | Status |
|------|--------|
| Go build `./...` | ✅ |
| Go test `./... -short` | ✅ (all packages pass) |
| Frontend asset hashes match live | ✅ (6/6 files) |
| New backend routes registered | ✅ (4 endpoints in `routers` group) |
| Migration for `device_discovery_labels` | ✅ Created |
| Integration tests with disposable PG | ⏳ Pending |
| Role/tenant assertions in tests | ⏳ Pending |
| Denied actions cause no mutation | ⏳ Pending |
| Built frontend vs candidate API | ⏳ Pending |
| Hardware/payment journey | ⏳ Not applicable (no live router) |

---

## Deployment Artifacts (Step 5 - Not Yet Executed)

| Artifact | Location/Value |
|----------|----------------|
| Reconciled commit | TBD (after verification) |
| Go binary hash | TBD |
| Web asset hashes | See above (6 files) |
| Migration to apply | `20261023120000_device_discovery_labels.sql` |
| Backup location | TBD (fresh dump before deploy) |
| Rollback command | TBD (docker compose down + previous image) |
| Compose file | `/opt/mega-magic-go-restored-20261018T100554Z/deploy/compose.go-restore.yaml` |