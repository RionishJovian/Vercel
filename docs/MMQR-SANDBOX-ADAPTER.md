# MyanMyanPay sandbox adapter

Date: 2026-11-13 (Asia/Yangon)

This first implementation adds a Go client and a standalone sandbox check command.
It is not registered in `cmd/server`, does not change the guest portal, does not
write a database, and cannot grant Internet, approve an order or mark settlement.
There are no production payment endpoints in this adapter. Both credentials must
have test prefixes; mixed test/live credentials are rejected.

## Protocol evidence

Read the provider-linked SDK at commit
[`f4127c393ccca11e2f596d0e2706c652701a0fef`](https://github.com/nawing/MMPay-Node-SDK/blob/f4127c393ccca11e2f596d0e2706c652701a0fef/src/index.ts)
and the [REST reference](https://docs.myanmyanpay.com/api/).

- HMAC-SHA256 over `nonce + "." + exact body bytes`, hex encoded.
- Sandbox handshake precedes sandbox-create or sandbox-get.
- The operation nonce is included in the handshake body, and reused in the
  operation body/header. The handshake itself has its own signed header nonce.
- Each operation keeps its handshake token locally. The adapter does not copy
  the SDK's instance-wide mutable token, which can race across simultaneous
  orders.
- Inquiry's published response omits currency; the client normalizes an omitted
  currency to MMK for this MMK-only adapter. An explicit different currency fails.
  This default is not independent evidence of provider-reported currency.
- Creation must return a matching pending order, exact amount, MMK, QR and QR
  reference. A different response fails closed and requires inquiry/review.
- Callbacks must match a locally stored expected order, amount and QR reference.
  The REST reference requires the QR reference although the SDK callback type
  omits it. The adapter rejects an absent reference until the provider resolves
  this contract discrepancy.

The protocol is independently implemented using the Go standard library; no SDK
dependency is added. Public SDK behavior is compatibility evidence, not provider
certification. Provider sandbox credentials and test vectors remain necessary.

## Running the isolated check

Configure secrets through the runtime's secret mechanism, not committed files:

| Environment variable | Meaning |
|---|---|
| `MMPAY_SANDBOX_BASE_URL` | Exact HTTPS API base from the provider dashboard |
| `MMPAY_SANDBOX_APP_ID` | Sandbox application ID |
| `MMPAY_SANDBOX_PUBLISHABLE_KEY` | `pk_test_...` key |
| `MMPAY_SANDBOX_SECRET_KEY` | `sk_test_...` key |
| `MMPAY_SANDBOX_CALLBACK_URL` | HTTPS callback, required only for create |

The command defaults to inquiry. Creating a test request requires `-create`:

```sh
go run ./cmd/mmqr-sandbox-check -order mmqr-test-001 -amount 1000 -create
go run ./cmd/mmqr-sandbox-check -order mmqr-test-001
```

No anonymous HTTP callback receiver is included yet. With an approved sandbox
receiver, use its exact raw bytes and headers with `VerifyCallback`; select the
client credentials and expected order from trusted merchant/order storage. The
command's output always labels the mode sandbox and `grants_access: false`.

On a timeout or ambiguous response, inquire using the same order ID before any
new create. The client deliberately performs no automatic POST retries. A local
pending order must be persisted before eventual app-level creation is wired.

## Validation

```sh
go test -race ./internal/payments/mmpay/...
go vet ./internal/payments/mmpay/... ./cmd/mmqr-sandbox-check
go build ./cmd/mmqr-sandbox-check
```

The local TLS provider fixture covers signed create/inquiry, 20 concurrent
orders, isolated handshake tokens, redirect rejection, interrupted contexts,
provider errors, malformed/oversized responses and payment mismatches. Callback
tests use a frozen Node-generated HMAC vector and reject tampering, other
merchant keys, mismatched order/amount/QR, invalid currencies and missing
successful transaction references.

These are local protocol tests, not real-provider, database, browser or router
acceptance. There is no changed app UI to browser-test in this slice.

## Required follow-up before app integration

1. Establish the merchant/payee model for each SME and obtain sandbox credentials
   through secure configuration. Confirm test-key prefixes and API base URL.
2. Exercise provider simulator callbacks. Confirm nonce format/age/retry rules,
   QR reference presence, order ID echo, currency and inquiry behavior.
3. Add a tenant-scoped durable order/attempt/event ledger and unique merchant
   transaction references. An authenticated callback is replayable: this pure
   verifier deliberately provides no replay storage or nonce-age guarantee.
4. Atomically record payment once and enqueue fulfillment once. Router I/O must
   run outside the payment transaction and retry without collecting again.
5. Connect the existing guest purchase or guest voucher grant path, preserving
   first activation. Keep business billing and paid media separate. Track paid,
   access confirmed and bank reconciled independently.
6. Verify iPhone captive-browser/Safari recovery, same-device QR workflow and
   pre-payment wallet connectivity, including unavailable mobile data.

The existing owner decision keeps external service management behind MegaMagic's
own UI. This QR-only slice does not add redirects or third-party dashboards.
Any later hosted checkout/handoff needs its own compatibility/product review.
