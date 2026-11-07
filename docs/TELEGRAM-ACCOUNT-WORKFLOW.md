# Account Telegram workflow

Implementation prepared on 2026-11-07. Deployment and real-message acceptance are separate from the local evidence below.

## One setup page

More → Your profile → Telegram approvals remains the only token form. Saved encrypted tokens are reused; there is no new credential table or required token re-entry.

- An active DEVELOPER account's business binding is the platform bot. It receives current app-pass renewal requests, legacy platform Store/billing purchase requests, and platform incident alerts from policy evaluation and AI. Approvals reuse the same transaction cores as the web console. The platform operator also retains their own business’s guest/media decisions through an explicitly tenant-scoped adapter; other customers’ guest/media decisions do not enter that path.
- Customer bindings handle only their own guest-payment and Jellyfin receipt decisions. A customer bot cannot resolve an order into a decision for another business.
- Scope and decision actor come from active database roles, never a browser-supplied field. Only the platform administrator may edit a platform-owned binding, including when legacy OWNER accounts share that business.
- A private approval chat uses the approver's numeric Telegram user ID as its chat ID. Groups use negative chat IDs and a separate positive approver user ID. Chat membership alone never authorizes a decision.

## Saved intent versus operating evidence

The saved enabled checkbox opts that binding into the account runtime. The old `billing.telegram` server token and the old guest-bot supervisor no longer create separate account receivers. The runtime reconciles every ten seconds and wakes after settings are saved. Media paid-access and router-write gates are unchanged.

The existing GET `/api/v1/guest-wifi/telegram-bot` response adds a `runtime` object containing:

- `scope`: platform or business.
- `token`: unknown, valid, or unverified.
- `receiver`: disabled, starting, running, error, or unavailable. Running requires a successful poll, not just a saved checkbox or launched goroutine.
- `approver_configured`: whether the saved destination/approver configuration is usable.
- `last_poll_at`: last successful receiver check.
- `last_delivery` and `last_delivery_at`: no recorded attempt, Telegram-accepted delivery, or failure during the current service run.
- `detail`: sanitized setup/receiver error information.

Status and delivery evidence reset with the process or binding. Telegram accepting a message does not prove a human read it. Background status refreshes preserve an unsaved token being typed into the form.

## Tests and deliberate sends

POST `/api/v1/guest-wifi/telegram-bot/test` probes getMe and getWebhookInfo without sending messages or consuming updates. It does not claim an end-to-end approval succeeded.

POST `/api/v1/guest-wifi/telegram-bot/test-message` is a separate explicit owner action, available through “Send test to saved chat.” It sends only to the current saved destination through a running receiver. Saving settings and probing credentials never send this test automatically.

App-pass renewal notifications are emitted only for newly created requests after the request transaction commits. Retrying the same existing request does not send another prompt. An approval prompt explicitly asks the platform operator to check the bank payment before activating a paid pass. Rejecting, granting, renewing, and replay handling use the shared web decision core; the platform actor is recorded.

## Security and compatibility

- One inbound stream is owned per token within the account runtime. Duplicate enabled tokens and multiple enabled platform bindings fail closed. Tokens reserved by the existing router-approval integration cannot start account receivers.
- Existing webhooks cause an explicit error. The runtime never deletes another integration's webhook automatically.
- Every decision re-reads the binding and checks token, enabled state, scope, actor, chat, approver, and business boundary. Disabling or rebinding also stops its old receiver.
- All updates in a Telegram batch are queued and delivered before polling beyond them. Both positive private-chat and negative group-chat restrictions are enforced.
- HTTP transport errors and Telegram error descriptions are sanitized before reaching logs or the browser. Raw bot-bearing URLs are not returned. TLS verification remains enabled; redirect following is refused.
- No schema migration, router reset, router-write gate change, or production configuration edit is part of this change.

## Evidence and remaining acceptance

Local validation passed: 365 frontend tests, frontend lint/typecheck/build, production release build, 18 deployment checks, eight proxy checks, 22 critical UI checks, and a reachable-vulnerability scan reporting zero called vulnerabilities.

Local evidence includes the full Go race suite with a newly initialized disposable PostgreSQL 17 instance, frontend tests/lint/build, and actual frontend rendering with a local fixture API in English/Burmese and day/night mode at a narrow viewport. The fixture does not prove real Telegram delivery.

After an authorized deployment:

1. Open the existing page; confirm the saved masked token remains and the scope is correct.
2. Correct any reported chat/approver mismatch. No saved IDs are silently rewritten.
3. Probe the token; wait for a successful receiver check.
4. Explicitly send a test notification and confirm it arrived in the intended chat.
5. Use a designated test request to verify rejection, approval with independently checked payment evidence, and refusal of replay/foreign-user/foreign-tenant actions. Do not approve an actual purchase merely to test the bot.
6. Disable and re-enable the binding; confirm queued callbacks cannot bypass the disabled state.

No real message or payment decision was sent during implementation. Production activation and real-message acceptance remain pending release authorization.
