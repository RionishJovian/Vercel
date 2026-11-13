# Mega Magic

@AGENTS.md

`AGENTS.md` (imported above) is the source of truth for layout, commands, deploys and owner decisions. Update it there, not here.

## graphify code graph

The code graph lives in `graphify-out/` (gitignored, local to each machine). Build it with `/graphify .`; `graphify hook install` keeps it current after commits, pulls and branch switches.

- When `graphify-out/graph.json` exists, run `graphify query "<question>"` before broad Grep, Glob or multi-file reads. Use `graphify path "<A>" "<B>"` for relationships and `graphify explain "<concept>"` for one concept. Read `graphify-out/GRAPH_REPORT.md` only for broad architecture review.
- Read only the files and line ranges the graph points to. Never read lockfiles (`package-lock.json`, `go.sum`), build output (`dist/`, `tmp/`, `vercel-public/`) or dependencies (`node_modules/`, `.venv/`).
- Put these rules in every subagent prompt that explores code.
- After changing code, run `graphify update .` (AST only, no API cost).
