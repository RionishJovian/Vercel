# Router-generated WireGuard keys (Wave 3, B2 and B4)

Status: built and tested against fakes and a disposable PostgreSQL. **Not run on a real router.** The feature is
**off by default** (`vps.hub_callback_enrollment_enabled`). Nothing in this document certifies live behaviour.

## What problem this solves

The existing one-paste flow (`POST /vps/peers/:deviceId/quick-enroll`) makes the router's WireGuard keypair **on the
server** so that one paste can configure the tunnel. The private key is then kept, encrypted, so a retry can reissue
the same script. That breaks the owner's rule that a router's private key never leaves the router.

The audit found two server-held copies, not one:

1. the `router_key` row in `hub_pending_enrollments` (kept 30 days), and
2. the `tunnelScript` inside the `management` row of the same table. That script contains `private-key="..."` and was
   kept up to 30 days.

## The new flow (router keeps its key)

```
Console ── POST /vps/peers/:id/callback-enroll (Admin) ──► app issues a single-use token (15 min), returns ONE block
Operator pastes the block in WinBox
Router: checks it runs RouterOS 7+, creates its dedicated WireGuard interface (RouterOS makes the private key itself),
        reads the PUBLIC key, POSTs {token, public_key, routeros_version} over verified TLS
        ──► POST /api/v1/hub-callback/enroll (public; the token is the only credential)
App:    validates shape, refuses RouterOS 6, spends the token (atomic), enrols the Hub peer for exactly that public key,
        records the allocation, builds the validated tunnel + management script, answers with it as text/plain
Router: runs the answer. The tunnel comes up; the existing auto-connect task finishes management login.
```

Properties, each covered by a test:

- The private key is never sent, stored, logged or returned. The app holds no router key in this flow.
- The token is 32 random bytes, stored only as a SHA-256, valid 15 minutes, **single use under concurrency**, and a new
  token replaces the old one (one live token per router).
- Validation (content type, size cap 2 KB, unknown fields, key shape, RouterOS version) happens **before** the token is
  spent, so a malformed or RouterOS 6 request does not burn it.
- RouterOS 6 has no WireGuard. It is refused in the pasted script (before anything is created) and again on the server.
  An unreadable version is refused too.
- A retried paste with the same public key reuses the existing Hub peer: no second `/32`. A peer with a **different** key
  is replaced only if it never completed a handshake; a connected router is never torn down by a setup request.
- If the app cannot record an allocation because the `/32` or key already belongs to another router, the Hub peer it just
  created is removed again, so a refusal leaves no orphan.
- The script uses `check-certificate=yes` and never falls back, because the reply is executed on the router.

## Purge for the legacy flow

The legacy flow stays available (and is still the default while the new one is off). Its exposure is now bounded:

- the `router_key` row lives **1 hour** (`database.RouterKeyLifetime`), not 30 days;
- on the first **verified, tenant-matched handshake** (seen by the 15-second auto-connect task or by the live-peer
  lookup) both copies are removed: the row is deleted and the private key is redacted out of the management envelope
  (which stays, because it still holds the management SSH key needed to finish the connection);
- once a router has handshaken, a quick-enroll request answers 409 `hub_peer_already_connected` instead of replacing
  its peer, since the key it would need is gone on purpose.

**Residual exposure of the legacy flow:** until a router first handshakes, the app still holds its key: up to 1 hour in
the `router_key` row, and up to 30 days inside the management envelope (the owner's rule that a late paste must still
connect). The in-process cache of a pending management envelope also holds it until restart. Only the new flow removes
this entirely. Turn the new flow on, move the console to it, then retire the legacy one.

## B4: bounded, idempotent, race-safe

| Concern | Change |
|---|---|
| Hub calls hung on an unreachable Hub | GET and DELETE are retried up to 2 times with jittered exponential backoff, each attempt freshly signed, a 25 s overall budget, and never after the caller's context ends. A create (POST) is **never** retried automatically: a timeout may have reserved the key. |
| Whole enrolment unbounded | Every enrolment request has a 60 s deadline. |
| Two pastes for one router racing | A per-router lock (waiters honour their context) makes enrolment single-flight. Without it, 8 simultaneous pastes made 8 allocations; with it, 1 (mutation-checked). |
| Duplicate `/32` or key | `hub_peer_state` has unique `(endpoint, tunnel_address)` and `(endpoint, router_public_key)`. Twelve concurrent claims on one address: exactly one wins. |
| `DecideGuest` held a transaction open across an unbounded router call | The router call now carries a 30 s deadline. On timeout the transaction rolls back and the guest's request stays `pending`, retryable. Semantics are otherwise unchanged. |

The Hub service itself (the real `/32` allocator) is not in this repository. These guards make the app's use of it
idempotent and detect duplicates; they do not prove the Hub never issues one.

## Database

`db/migrations/20261114120000_hub_callback_enrollment.sql`, additive, forward only (the previous release ignores the new
tables, so a rollback keeps working). Both tables are service-only: forced row security, no access for the tenant role,
and the API refuses to start enrolment if either protection is missing.

- `hub_callback_tokens`: token hash, expiry, consumed time.
- `hub_peer_state`: peer id, router **public** key, `/32`, endpoint, last verified handshake. No private key.

Python (`backend/`) needed no schema change: its plan table already allocates `/32`s atomically, never accepts a private
key, and reports `dispatch: not_implemented`. It stays a planning prototype. A test now covers eight concurrent plans.

## Configuration

```yaml
vps:
  hub_lookup_enabled: true
  hub_enrollment_enabled: true
  hub_callback_enrollment_enabled: true        # default false
  public_base_url: "https://megamagic.tech"    # https origin routers can reach; required when the flag is on
```

Env equivalents: `VPS_HUB_CALLBACK_ENROLLMENT_ENABLED`, `VPS_PUBLIC_BASE_URL`. Startup fails on a missing or non-https
base URL, or if Hub enrolment is not enabled. `GET /vps/enrollment-capability` now includes
`callback_enrollment_enabled` (additive).

## Not done, and not verified

- **The console does not call `callback-enroll` yet.** The capability flag is there; the screen change (and its Burmese
  copy) is a separate piece of work. Until then the endpoint is reachable only through the API.
- **Nothing here ran on RouterOS.** Unverified until a hardware trial: `/tool/fetch` POSTing JSON and returning `data`;
  `[:parse]` of a 5 KB reply; `[:find $mmVer "."]` on real version strings; and above all **certificate trust**. A router
  with no CA bundle for the app's certificate chain will stop with "did not accept this setup". That fails safe, but it
  may mean the CA must be installed first or this flow is unusable on older RouterOS 7 builds.
- The Hub's behaviour when the same public key is enrolled twice is assumed idempotent (the existing retry design relies
  on it); not verified against the live Hub.
- Rate limiting of the public callback is per client IP (30/min, shared with the portal), and whether production
  presents real client addresses is still unverified (STATUS item 4).
- The in-process lock matches the single API instance; the unique indexes are the backstop if that changes.

## Trial checklist (when a router is available)

1. Enable the flag on a staging server; request a script; confirm the block contains no `private-key`.
2. Paste on a RouterOS 7 router that trusts the app's certificate; confirm one peer, one `/32`, tunnel up, management
   login completes, and the app database holds no router private key (`hub_pending_enrollments` has no `router_key` row).
3. Paste the same block twice; confirm no second allocation.
4. Paste on a RouterOS 6 router; confirm "needs RouterOS 7", nothing created.
5. Paste on a router with no CA bundle; confirm it stops cleanly.
6. Turn the Hub off mid-paste; confirm a clear error and no orphan peer after it returns.

## Rollback

Set `vps.hub_callback_enrollment_enabled: false` and restart. The legacy flow is untouched. The migration is additive and
can stay applied.
