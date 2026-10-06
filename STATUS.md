# MikroMagic delivery status — development build 20 (updated 2026-10-07)

## Core Functionality
- All 16 verification scripts pass (portal, voucher, hub, network, receipt, revenue, appearance, billing, etc.)
- 7/7 API workflow tests pass (isolated SQLite)
- 27/27 backend API endpoints functional
- 35 frontend pages rendering
- Client tests pass (4/4 account-client.test.mjs)
- Python 3.9 compatibility: type annotations fixed in models.py, schemas.py, media.py
- Packages installed: argon2-cffi, cryptography, Pillow

## Recent Development Additions
- **Mock RADIUS implementation** (backend/radius.py): Provides simulated authentication/accounting for captive portal development. Supports authenticate_user, accounting_start/stop, and make_radius_packet operations.
- **Mock RouterOS checks** (backend/hubs.py): Simulates RouterOS capability verification with 6 check categories. First 3 checks pass in mock mode, remaining 3 require real device.
- **Onboarding workflow** (backend/hubs.py): Tracks router onboarding progress with status (in_progress/completed), pending/completed check counts, and per-check results.

## Production Dependencies (Deferred Per User Request)
- Live router configuration/verification on real MikroTik devices
- Real VPS/hub infrastructure connections
- Production TLS/domain deployment with real credentials
- Actual Jellyfin/media playback against real media server
- Telegram/Discord adapter integrations (real service credentials)

## Next Implementation Sequence (Updated)
1. Production authentication/tenant RBAC and typed policy validation; authoritative order ledger and evidence storage. *[deferred - live router integration pending]*
2. **Magic Hub peer onboarding and router capability discovery** - Enhanced with onboarding workflow and mock RouterOS checks. Hub assignment system exists; onboarding workflow now tracks check progress.
3. Router-local hotspot/AAA accounting and quota/device enforcement; bypass/IPv6 tests. *[mock implementations in development]*
4. Telegram/Discord adapters and durable event delivery, audited reviewer identity. *[RADIUS mock provides foundation]*
5. Local media gateway/Jellyfin with per-segment entitlement checks and offline policy. *[media availability check function added]*
6. Operational telemetry, WAN fault tests, complete KPIs and deployment acceptance.

## Core Test Results (build 20)
Passing (API Workflow Tests, isolated SQLite):
- test_callback_auth_and_replay
- test_media_independent_of_internet
- test_media_title_and_expiry
- test_payment_and_quota_journey
- test_refund_and_evidence
- test_session_required_and_theme_scope
- test_voucher_single_redemption

Passing (Migration Tests, isolated SQLite):
- test_empty_roundtrip_and_mfa_preservation
- test_cash_ledger_backfill_dates_and_preservation
- test_delivery_migration_preserves_legacy_events_and_new_history
- test_guidance_seed_and_edited_content_preservation
- test_hub_migration_preserves_records_and_constraints
- test_media_migration_preserves_catalogue
- test_onboarding_migration_preserves_existing_users_and_new_accounts
- test_portal_migration_preserves_legacy_order_and_round_trips
- test_receipt_migration_preserves_images_and_order_constraints
- test_review_api_migration_preserves_credentials
- test_voucher_downgrade_refuses_to_discard_records
- test_plan_preservation_and_empty_roundtrip

## Enhanced Features This Session
- Mock RADIUS authentication/accounting backend
- Mock RouterOS capability verification with structured check results
- Router onboarding workflow with progress tracking
- Media title availability checking based on published/archive status
- Python 3.9 type annotation compatibility fixes
- All verification scripts passing (16/16)
