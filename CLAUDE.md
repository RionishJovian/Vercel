# Mega-Magic (MikroMagic)

## 1. Project Overview

MikroTik hotspot, voucher and router-fleet management platform (guest WiFi, billing, RouterOS provisioning, Magic Hub).

**Tech stack**
- Go 1.26 service: Gin, lib/pq (Postgres), go-redis, NATS, Viper, Zap, JWT
- Python account API: FastAPI-style app served by uvicorn, Alembic migrations
- Frontend: React 19 + Vite 6 (root `src/`, JSX) and the v2 rebuild in `app/` (TypeScript, Vitest, ESLint)
- Deploy: Docker (`Dockerfile.prod`, `docker-compose.yml`), Vercel proxy (`vercel.json`, `deploy/vercel/`)

**Main entry points**
- `cmd/server` — Go API server (routes wired in `setupRoutes()`)
- `cmd/provisioner` — RouterOS provisioner; other `cmd/*` are ops and schema CLIs
- `backend/app.py` — Python app factory (`backend.app:create_app`)
- `src/App.jsx` — root frontend; `app/src/` — v2 frontend
- `scripts/dev.mjs` — local dev orchestrator

## 2. Commands

Root (Node + Python):
```bash
npm run dev               # alembic upgrade, then server.py + uvicorn :8788 + vite
npm run build             # node scripts/prepare-vercel-build.mjs
npm run build:sites       # vite build + prepare-sites-build
npm run preview           # vite preview
npm run test:api          # python3 -m unittest discover -s tests -p 'test_*.py' -v
npm run test:critical-ui  # node --test scripts/test-*-ui.mjs
npm run test:sites        # node --test tests/sites-worker.test.mjs
npm run test:vercel       # node --test deploy/vercel/proxy.test.js
npm run test:deployment   # deployment config + hub reconcile tests
```

Go service:
```bash
air                                   # hot reload (.air.toml)
go build -o ./tmp/main ./cmd/server   # build
go test ./...                         # tests
go vet ./...                          # lint
```

v2 frontend (`cd app` first):
```bash
npm run dev         # vite
npm run build       # tsc --noEmit && vite build
npm run typecheck   # tsc --noEmit
npm run test        # vitest run
npm run lint        # eslint src
```

There is no lint script in the root `package.json`; lint lives in `app/` and `go vet`.

## 3. Token-Saving & Graphify Rules

- Always query `graphify-out/GRAPH_REPORT.md` or `graphify-out/graph.json` before running broad Grep, Glob, or multi-file reads.
  - Prefer `graphify query "<question>"`, `graphify path "<A>" "<B>"` and `graphify explain "<concept>"`; they return a scoped subgraph far smaller than the full report.
  - Read `GRAPH_REPORT.md` itself only for broad architecture review. Put this rule in every subagent prompt that explores code.
- Only read specific files and line ranges identified by the graph. Never read lockfiles (`package-lock.json`, `go.sum`), build folders (`dist/`, `tmp/`, `vercel-public/`), or dependencies (`node_modules/`, `.venv/`).
- Keep responses concise and output only modified code blocks unless asked otherwise.
- After modifying code, run `graphify update .` (AST-only, no API cost).
- `graphify-out/` is gitignored and local to each machine; build it with `/graphify .` and keep it current with `graphify hook install`.

## 4. Core Architecture

- **`cmd/` + `internal/` + `pkg/` (Go)** — `internal/handler` holds Gin HTTP handlers, `internal/database` the `DB` type (most-connected node in the graph), plus `auth`, `billing`, `broadband`, `fleet`, `hub`, `guestaccess`, `provisioner/routeros`. Handlers scope every query by tenant through `GetBusinessID()`.
- **`backend/` (Python)** — one module per feature (`vouchers.py`, `subscriptions.py`, `mfa.py`, `sites.py`), shared `models.py` and `schemas.py`; unittest suites live in `tests/test_*.py`.
- **`src/` and `app/` (React)** — `src/` is one JSX file per page; `app/` is the typed v2 rebuild. UI text goes through `useI18n()`; shared helpers such as `errorText()` and `asArray()` are used across pages.
- **Schema and migrations** — Alembic under `backend/migrations` and `migrations/`, raw SQL in `database/`, `db/`, `schema-postgresql.sql`. Production migrations are an explicit operator step, never automatic.
- **RouterOS work** — command execution goes through the `CommandRunner` abstraction; live-device and hardware-certification tests in `internal/provisioner/routeros` need a real router and form known import cycles with `internal/database` test files.
