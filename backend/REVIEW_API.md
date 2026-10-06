# MikroMagic delegated payment review API

This API is implemented in the account backend. Telegram/Discord bot handlers, notification dispatch, platform identity verification and deployment are not connected. No messages are sent by these endpoints. A provider label does not verify a bot connection.

## Authority and credentials

An authenticated SME owner creates a credential in Integrations or `POST /v1/review-credentials`. Required fields:

```json
{
  "label": "My counter review bot",
  "provider": "telegram",
  "reviewer_identity": "EXTERNAL_OWNER_USER_ID",
  "expires_days": 30,
  "can_decide": true,
  "authorize_service": true
}
```

Provider is `telegram`, `discord` or `custom`. Expiration is 1–90 days. Read-only is the default. The token is returned once; only its SHA-256 digest is stored. The token is a backend secret: never put it in button payloads, URLs, frontend code, chat messages or logs. Use HTTPS in deployment. Normal account tokens cannot authenticate the review API; delegated tokens cannot access the normal account API.

The credential authorizes this service to act on the creating SME user's behalf for that user's business. It is bound to the configured reviewer identity. The trusted adapter must validate the genuine Telegram/Discord/custom user before forwarding a decision and must reject other users. Matching a request field is not independent verification by MikroMagic of the external platform user. For several reviewers, issue separate credentials per configured identity; sharing one credential among unrelated users loses accountability. Owner account disable, expiry or revocation denies future authenticated requests.

`GET /v1/review-credentials` lists only the current SME user's credentials, without token/hash. `POST /v1/review-credentials/{id}/revoke` takes `{"reason":"…"}`. Credentials cannot be recovered; rotate by creating another and revoking the old one. Revocation and decisions serialize on the credential row: a committed decision before revocation remains in history, subsequent decisions are denied. PostgreSQL concurrency acceptance remains pending.

## Read review context

Use `Authorization: Bearer REVIEW_TOKEN` on these endpoints:

| Endpoint | Result |
|---|---|
| `GET /v1/review-api/orders?status=pending&limit=30&offset=0` | Business-scoped queue, maximum page size 100 |
| `GET /v1/review-api/orders/{id}` | Complete review context |
| `GET /v1/review-api/orders/{id}/receipts/{receipt_id}` | Private normalized PNG, no public URL |

Context includes the immutable package/policy/price/currency snapshot, guest identifier and username, site identifier/name, payment destination/instructions, method/reference, submission time, text evidence, current status/reviewer/reason, fulfillment status, receipt version, receipt metadata and up to 20 same-business duplicate-reference order IDs. Metadata reads query byte lengths without loading image blobs. Duplicate-reference warnings are advisory, not fraud proof. Images require separate authenticated fetching. `settlement_verified` and `adapter_connected` remain false: actual settlement and network/media enforcement are not verified.

`context_hash` fingerprints the order's current review fields. Keep it with the buttons displayed to the owner; do not silently fetch a fresh hash when forwarding a stale button. For changed context, fetch and show the new information, then ask the reviewer to decide again. This also catches changed text after an evidence resubmission, even when no new image was attached.

## Submit a decision

`POST /v1/review-api/orders/{id}/decision`:

```json
{
  "reviewer_identity": "EXTERNAL_OWNER_USER_ID",
  "request_key": "UNIQUE_CALLBACK_REQUEST_KEY",
  "context_hash": "64_LOWERCASE_HEX_CHARACTERS_FROM_DISPLAYED_CONTEXT",
  "decision": {
    "decision": "approved",
    "reason": "Payment independently confirmed at the counter",
    "expected_receipt_revision": 1
  }
}
```

Allowed decisions: `approved`, `rejected`, `needs_evidence`. Reject/request evidence require a nonempty reason. Expected receipt revision is mandatory, including zero for no images. A stable request key identifies one callback operation: repeat the exact body and key after a timeout. A repeat returns the recorded result and `replayed: true`; changing any field or order with that key returns 409. Tokens are bearer credentials, not HMAC signatures; request-key deduplication is not a substitute for validating provider callbacks at the adapter.

The callback must have decision permission, the configured reviewer identity, unchanged displayed order context and a pending order. Another decision, a new image or changed evidence causes 409. Approval uses the same transaction as dashboard review: one entitlement, one financial collection, outbox event, normal audit and an external decision record commit together. Title publication checks remain in force. Reject/request evidence do not create revenue or entitlement. The external action snapshots credential ID, service label/provider, configured identity, reason, order and result. Owner user ID is the authorizing account, while the external action identifies the actual service credential and its reviewer claim.

`GET /v1/external-review-actions` uses a normal SME login and returns up to 200 same-business external decisions. No destructive edit/delete endpoint exists. Dashboard refunds remain the existing manual record-and-revoke flow; delegated refund permission is not implemented.

## Delivery and acceptance work remaining

Durable submission/evidence notification dispatch, bot creation and platform authentication, callback signature verification, retry/dead-letter queues, service rate limiting, controlled operator support, PostgreSQL concurrency tests, deployment secrets/TLS, retention and real bot/browser acceptance. A stored payment decision does not activate live HotSpot access or media playback.

## Durable pull events — increment 15

Set `receive_events: true` when creating a credential, or call `POST /v1/review-credentials/{id}/events` with `{"receive_events":true}` from the creating owner's normal login. Each active, subscribed credential gets its own delivery record when a future event commits. Creating/enabling a subscription does not replay older events. Pausing prevents collection and new fanout; already queued records remain. Expired/revoked/unavailable-owner credentials cannot collect. Reassignment of their existing queues to replacement credentials remains pending.

The following transitions emit a new immutable order-state snapshot in the same transaction as the payment change:

- `payment.submitted`
- `payment.needs_evidence`
- `payment.evidence_resubmitted`
- `payment.approved`
- `payment.rejected`
- `payment.refunded`

An order's event revision increases for every transition. Repeated evidence rounds produce distinct events. An exact request retry does not emit another event. Legacy pre-migration event rows retain unknown timestamp/revision/payload and are not silently sent to newly created subscriptions. Receipt attachment itself does not resubmit evidence or emit an evidence-resubmitted event; changed images are available when the owner fetches current review context.

The authorized service polls `POST /v1/review-api/events/claim`:

```json
{"limit":10,"lease_seconds":120}
```

Limit is 1–50, lease duration 30–300 seconds. The response lists delivery ID, stable event ID, event name/order/revision/time, attempt counters, deadline, immutable snapshot and a lease token. The token is returned only for that claim; only its hash is stored. Keep the token private to the worker. One service's claim or acknowledgement has no effect on another service's delivery of the same event.

After the handler succeeds, `POST /v1/review-api/events/{delivery_id}/complete`:

```json
{"lease_token":"TOKEN_FROM_THIS_CLAIM","result":"acknowledged","error":""}
```

If handling failed, submit `result: "retry"` with a nonempty error description (maximum 1000 characters; omit credentials, secret URLs and other sensitive data). Failure schedules backoff of 15 × 2^cycle_attempts seconds, capped at 3600. Five claims per retry cycle lead to failed status after a reported failure or expired final lease. Expired earlier leases can be reclaimed. A replaced/expired lease cannot acknowledge a newer claim. Exact completion retries with the same token/result/error return the recorded completion; conflicting/stale completions return 409.

This provides **at-least-once delivery**. If a worker sends a provider message and crashes before acknowledgement, the event may be handled again. Deduplicate downstream by stable event ID (and intended recipient/service), or use the provider's idempotency mechanism when available. An acknowledgement is the service's report that its handler succeeded; it is not independent proof of a message's arrival, payment settlement or guest access. Do not acknowledge before successful handling. Keep handler runtime inside the lease and handle late-completion conflicts; renewal is not implemented. No external messages or provider calls are made by this API.

SME `GET /v1/event-deliveries` supports optional state (`pending`, `leased`, `acknowledged`, `failed`), limit 1–200 and offset. The UI shows the latest 100 records, service/order, state, subscription/credential availability, attempts and last error. `POST /v1/event-deliveries/{id}/retry` with a reason restarts only a failed delivery for an available, subscribed credential. Total attempts remain; the current five-attempt cycle resets. Existing completion/lease tokens are fenced off. Original outbox events and payment history are never deleted. Full attempt/error history, reassigning queues, lease renewal, push delivery and provider workers remain pending; only the latest completion/error and total counters are stored per delivery.
