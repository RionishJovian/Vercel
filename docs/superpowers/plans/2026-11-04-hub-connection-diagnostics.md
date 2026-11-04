# Magic Hub connection diagnostics implementation plan

> **For agentic workers:** Use superpowers:executing-plans to implement task-by-task.
> The owner authorized app-side continuation; local router access is unavailable.

**Goal:** Explain partial connection progress without changing router identities.

**Architecture:** Extend the existing authorized LivePeer read with separate saved
management and setup states. Derive setup states from pending-record expiry
metadata only; never decrypt or return installation material for this read. Keep
tunnel handshake evidence independent from these app-side states.

**Tech stack:** Existing Go/PostgreSQL and React/TypeScript.

**Spec:** User requested continuing app-side recovery after the workflow repair.
The live record has no handshake or saved management connection; both original
pending purposes are expired. Physical key comparison remains deferred.

## Constraints
- No migrations, credential replacement, peer rekeying, router writes or change to existing modules.
- Retain tenant/role/plan/owned-router checks before reading any diagnostic metadata.
- Saved credentials do not prove current management connectivity; unavailable reads remain unknown.
- Existing clients can ignore the additive response; missing fields remain unavailable in the new UI.

## Tasks
- [x] Add failing database tests for expiry-only reads, tenant/router boundaries and zero writes/decryption; implement metadata inspection.
- [x] Test saved/missing/expired/pending/repair states and protected handler reads; wire diagnostics into automatic management without SSH or secret responses.
- [x] Add UI regressions for expired setup, saved-but-unverified management, missing diagnostics and read-only refresh; implement concise guidance.
- [x] Run relevant PostgreSQL/race/frontend/regression checks, browser fixture and focused review; prepare delivery through the existing approved release process.

## Release verification protocol
After CI delivery, verify the served release and the owner's live read-only
diagnostic state. Record release/rollback evidence separately. Keep physical
recovery unverified while the router is unavailable.
