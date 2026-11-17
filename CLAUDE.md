# Mega Magic

@AGENTS.md

`AGENTS.md` (imported above) is the source of truth for layout, commands, deploys and owner decisions. Update it there, not here.

## graphify code graph

The code graph lives in `graphify-out/` (gitignored, local to each machine). Build it with `/graphify .`; `graphify hook install` keeps it current after commits, pulls and branch switches.

- When `graphify-out/graph.json` exists, run `graphify query "<question>"` before broad Grep, Glob or multi-file reads. Use `graphify path "<A>" "<B>"` for relationships and `graphify explain "<concept>"` for one concept. Read `graphify-out/GRAPH_REPORT.md` only for broad architecture review.
- Read only the files and line ranges the graph points to. Never read lockfiles (`package-lock.json`, `go.sum`), build output (`dist/`, `tmp/`, `vercel-public/`) or dependencies (`node_modules/`, `.venv/`).
- Put these rules in every subagent prompt that explores code.
- After changing code, run `graphify update .` (AST only, no API cost).

## Backend, migrations and RouterOS principle

- Go (`cmd/` + `internal/`) is the production backend; `backend/` (Python) is a historical prototype.
- `db/migrations/` (with `db/baseline.sql`, run by `mm-migrate`) is the canonical PostgreSQL migration source. `migrations/` is retired history and `database/init.sql` is local dev/staging seed only.
- Old handoffs and design notes are in `docs/archive/`; do not treat them as current.
- RouterOS: one uniform CLI across RouterBOARDs; rely on documented board strings and runtime capability checks (`/interface/wifi` vs `/interface/wireless`, `/system/resource`), not per-SKU blocking or hardware-ownership disclaimers. Details: `AGENTS.md` section 8.

## Full-App Copy & State Sync Rule (mandatory)

When a capability expands or a device is deleted, sweep all of `app/`, `web/` and `internal/` on every route, in English and Burmese, for stale feature descriptions, raw or unlocalized timestamps, and any list, dropdown, topology node or counter that could still show a deleted router. Details: `AGENTS.md` section 6a.
