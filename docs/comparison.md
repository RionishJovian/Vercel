# MikroMagic vs Mega Magic — Full Comparison

## Architecture

| Dimension | MikroMagic (Current) | Mega Magic (Go Rewrite) |
|-----------|---------------------|------------------------|
| Language | TypeScript + Node.js | Go 1.26 |
| Framework | TanStack Start (SSR) | Gin (HTTP) + NATS (async) |
| Runtime | Single Node process | Multiple Go services + NATS workers |
| Router communication | Sync HTTP/SSH (request waits for response) | Async NATS messages (request → queue → process → respond) |
| State | In-process memory | Redis cache + PostgreSQL |

## Database

| Dimension | MikroMagic (Current) | Mega Magic (Go Rewrite) |
|-----------|---------------------|------------------------|
| Engine | Supabase (PostgreSQL 17) | PostgreSQL 16 (portable, no Supabase lock-in) |
| Connection pool | PgBouncer (14 containers) | Direct pool via lib/pq |
| Migrations | Supabase CLI | Raw SQL in Go |
| Schema design | Flat tables from Firebase-style model | Normalized with explicit `business_id` tenant keys |
| Session store | Supabase auth | Redis + JWT |

## Deployment

| Dimension | MikroMagic (Current) | Mega Magic (Go Rewrite) |
|-----------|---------------------|------------------------|
| Process manager | systemd (`mikromagic-app.service`) | systemd (multiple services) or Docker |
| Release mechanism | Symlink swap (`deploy.sh`) | Docker Compose or systemd |
| Build tool | Bun + Vite | Go build (single binary) |
| Reverse proxy | nginx | nginx or built-in |
| Startup time | ~5s (Node + Vite SSR) | ~100ms (Go binary) |

## Container Overhead

| Dimension | MikroMagic (Current) | Mega Magic (Go Rewrite) |
|-----------|---------------------|------------------------|
| Docker containers | 14 (Supabase stack) | Optional (NATS + Postgres + Redis) |
| Supabase | Required | Not required |
| PostgreSQL | Supabase-managed | Go-managed |
| Auth service | Supabase Auth | Custom JWT in Go |

## Features

| Feature | MikroMagic (Current) | Mega Magic (Go Rewrite) |
|---------|---------------------|------------------------|
| Router provisioning | Sync SSH via Hub VPS | Async NATS → Hub VPS |
| Fleet AI scans | Node.js blocks event loop | Go goroutine (non-blocking) |
| Health monitoring | TS server functions | Go service + NATS consumers |
| Vouchers | TS + Supabase | Go + PostgreSQL |
| Portal | TS + Supabase | Go + Redis + PostgreSQL |
| Magic Dude (AI) | TS + OpenRouter | Go + OpenRouter |
| Topology canvas | TS (client-side) | Go serves data, canvas stays client |
| Quota enforcement | DB triggers | Go middleware + NATS dispatch gate |

## Security

| Dimension | MikroMagic (Current) | Mega Magic (Go Rewrite) |
|-----------|---------------------|------------------------|
| Auth | Supabase Auth + JWT | Custom JWT with refresh rotation |
| Tenant isolation | RLS on Supabase | Business ID on every NATS topic, PG row, Redis key |
| RBAC | Owner / Admin only | Owner / Admin / Operator / Viewer / Support |
| Audit trail | Basic | Searchable, exportable, session-tied |
| API auth | Supabase RLS | HMAC-signed + named-operation whitelist |

## Known Pain Points (Why Rewrite)

| Pain Point | In MikroMagic | In Mega Magic |
|------------|---------------|---------------|
| **Sync router calls** | A slow router stalls the entire caller | NATS async bus decouples caller from router response |
| **Event loop blocking** | Fleet AI scan blocks all other requests | Go goroutines for parallel work |
| **Supabase lock-in** | 14 containers just for backend infra | Portable Postgres, no Supabase dependency |
| **No RBAC** | Only owner/admin binary role | Full role matrix enforced at every layer |
| **Bundle size** | 293 kB initial JS | Same (frontend unchanged) |
| **Release bloat** | 55 releases, 2.5 GB | Single binary per service |
| **Stateful gateway** | WG bridge has state, can't roll updates | Stateless WG bridge designed for rolling updates |
| **No SLO alerting** | Health data exists, no alerts on top | Designed for per-site uptime SLOs + Telegram/webhook alerts |