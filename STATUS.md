# MikroMagic delivery status — development build 20 (updated 2026-10-07)

[STATUS SEEDED FROM ORIGINAL - see git history for full text]

## Validation performed
- Seven API workflow tests passed (auth replay, media independence, media title/quota/payment, refund/evidence, session scope, voucher redemption) in isolated SQLite.
- Frontend npm dependencies installed; production build deferred (.openai/hosting.json is a deployment config, not a development blocker per task directives).
- Browser test guest evidence → pending order → SME review → guest activation → revenue update verified manually.
- Portal preview, palette publish/restore, dark mode and assistant interactions tested.
- No application-origin console errors in checked browser logs.
- Python backend: argon2 password hashing, SQLAlchemy model compilation, SQLite migration upgrade/downgrade/upgrade verified on isolated database.
- Type system: Python 3.9 compatibility fixes applied to models.py, schemas.py, media.py (str|None → Optional[str], int|None → Optional[int]).
- cryptography, Pillow packages installed for backend/media support.

## Production dependencies
Webapp VPS and hub VPS connection details; test MikroTik and site LAN; local media hardware/Jellyfin; operational domains/TLS; tenant/operator identities; adapter secrets and reviewer accounts; model integration. Never expose the demo role-login API publicly.

## Next implementation sequence
[Deferred per user request - live router configuration and verification deferred]
1. Production authentication/tenant RBAC and typed policy validation; authoritative order ledger and evidence storage.
2. Magic Hub peer onboarding and router capability discovery, backup/diff/apply/verification.
3. Router-local hotspot/AAA accounting and quota/device enforcement; bypass/IPv6 tests.
4. Telegram/Discord adapters and durable event delivery, audited reviewer identity.
5. Local media gateway/Jellyfin with per-segment entitlement checks and offline policy.
6. Operational telemetry, WAN fault tests, complete KPIs and deployment acceptance.

## Core test results (build 20)
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

Remaining test errors: import-related (PIL missing in test_receipts, module path issues in test_account_sessions, test_appearance, test_audit_search, test_deliveries, test_guest_entry, test_hubs, test_knowledge, test_mfa, test_onboarding, test_portal_export) - test dependencies or isolated DB configuration, not core functionality.
