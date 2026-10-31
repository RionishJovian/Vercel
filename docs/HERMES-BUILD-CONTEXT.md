# Hermes: MegaMagic local build context

> **Note (2026-11-01):** the old console this document describes (`web/app.html`, the vanilla `web/v2` and their scripts) was removed. The console is now the React app in `app/`, served at `/v2/`. This is a dated record; file names and UI steps below may no longer exist.

Verified locally on 2026-10-21. Read `AGENTS.md` first, then this file and
`docs/HERMES-CONTINUATION-20261021.md`. Recheck Git identity and current state at
each new task; the commands and observations below do not establish live state.

## Exact workspace and source boundaries

- Continue the reviewed checkout `/Users/aces/123/mega-magic-implementation`.
- Its observed origin is `https://github.com/RionishJovian/Mega.git`.
- Observed branch: `feature/access-tier-user-role-separation`; HEAD `b5e7c0c`.
- Other checkouts, including `/Users/aces/GitHub/Mega-Magic` and
  `/Users/aces/GitHub/Mega-Magic-App`, have different remotes and commits.
  Do not silently substitute or synchronize them. Resolve source identity before
  publishing or deployment; local continuation here does not settle release authority.
- The existing production app source in THIS checkout is `web/app.html` and
  `web/assets/`, with the Go API in `cmd/server` and `internal/`.
- `src/`, Vite, the Python demo/account APIs, and the Sites worker form separate
  prototype scaffolding. Preserve it, but do not use its build as proof that the
  production Go/web app works. `npm run dev` starts that prototype and may create
  local SQLite files; it is not the production Go app launcher.

## Verified local tools and dependencies

- macOS arm64; Node `v26.9.0`, npm `11.19.1`, Go `go1.26.5 darwin/arm64`.
- `go.mod`: module `mega-magic`, language version `1.26.0`.
- Existing `node_modules` is available. Do not reinstall or upgrade dependencies
  merely to start work. Compare `package.json` and lockfile first.
- Docker client `29.8.0` is installed, but its local daemon was unreachable on
  2026-10-21. Database/container integration readiness has NOT been established.
- The restored Compose reference uses PostgreSQL 16, Redis 7 and NATS 2.
  `deploy/compose.go-restore.yaml` names existing external volumes and production
  services: it is not a disposable local test stack. Do not start it for tests.
- Database tests use an explicitly selected disposable fixture (`MM_TEST_DB_URL`,
  and sometimes `MM_TEST_DSN`). Never reuse production connection settings.
  Skipped database tests do not count as integration coverage.

## Commands and what each establishes

Run from the exact repository root:

```sh
git status --short
git branch --show-current
git remote get-url origin
git log -1 --oneline
git diff --check
npm run test:critical-ui
go test ./internal/model ./internal/auth
go test ./internal/handler ./internal/server ./internal/database
go build -o /tmp/megamagic-local-build-check ./cmd/server
```

Use a task-owned scratch directory instead of `/tmp` when preserving build
evidence. The broader Go test line is a suggested check, not a recorded pass;
inspect skips and fixture requirements. Do not run live/router tests implicitly.
Use focused tests for changed behavior, then the required release checks.

- `npm run test:critical-ui`: current local DOM/contract regressions. It currently
  includes uncommitted scripts and dependency changes; a clean checkout of HEAD
  does not have the command. A pass describes the exact tested working tree.
- `npm run build`: builds the React/Sites prototype (`dist/client`, `dist/server`,
  `dist/.openai`); it does not compile or validate production `web/app.html`.
- Check production inline JavaScript syntax separately with Node's `vm.Script`;
  then exercise affected production screens with real DOM/browser checks.
- `go build ... ./cmd/server`: compiles the actual Go API; it does not prove that
  database migrations, external services, deployment or router operations work.

On 2026-10-21, `go test ./internal/model ./internal/auth` passed and the actual Go
server compiled successfully. This is local build evidence only.

## Working rules

Revamp the existing app; preserve data, auth, tenant isolation, working Hub
integration, Internet/Jellyfin separation and the existing stack. Read each
subdirectory's instructions before changing it. Preserve unrelated dirty work.
Inspect diffs before staging; do not use blanket staging, resets, cleanup or
stashes to make the checkout look clean. Never weaken tests just to get green.

Track implemented, tested, migrated, deployed and live/hardware verified as
separate states. No production, database migration, customer-router, payment or
message action follows from this setup. Continue routine local app work safely;
carry forward explicit user approval only for its exact scope.

## Knowledge and memory

Default-profile Hermes memory is `~/.hermes/memories/MEMORY.md` and `USER.md`.
They load as a frozen snapshot in a fresh session. A profile has its own memory.
Use Hermes's memory tool for concise durable facts; keep detailed build and
handoff evidence in these repository documents. Never store credentials in memory.
`~/.hermes/MEMORY.md` and older Triage deployment notes contain historical values;
their dates are not current verification. Local checkout evidence takes priority
for this local continuation, and live operations require fresh evidence.

The selected model setup is GLM 5.3 main, GLM 5.3 Flash vision, and Qwen3 Coder
fallback, all through the existing OpenRouter key. The user excludes Anthropic.
Paid OpenRouter model use is explicitly intended; the old free-only note is stale.
