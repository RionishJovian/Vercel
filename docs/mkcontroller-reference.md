# MKController Competitive Reference — Mega Magic builds better

> **MARKET (2026-09-26, user decision):** target market = **Myanmar**, timezone
> **UTC+6:30**, currency = **MMK (Myanmar Kyat)**. All money is `price_mmk`
> (already the schema's unit). Where a product decision touches price, date, or
> locale, default to Myanmar's market not a generic Western default.
>
> **DIRECTION (2026-09-26, user decision):** MKController is a REFERENCE to
> study and EXCEED, never a template to copy. Goal = **better than MKController**,
> on the same surfaces but friendlier + deeper + tuned to Myanmar. This doc maps
> their features so we know what to beat; every build must carry its own identity.
>
> **UX (2026-09-26, user decision):** MKController's reach surface is
> MikroTik's OWN WebFig/Winbox — a raw, technical, router-expert interface. Mega
> Magic deliberately DIFFERS there: we are NOT "WebFig on the web." Our core
> position is a **user-friendly, senior-human-designed operator UX** (the
> MikroMagic philosophy carried forward) that HIDES router complexity behind
> clean, professional product surfaces — the machine stays abstracted, never
> exposed as config. Every interface decision answers: "does this read like a
> polished product a non-CLI person runs, or like a router's admin screen?"
> Competence on reach/monitor/monetize must ride on a surface the operator WANTS
> to use, not a technical tool they tolerate. Build better = friendlier + deeper,
> never "another WebFig."

Source: mkk controller public site + docs + pricing, studied 2026-09-26.
Purpose: ONE reference doc the platform builds against. Mega Magic's goal is to
EXCEED MKController on the same surfaces, not copy them. Review before every
feature slice; close gaps the competition already ships; out-build anything they
only promise on a roadmap.

MKController = cloud platform SaaS for MikroTik + UniFi + Starlink fleets
("a UISP for MikroTik"). Same positioning as Mega Magic: multi-tenant platform
SaaS giving operators a single console to reach, monitor, monetize, protect,
govern their routers. "We never own your connectivity, never change your plans,
never ask you to expose your router, never claim a roadmap feature as live."
We should hold ourselves to the same honesty bar.

===========================================================================
## THE MOAT: outbound tunnel, no public IP
Every device adopts OUT to the platform over an encrypted tunnel (MK uses
per-device OpenVPN; Mega Magic uses WireGuard). CGNAT / dynamic IP / no public
IP are non-issues. No inbound ports, no port forwarding, no spare VPS, no static
IP. This ONE property is what no self-hosted tool can match. It is the core of
both products and must never be regressed.

===========================================================================
## PLATFORM (multi-tenant) CONSOLE — processes MK ships
Reach, watch, act — "a console that acts, not a dashboard that watches."
1. Adopt device (tunnel) -> remote access (terminal/WebFig-style) from browser.
2. Real-time up/down alerts; NON-URGENT events BATCHED INTO A DIGEST so only
   urgent interrupts you. (We currently send flat Telegram alerts — add digest.)
3. Security posture: at adoption parse RouterOS version (MK parses v6.4->7.23,
   413 combos) and flag exposed/outdated vs safe. ("Your router may be in a
   botnet.") We DO NOT have this.
4. Backups + change history: automatic backups + per-device config change
   TIMELINE, one-click restore. "Recovery is a restore, not a rebuild." We DO
   NOT have this first-class.
5. NATCloud: tunnel through to the LAN side — cameras, DVRs, local web apps
   behind the router. We DO NOT have this.
6. Bulk operations: push config/firmware to 10..1000 routers in one action.
   We DO NOT have this.
7. Multi-tenant fleet console: your sites + each client's network side by side,
   TENANTS KEPT SEPARATE. (We HAVE business/tenant scoping.)

===========================================================================
## WIFI MONETIZATION — hotspot vouchers
### Operator flow
1. Adopt MikroTik -> build branded captive portal (pushed from cloud) ->
   generate voucher batch: quantity, session duration, DATA LIMIT (MB/GB),
   single-/multi-device, price, notes -> print PDF or printer -> distribute
   via resellers, per-batch tracking.
2. Guest: connect -> branded login -> redeem 6-digit code OR buy plan by card
   right on login screen (1hr/3hr/full-day) -> online in seconds with LIVE
   time/data on screen -> auto-deauth at voucher expiry (RADIUS + captive,
   RouterOS enforces).
3. Revenue: real-time dashboard, filter by device across fleet, per reseller/
   batch. Every voucher carries price so revenue ties back automatically.
### We have: batches (qty/duration/price/multi-device), captive portal, redeem,
### guest purchase, auto-deauth, revenue dashboard, reseller/batch tracking.
### We LACK: per-voucher DATA LIMIT (MB/GB combined) — the missing monetization
### lever, small to add. Also optional: live time/data on guest connected screen.
### MK roadmap (promised, not shipped): card checkout, mobile money (Airtel,
### M-Pesa), per-voucher speed limit, mobile revenue dashboard. -> We can
### out-ship them here: shipping card checkout + per-voucher data limit BEFORE
### they do = winning on the thing they only promise.

===========================================================================
## STARCONTROLLER — Starlink fleets (new, invite-only, from US$3/mo)
We have ZERO Starlink support. HOLD unless customers actually run Starlink
fleets. But PORTABLE IDEAS, independent of hardware, worth stealing:
- Password-free onboarding via invite inside OWNER's account (they stay owner,
  no credential handover). Removes the objection that kills onboarding.
- CROSS-ACCOUNT fleet dashboard + map (device/cost/data/site status, one pane).
- EPHEMERAL field sessions: time-limited (15min..48h), per-job, server-enforced
  expiry — never hand out a standing password.
- Fleet-wide billing risk: overdue/open/on-time per terminal, invoice settle.
- Per-terminal cockpit: usage, uptime, connected devices, plan, location.
- Roadmap (not shipped): proactive alerting, least-privilege scoped roles,
  access audit logs, cross-account governance, backup-WAN failover intelligence.

===========================================================================
## PRICING — "pay for devices you adopt, nothing else"
- Basic US$15/mo (2-5 devices): remote access (terminal), backups, monitoring,
  alerts. RouterOS v6.4->7.23.
- Advanced US$45/mo (2-5): everything + NATCloud (LAN-side, FREE of plan
  capacity) + WiFi monetization + Sites 2.0 + failover + content filter +
  multi-vendor alerts (push/email/Telegram) + ISP Manager (PPPoE/IPoE subs,
  plans, billing, dunning — coming soon).
- Enterprise custom from US$60/mo: contracted SLA, named engineer, priority.
- Per-extra device: Basic +US$4, Advanced +US$10 past 5; fleet-quoted.
- Semestral -20%, annual -30%. Free trial: 3 days, full Advanced, 5 devices,
  no card.
### Lesson for Mega Magic: device-tiered plans, trial, annual incentives are a
### clear commercial posture to mirror — but we can sequence our own tiering.

===========================================================================
## PRODUCT/ENGINEERING NOTES (how MK positions itself)
- "Most tools watch. MKController acts." Differentiation = ACTION surfaces
  (push config, restore, reach, bill), not passive dashboards.
- Honesty standard: nothing on a roadmap is claimed live. We hold the same bar
  (a feature is DONE only when audited + live, never by report).
- Brand voice: restrained, operator-trust, no AI-slop. Professional.

===========================================================================
## WHAT TO BUILD — prioritized against current Mega Magic
1. Per-voucher DATA LIMIT (MB/GB) — closes the monetization gap; small.
2. Security-posture audit (RouterOS version parse -> affected vs safe) — the
   trust/lead feature MK leads its MikroTik page with.
3. Config backups + per-device change-history TIMELINE + one-click restore.
4. Alert digesting (urgent interrupts; non-urgent batches).
5. Bulk fleet config/firmware push.
6. Card checkout on the captive login screen (out-ship MK's roadmap).
7. NATCloud-style LAN-side reach (cameras/DVRs) — later.
8. StarController surface — only if Starlink customers appear.

Architecture guard: keep the outbound-tunnel moat (WireGuard), tenant separation,
tail-light dependency-aware system-level reads. Verify live on real data, not by
self-report. Release hygiene: prune old releases.
