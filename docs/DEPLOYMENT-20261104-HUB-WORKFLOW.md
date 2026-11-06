# Magic Hub enrollment workflow correction

Released 2026-11-04. This is software release evidence, not physical RouterOS
acceptance or proof that the owner's router has reconnected.

- [PR #105](https://github.com/RionishJovian/Vercel/pull/105).
- Feature commit: `c620fdb9301800a3da25c62a75f996b515a7bb93`.
- Production merge: `eef916392bbb87fc8bed6124a07a33a8669de13a`.
- Previous release: `174510cf0cfcc274e19a333c0e20f26871292784`.
- [Release workflow](https://github.com/RionishJovian/Vercel/actions/runs/41794325474): validation, build and deploy-vps successful.
- Rollback directory:
  `/root/mm-rollout-backups/ci-eef916392bbb87fc8bed6124a07a33a8669de13a-20261104T075323Z`.
- No schema migration; the runner reported the schema up to date.

## Corrected behavior

The old wizard created a router-generated key with GatewayBootstrap, then called
QuickEnrollGateway using an unrelated app-generated key. The router identity guard
correctly rejected that combination. The new Create → Paste → Check sequence calls
quick enrollment directly and presents one complete script. There is no bootstrap
request. Existing-connection verification is reachable immediately and following
a saved-management conflict. Progress reflects actual completed actions, and
generation/verification lock navigation and duplicate requests while pending.

Backend tenant/role checks, confirmation, retry identity, encrypted pending keys,
management proof and mismatch guards are unchanged. Partial interfaces created by
the old workflow are not overwritten: their actual public key/allocation still
need local inspection before recovery. A STOP message may follow partial script
completion, which the updated guidance now states explicitly.

## Verification

Nine workflow cases failed against the former component before implementation.
The final source passed all 205 frontend tests, lint/typecheck/build, 22 existing
critical UI tests, two deployment and eight proxy tests. Focused Go race tests
passed for real script identity, quick-enrollment retries, tenant boundaries and
management proof. Independent review found no actionable issue and independently
passed nine workflow cases and typecheck. Existing NetworkEditor hook and large
map-bundle warnings remain.

Local browser fixtures exercised normal script creation/paste/check and the
saved-management conflict/check path, with no bootstrap request and no mobile
panel overflow at 390px. These were synthetic API responses; no router script was
executed. Local preview was stopped and fixtures removed.

Production API/web labels match the release. Public readiness (database, Redis,
NATS), endpoint authentication, retained broadband forced RLS/runtime policies,
and entry/script byte comparisons passed. The served Hub lazy chunk matches the
release container and contains quick-enroll/direct-check with no bootstrap path.
Console HTML SHA256:
`e47d582203962ed65d9c4cb9269c5e4013e2538554053813b86db560c7ace078`.

An already-open browser retained the old SPA on same-URL navigation. Opening
`/v2/?release=eef9163#/hub` loaded the new release: script creation shown first,
direct Check existing connection available, rail todo/todo/current after opening
the check screen, zero bootstrap requests and zero API writes. No live enrollment,
finish verification, credential replacement or router configuration was submitted.
Actual router connectivity remains unverified.

## App-side connection diagnostics — latest release

The owner confirmed the physical router unavailable and requested app-side work
only. A read of pending expiry metadata showed both original purposes expired.
No private keys, passwords or ciphertext were retrieved for that diagnosis.

- [PR #106](https://github.com/RionishJovian/Vercel/pull/106).
- Feature commit: `f640c504acee7cee8fc4cdcc119f77fa1c8bd701`.
- Latest production release: `9e60b21db8c5936f7de1f99de1be15f5e2c3a38b`.
- [Release workflow](https://github.com/RionishJovian/Vercel/actions/runs/41798723292): validation, build and deploy-vps all successful.
- Previous release: `eef916392bbb87fc8bed6124a07a33a8669de13a`.
- Rollback directory:
  `/root/mm-rollout-backups/ci-9e60b21db8c5936f7de1f99de1be15f5e2c3a38b-20261104T083009Z`.
- No migrations added; the runner reported the schema up to date.

LivePeer now adds independent management/setup labels after the existing role,
plan, tenant, router and signed Hub identity checks. The read queries saved
presence and pending expiry metadata only: no secret retrieval/decryption,
new installation, expiry extension, SSH or router command. Unknown diagnostics do
not replace independent handshake evidence. The UI distinguishes expired setup,
incomplete preparation, pending installation and repair awaiting verification.
Saved access is never called authenticated connectivity.

Verified full uncached Go race suite against disposable PostgreSQL, repeated and
shuffled pending-record tests, all 210 frontend tests, lint/build, 22 critical UI,
two deployment and eight proxy checks. The translation catalog initially missed
registration of the new dictionary; it was registered and the complete suite
passed. Independent review found no actionable issue. Existing hook/map-bundle
warnings remain. Hardware-gated checks are not physical acceptance.

The local browser fixture observed expired setup, a later saved-but-unverified
state, removal of the old warning and no overflow at 390px, with zero writes.
The initial browser fixture changed state after its first read, which the fleet
poll consumed; it was corrected to use explicit phases. Preview and disposable
PostgreSQL stopped; fixtures removed.

Production labels/readiness, served entry and Hub lazy chunks, authentication and
retained tenant/runtime policies passed. Console HTML SHA256:
`31726c82eeb2f6320059cb6736c8412e85a7671790a789825ce3d3c24dffc39a`.
The actual signed-in LivePeer GET returned HTTP 200/no-store with
`connection={management:missing,setup:expired}` and no recent handshake. The live
UI shows the expired/local-recovery guidance and missing saved access. Browser
verification submitted zero API writes. No live key, peer or router changes were
made. Physical recovery remains deferred until local router access is available.
