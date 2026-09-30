# Sites, Routers, Topology and Store recovery contract

**Old product reference:** `/Users/aces/Nish/mikromagic/app`  
**Current product:** `/Users/aces/Magic Development/mega-magic-project`  
**Audit date:** 2026-09-30

The first product remains the interaction and information-design reference. The
current Go authentication, tenant scope, encrypted credentials, target guards,
approval gates and audit trail remain authoritative.

## Sites

The first product had a dedicated Sites workspace, not merely a site table:

- physical-location cards with notes, timezone and coordinates;
- an interactive map with online, offline and unassigned state;
- router-to-site assignment;
- a persistent selected-site filter reused by Fleet, Vouchers and Live Users;
- device allowance visibility and a Go Live readiness strip;
- guarded site deletion when devices were still attached.

The current app preserves site CRUD, coordinates, device counts and router
records, but combines Sites and Routers into one long administrative page. The
map, shared site filter, readiness state and assignment-focused workflow are not
recovered.

**Recovery:** restore a dedicated Sites view backed by the current `/sites`,
`/routers`, `/fleet/health` and allowance APIs. Deletion must continue to use the
current backend's tenant and capacity rules.

## Routers

The first Routers workspace contained:

- separate router cards with site, model, state and last access result;
- Magic Hub and Local Connector paths;
- reachability testing, reboot, lock/key and reactivation states;
- one-time hub paste artifacts;
- gateway bootstrap, Hotspot SSID and Quick Config panels;
- telemetry and service counts;
- support controls and WebFig launch state.

The current app has real router records, encrypted connection management,
capacity enforcement and guarded topology reboot, but the operator experience is
spread across Sites & Routers, Templates, Terminal log and other pages. Quick
Config and secure WebFig launch are not implemented in the current engine.

**Recovery:** create a dedicated RouterBOARD workspace. Begin with read-only
identity, connection, reachability and configuration snapshot data. Any change
must use plan → approve → apply → verify → rollback and pass CHR certification
before it is enabled for production devices.

## Topology

The first topology was a site-scoped operations workspace:

- Site and gateway selection;
- WAN → router → LAN port → managed switch/AP → learned-device graph;
- Ports, Tree and Map views;
- searchable inventory and a selected-node properties inspector;
- editable port labels and manually described downstream devices;
- guest HotSpot/DHCP pool integrity and capacity state;
- Magic Dude investigation attached to individual topology nodes.

The current topology truthfully shows registered device relationships, fleet
state, hierarchy integrity and a guarded reboot action. It does not yet have
port-level RouterOS discovery, inventory/properties panels, view switching,
editable labels or guest-pool analysis.

**Recovery:** retain `/topology/map` as the registered-device layer, then add a
separate tenant-scoped port snapshot contract. A missing RouterOS probe must be
shown as unavailable, never replaced by invented ports or clients.

## Store

The owner's “Store” was implemented by the first product as the **Services**
workspace. It sold platform services rather than physical inventory:

- Sovereign Monthly and Annual Tier Passes;
- Magic Coin wallet top-ups at the configured MMK rate;
- eligible services and locked capabilities paid with Magic Coins;
- bank-transfer checkout using developer-owned payee accounts;
- payment-slip upload, reference, pending review and approval/rejection;
- purchase history, receipt review and entitlement activation;
- platform-only bank-account administration.

The migration preserved important data:

- `pricing_promo` and the public `/api/v1/pricing` contract;
- old service purchases consolidated into tenant-scoped `purchase_requests`;
- `account_entitlements`;
- `platform_service_bank_accounts`;
- Magic Coin wallets/transactions;
- consolidated `unlock_key_activations`.

However, the current billing engine only knows guest Wi-Fi plan effects. It must
not approve `monthly` or `annual` through that handler: those keys are not guest
passes, their migrated rows have zero Wi-Fi duration, and approval must update an
account entitlement rather than a RouterOS guest quota. The current unlock-key
surface is also read-only.

**Recovery:** add a dedicated Store API and transaction core with distinct
product kinds. The server, not the browser, resolves the published price and
effect. A purchase decision must atomically:

1. lock one pending tenant purchase;
2. validate the product and payment method;
3. debit coins or accept the reviewed bank receipt exactly once;
4. extend the account entitlement or create the requested unlock key;
5. write an immutable ledger/audit record;
6. return authoritative post-purchase wallet and entitlement state.

The Store UI is added only after those invariants have tests. It should recover
the first product's catalogue cards, wallet summary, checkout, purchase history
and developer review—not copy its Supabase write path.

## Implementation order

1. Dedicated Sites workspace using existing current APIs.
2. Richer Topology workspace on the current registered-device map.
3. RouterBOARD workspace and read-only configuration snapshots.
4. Store read catalogue and current entitlement/wallet summary.
5. Store bank-receipt checkout and developer review.
6. Atomic entitlement/key activation and Magic Coin checkout.
7. CHR-certified RouterBOARD mutations and secure WebFig session launch.

