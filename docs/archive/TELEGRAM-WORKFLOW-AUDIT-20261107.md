# Telegram workflow audit — 2026-11-07

Scope: source at main `2da523b06efc900138753c4c4236e3c0b59c6123`, production configuration, redacted database binding metadata, Telegram read-only getMe/getWebhookInfo probes, existing tests, and temporary local fake-API reproductions.

## Verified live state

- The saved account bot is `@Magicpointspayment_bot`. Telegram getMe returned ok=true and is_bot=true.
- One business binding exists, in the platform account business. It is enabled, has a private-chat target and positive approver user ID, and its token is encrypted with the enc:v1 envelope. Neither the token nor its encryption key was printed or saved by this audit.
- Telegram reports no webhook, zero pending updates, and no webhook error date. This is a point-in-time observation, not proof of delivery or receipt handling.
- Platform billing Telegram configuration is disabled, with no server bot token or chat configured. Guest Wi-Fi supervisor configuration is absent/default-off; no relevant process environment overrides were present.
- API startup logs contain none of the billing-bot configured, purchase bridge started, or owner-bot supervisor started markers.
- Therefore no application approval receiver is currently running for this token. Saving it from a platform-admin login did not configure the server platform billing bot.

## Frontend verification in the signed-in platform-admin account

Checked the live Safari session on 2026-11-07. The account displays role DEVELOPER. More → Your profile → Telegram approvals is the only token setup form. It shows a masked bound token, populated approval chat and approver user IDs, and the approvals/notifications checkbox checked. No values were edited or submitted.

The Platform workspace exposes Accounts, Renewals, Add-ons, Pricing, Businesses and Incidents; it contains no separate Telegram setup page. Source search confirms the only token form is Profile.tsx. That same profile component and business-scoped endpoint are used for the platform administrator and customer accounts, with no platform-admin-specific integration routing.

The distinction in this report is between backend consumers, not two frontend configuration pages. Expecting the sole Telegram form on a platform-admin account to configure platform approvals is reasonable; the current implementation does not connect those pieces. Fix the existing workflow and make its scope explicit rather than asking the owner to find a nonexistent second setup page.

## Workflow mapping

1. Profile → Telegram settings calls PUT `/api/v1/guest-wifi/telegram-bot`, scoped to the signed-in business. Admin role does not promote that binding to a global platform integration.
2. GET returns masked token metadata; empty token on subsequent saves preserves the existing secret. Stored production tokens are encrypted.
3. The Test action calls Telegram getMe only. It does not check supervisor enablement, receiver activity, chat delivery, receipt delivery, or approval processing.
4. Guest Wi-Fi purchase notifications and Jellyfin receipt reviews use each business's saved bot. Receiving approvals requires `guest_wifi.enabled` and the corresponding supervisor. That is off in production.
5. Platform Store/legacy billing approvals use `billing.telegram` from server configuration. The platform bridge combines StoreHandler and BillingHandler; it does not read the profile bot binding. This is also off.
6. Router high-risk provisioning approvals and AI incident messages have additional independent token/configuration paths. A profile token is not automatically used for either.

## Findings

### P1 — Platform approval receiver lacks fail-closed operator authorization

`internal/server/server.go:258-266,509-516`, `internal/telegram/bridge.go:62-91`, `internal/telegram/bot.go:336`.

The server starts the platform bridge when enabled plus token are set, even with chat_id=0. It never installs the business-bot authorization guard. With no chat filter, any sender who knows a valid request ID can submit an approval/rejection. Negative Telegram group chat IDs are also ignored by the transport's `allowChat > 0` condition. With a group correctly filtered, sender-ID authorization would still be needed to distinguish the designated operator from other members. The current production platform bridge is off, so the audit does not establish live exploitation.

Fix before activation: require configured operator chat and user IDs, enforce both for every decision, reject missing identity, support signed negative group chat IDs, and keep business bots separate.

### P1 — Token can leak through network-error details

`internal/telegram/bot.go:199-200,344-353`, `internal/handler/guest_wifi_handler.go:597-598`.

The HTTP client returns a URL-bearing error containing `/bot<TOKEN>/getMe`. VerifyToken returns err.Error(), and the Test endpoint forwards that detail to the browser. A closed local fake server reproduced exposure using a fake token; no real token was exposed during this audit.

Fix: sanitize transport errors before returning or logging them; preserve useful status/method context without the credential-bearing URL. Cover send, verify and polling paths.

### P2 — Multiple updates in one poll lose all but the first

`internal/telegram/bot.go:234-246,301-304`.

nextBatch advances the offset over every returned update, while Next returns only msgs[0]. On the following getUpdates call, remaining updates are confirmed past and discarded. A batch containing two approvals delivered only the first in the local reproduction. Telegram confirms updates when polled with a higher offset: https://core.telegram.org/bots/api#getupdates.

Fix: queue and deliver all updates before fetching/acknowledging later ones; cover multiple button taps and mixed allowed/foreign messages.

### P2 — Saved/tested state does not report operational readiness

`app/src/pages/Profile.tsx:62-74`, `internal/handler/guest_wifi_handler.go:566-598`, `internal/server/server.go:523`.

A token can be saved as enabled and pass Test while its receiver is disabled. This is the observed live state. The platform/business distinction is not sufficient to explain whether platform purchases will notify this bot.

Fix: show separate token validity, integration scope, receiver enabled/running, configured chat/approver, and delivery/approval acceptance states. A valid token must not imply a working approval workflow.

### P2 — Legacy billing notification targets a business UUID as chat_id

`internal/handler/billing_handler.go:74-85`.

notifyOperator calls SendMessage(businessID, text), while the configured operator chat is wired only to StoreHandler. An internal business UUID is not a Telegram chat target. The error is ignored, leaving no visible delivery confirmation. Source-confirmed; not exercised against real Telegram because it would send a message.

Fix: supply the configured platform operator destination and record/report delivery failures.

### P2 — Supervisor activation needs configuration defaults/validation

`internal/config/config.go:70-74`, `internal/config/config.go:setDefaults`, `internal/server/server.go:537-541`.

GuestWifi.Enabled and BridgeScanInterval have no defaults or explicit environment bindings. Env-only activation is subject to the Viper behavior already seen elsewhere in this repository. If enabled in YAML without a positive scan interval, time.Sleep(0) repeatedly reconciles against the database. This is source-confirmed, not activated on production.

Fix: default disabled, set a positive scan interval, bind environment keys explicitly, validate interval bounds, and test both YAML and env-only activation.

## Validation and limits

- Existing tests passed with race detection: `go test -race ./internal/telegram ./internal/server ./internal/handler ./internal/billing ./internal/provisioner`.
- Four temporary local audit tests reproduced batch loss, negative-group filtering bypass, token-bearing errors, and acceptance without a platform operator restriction. They failed the expected safety assertions. Probe source is preserved outside the repository at `/private/tmp/mm-telegram-audit-probe-20261107_test.go`; it is not a production change.
- Existing business-bot authorization checks cover sender, chat, tenant, enabled state, and token changes. These protections are not wired onto the platform bridge.
- No messages, receipts, approvals, rejections, getUpdates calls, or webhook changes were sent to the real bot. No purchase, router or production configuration was modified.
- End-to-end delivery, rejection, approval, replay/idempotency, and activation acceptance remain unverified on the real bot. Token validity and passing existing tests do not establish those outcomes.

## Recommended order

1. Fix operator authorization and token error sanitization.
2. Fix update queuing and notification addressing; add regression tests.
3. Make the platform/business scope and runtime readiness explicit in the UI/API.
4. Validate supervisor configuration and ensure one update consumer per token.
5. Activate only the intended integration, then run an explicitly authorized test notification plus test-order approve/reject flow with tenant and replay checks.

## Implementation follow-up

The owner approved correcting the single existing page on 2026-11-07. The feature branch implements a shared role-scoped account runtime, current app-pass renewal notifications/decisions, receiver/delivery evidence, batching and token-error fixes, and a separate explicit saved-chat test action. See [the workflow and acceptance record](TELEGRAM-ACCOUNT-WORKFLOW.md). This follow-up does not change the dated live audit observations above or establish deployment/real-message acceptance.
