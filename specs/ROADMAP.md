# ROADMAP — Path to "Fully Developed"

> Working mode (2026-09-26): I do NOT stop to ask before each step. This doc is
> the forward plan + your advance decisions. When something needs a decision, a
> numbered OPTION is listed below; you pre-answer once here, and I execute
> without pausing. MKController = reference to EXCEED, never copy. Market:
> Myanmar (UTC+6:30, MMK).
>
> "Fully developed" = a Myanmar hotspot owner can run a real monetized wifi
> business end-to-end on megamagic.com with no external help, and a guest can
> buy/use wifi in Burmese — verified live, not by self-report.

## Definition of DONE (the bar before self-announcing "fully developed")
1. Guest lands on a captive page → sees Burmese portal → redeems a code or buys
   a plan (owner bot approves) → gets online. (BUY PATH)
2. The bought entitlement is actually ENFORCED on the router (time + data), not
   just recorded. (ENFORCEMENT / applier wired to real RouterOS)
3. Owner configures plans/branding/pricing in the dashboard in one place.
4. Guest-facing + operator surfaces verified live on PUBLIC data.
5. No manual host steps for day-to-day operation (systemd, automation only).
6. Production hardened + release hygiene (only current + previous release kept).

## Required work — ordered

### PHASE A — COMPLETE THE BUY PATH (guests can actually buy)
- A1. **Guest captive portal HTML page** — MISSING today. The API endpoints
  exist (`/portal/:deviceId/config|redeem|status`, `guest-purchase`) and the
  dashboard has a Portal view, but there is NO public page a guest's browser
  loads on the hotspot. This is the #1 gap on the money path.
- A2. **Burmese localization** of that guest page (my-MM default, en fallback).
  welcome/terms/consent are already owner-set free-text — only page chrome is
  hardcoded. (Owner can type Burmese today.)
- A3. **Card/bank reference + receipt upload** on the guest page (bank via
  owner bot approve/reject — already built). Wire guest UI to it.

### PHASE B — ENFORCEMENT (make entitlements real on hardware)
- B1. **RouterOS applier / live transport** — the provisioner is lab-proven but
  the flag is OFF and there is NO real SSH/RouterOS transport. Data limits,
  voucher time-expiry, and reboot all ride this. THE unlock for most gated
  features. High-risk; needs deliberate live smoke against one approved router
  + OTP gate + rollback.
- B2. **Wire data-limit + time expiry enforcement** through the applier (so a
  bought "1GB" actually caps at 1GB).
- B3. **In-portal card checkout** (out-ship MKController's roadmap) — only if
  Myanmar payment rails permit; otherwise owner-bot is the money path.

### PHASE C — OPERATOR TRUST + GOVERNANCE
- C1. **Security-posture audit** — parse RouterOS version, flag affected vs safe
  (MK leads its page with this; entire fleet health readability).
- C2. **Config backups + per-device change-history timeline + one-click restore.**
- C3. **Role enforcement** on tenant routes (OWNER/ADMIN/OPERATOR/TECHNICIAN/
  VIEWER/SUPPORT defined but NOT enforced today) + owner team-management API.
- C4. **Alert digesting** (urgent interrupts; non-urgent batches).

### PHASE D — SCALE + COMMERCIAL
- D1. **Device-tiered plans / free trial** posture (mirror MK's Basic/Advanced,
  device-count billing, semestral/annual discount), our own tiering.
- D2. **Bulk fleet config/firmware push.**
- D3. **NATCloud-style LAN-side reach (cameras/DVRs)** — later.
- D4. Export a deployable demo (a seeded tenant a prospect can click real).

### PHASE E — GO-LIVE HARDENING
- E1. Production hardening pass (already partly in place): secrets, ports,
  ufw, systemd, backups, release pruning.
- E2. Full regression suite green + a scripted E2E against the public domain.
- E3. Written "running a business on Mega Magic" guide (Myanmar).

## Codeset of currently bulit but OFF (not a gap; a deliberate on/off)
- AI chain (analyze→meter→finding→incident→notify): built, gated OFF.
- Guest-wifi per-owner bot loop: built, flag-gated OFF.
- Provisioner apply/rollback/OTP: built (lab), transport not wired.
- Keystone RLS restricted pool: built, used by money path.
All become LIVE by flipping flags AFTER a live smoke — Lead-gated, deliberate.

## ADVANCE DECISIONS (ANSWERED 2026-09-26 — do not re-ask)
- D1. Payment = **owner-bot only, MMQR flow**. Guest sees the owner's MMQR
  payment QR + bank info, gets a **free grace trial** (guest_grace_minutes,
  set ~10) to complete the bank transfer — essential where only Starlink /
  limited mobile data exists. After paying, guest submits **paid phone number
  + last 6 digits of the bank/transaction number**; owner approves/rejects on
  their own Telegram bot. Card checkout is NOT in scope (strict Myanmar rails).
  → Requires: MMQR QR photo field on portal config + paid-phone + last-6 on
  GuestPurchaseRequest + the guest captive page.
- D2. Burmese **default-on** (my-MM); where a word has no clean Burmese
  equivalent, keep English (no bad machine transliteration).
- D3. Owner can add **at least 4–5 plan options**; guest picks the one to buy.
- D4. AI chain stays **OFF until Phase B (enforcement/applier) lands.**

## Phase A build order (accepted, in progress)
1. Guest captive portal HTML page (the missing buy surface).
2. Add MMQR QR photo field to portal config + owner-dashboard editor.
3. Add paid-phone + last-6-of-tx to GuestPurchaseRequest (schema + repo +
   handler capture), shown to owner for approval.
4. Burmese localization of the captive page (my-MM default, en fallback).
5. 10-min trial countdown surfaces grace on the guest page.
6. Wire owner-bot approve/reject display of phone+last-6.