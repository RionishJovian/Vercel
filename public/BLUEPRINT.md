# MikroMagic — product blueprint

Version 1 • 5 October 2026 • implementation baseline

## Goal
A cloud-managed MikroTik platform for SME owners to run their networks and monetize guest internet and local media access. Three connected interfaces, one consistent product. No requirement below is completed merely because a screen exists.

## Roles and ownership
| Role | Workspace | Responsibilities |
|---|---|---|
| Platform owner (you) | Cloud operations console | SME accounts, subscriptions, WireGuard Magic Hubs, router fleet, service health, scoped support, audit logs, global appearance and permitted tenant customization |
| SME owner (your client) | Network and business workspace | Sites, router onboarding, network configuration, captive portal settings, access packages, payments, vouchers, media, revenue KPIs, costs |
| Guest (SME customer) | Captive portal and media library | Choose package, submit payment evidence or redeem voucher, view remaining allowance, consume authorized internet/media |

SMEs pay you for cloud/software service. Guests pay the SME. Report these revenue streams separately. A role switch in the development preview is a simulation, not production authentication.

## Network foundation
SME creates a site, registers MikroTik, reviews a generated connection plan, applies it locally, and verifies WireGuard handshake, management reachability and capabilities. Magic Hub is the management path, not the default guest internet or local-media path. Isolate tenants and limit management reachability. Never show a demo connection as a real tunnel.

Network GUI includes router health, interfaces, Wi-Fi/hotspot, firewall/guest isolation, bandwidth policies, topology, WAN failover, compatible load balancing, configuration history and backup/restore. Router-local policies must continue without the cloud. Discover topology with evidence; label manual/inferred links. Validate RouterOS version, HotSpot device mode, IPv6 exposure and routing compatibility before applying changes. HotSpot/PCC compatibility must not be assumed.

## SME captive portal configuration
Owned by SME per site; platform supplies templates, guardrails and support.
- Selected MikroTik and guest interface/VLAN, address pool/subnet, DHCP, hotspot and hostname/TLS.
- Business branding, language, welcome text, terms, contacts and preview.
- Voucher login and direct-purchase options; payment instructions and destination.
- Device/concurrent session limits, speed, activation and expiration.
- Explicit pre-login walled-garden allowlist for portal, media catalogue and required checkout resources.
- Review diff, backup, apply, verify guest journey, rollback.

## Access packages
| Service | Modes | Enforcement |
|---|---|---|
| Internet | Data only, time only, data + time | Router/AAA enforces bytes and time; combined stops at first limit |
| Media | Timed library, single title, both | Media authorization checks entitlement, title and expiry on every stream/segment request |

Choose connected-time allowance versus fixed elapsed-time expiry after activation. Define duration units, counted upload/download bytes, device limit and speed. Single-title purchases define activation, viewing window and replay policy. Media and internet entitlements are independent; internet expiry must not silently cancel a still-valid media pass. Voucher codes must be unpredictable and issuance/redemption atomic. Counters survive reconnection and reboot. MAC binding alone is not adequate protection.

## Starlink and unauthorized access
Set site-wide WAN budget and billing reset, warning thresholds and owner-selected actions. Label router totals as estimates, not provider billing data; reconcile where possible. LAN streaming does not consume WAN if truly local; ingest, updates and remote streams can. Configure policies against actual owner plan, not a universal assumed cap. Restricted unauthenticated forwarding, intentional IPv6 policy, guest isolation, account/session controls and audit alerts address bypass risks. Test with representative bypass attempts; no claim of bypass-proof security.

## Manual payment review API
No payment-provider dependency. Dashboard is authoritative; adapters for Telegram, Discord and other platforms act through one API.

Order records tenant/site, immutable package/price/currency snapshot, customer/session, payment destination, method, transaction reference, receipt evidence and timestamps. Reviewer sees potential duplicate references and history. Approve, reject with reason or request more evidence. Authorize reviewer per tenant, authenticate callbacks, prevent replay and duplicate fulfillment, atomically resolve concurrent decisions, record audit. States: awaiting evidence → pending review → approved/rejected/needs evidence; fulfillment independently pending/active/failed. Receipt is a claim, not proof of settlement. Notify customer and reviewer. Support expiry, retries, refunds and revocation without deleting accounting history. An external messaging button must not grant access by itself.

## Media architecture
Preferred streaming engine: Jellyfin, behind MikroMagic entitlement enforcement. Evaluate per-title and timed access experimentally. Never expose server-wide API keys or unprotected alternate streaming routes to guests. Walled garden exposes catalogue/login/payment, not unconditional playback. Host media and access gateway at SME site to save WAN. Cloud stores management/catalogue metadata; site connector handles local enforcement and synchronization. Content is business-owned or licensed. Offline grace/cached entitlements require an explicit policy.

## Revenue and KPI dashboard
SME: confirmed collections less refunds, pending/failed payments, passes sold, average paid order value, revenue by internet/media/combined package, repeat purchases, active guests, redemption rate, media sales/plays and optional costs/profit. Date/site filters. Combined revenue counted once. Profit only with entered costs and labeled estimate. Platform: SME subscription revenue and account/service operations, never guest revenue mistaken as SaaS income.

## Magic Dude
Bottom-right floating wand launcher opens an overlay chatbot. Context includes authorized role, SME, site and selected router/screen. Guide setup, explain settings, diagnose using sourced telemetry, propose deterministic changes. Explicit review/approval, backup, verification and rollback. No raw AI commands silently applied. Demo responses are labeled; production requires model integration and tool authorization.

## Appearance
User screenshots define forest/sage dark and ivory/evergreen light, bold readable typography, spacing and subtle borders. Platform editor sets primary/accent/background/surface/text palettes, light/dark presets, scope, preview, publish, restore and tenant portal override policy. Validate contrast; maintain recognizable status colors.

## Connected acceptance journeys
1. Platform creates SME/site → SME adds router → reviews plan → real handshake and reachability verified → router appears in permitted fleet.
2. SME configures portal and packages → reviews/ applies changes → guest reaches portal with no unauthorized forwarding.
3. Guest creates order/submits evidence → tenant owner reviews → one approval → exactly one access grant → customer sees status.
4. Voucher redemption/first activation → bytes/time tracked → first configured limit stops internet → reconnection does not reset allowance.
5. Media catalogue reachable before internet purchase → paid title/library plays → unpaid/expired/direct URL/segment requests fail → valid independent media survives internet expiry.
6. Sales/payment/refund changes → correct tenant KPI/date filters → no duplicate counting or cross-tenant leakage.
7. WAN fault → router-local failover → cloud outage does not stop existing policies; incompatible load-balancing combinations rejected.
8. Theme change → preview → publish → selected scopes update → restore works.
9. AI recommendation → scoped evidence → approved change → verification or rollback, all audited.

## Delivery status and dependencies
This blueprint is established; implementation and verification must be tracked separately in STATUS.md. Development uses persisted demo state and explicitly simulated router/media/AI adapters until real infrastructure arrives.

Required for production: webapp and WG hub VPS details, test MikroTik/site media machine, operational domains/TLS, tenant identity setup, bot credentials and authorized reviewers, model credentials, media library and content rights. Production network enforcement, billing settlement, stream protection and outage recovery cannot be claimed from a browser prototype.
