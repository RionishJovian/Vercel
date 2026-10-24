# Production frontend on Vercel, stateful API and Hub on VPS

This repository preserves Mega Magic's existing Go application. `npm run build` copies the real `web/` frontend into `vercel-public/`. It never selects the unrelated Vite/Sites prototype. The old prototype command remains `build:sites` and is not used for production.

The Vercel middleware validates each browser Origin against its own request origin before translating it for the fixed megamagic.tech API upstream. Writes require explicit Origin and X-MM-CSRF:1. Client forwarded-host headers are removed; API caching is disabled. Existing API authorization and tenant/pass checks remain on the VPS. No database, router keys, WireGuard or app secrets are copied into the public deployment. Node 24 and the dependency lock are used. The newly introduced transitive Zod dependency is pinned to patched 4.6.7 through a targeted override.

Vercel is an additional frontend entry. It does not change megamagic.tech DNS or migrate the Go workers, database, Redis, NATS or WireGuard. Its API proxy must pass deployed login/cookie/refresh/read/write/CSRF verification before being called functional. Captive portals still require the correct router/guest network context; physical router acceptance remains separate.

## VPS release workflow

Ordinary pushes and PRs validate only. Only a manual dispatch on main with deploy_vps=true can build/publish API/web and enter the production environment. The job uses immutable image digests from the same commit and pinned official action revisions. No automatic schema migration is attempted.

The deployment uses an explicit SSH identity and the independently known pinned VPS Ed25519 key in deploy/vps-known-hosts. Registry authentication arrives on encrypted stdin and lives in a temporary Docker credential directory removed on exit. A host flock serializes all releases. Only api/web services are recreated. It validates production configuration first, saves the previous Compose/config/binary/image identities plus a validated DB dump, waits for JSON dependency readiness, verifies exact running image IDs and performs a public JSON readiness check. Failure restores the previous application images; additive databases are not blindly restored. No image pruning occurs.

Required existing GitHub secret: VPS_SSH_KEY. GITHUB_TOKEN is job-scoped with package read/write permissions. The production environment should retain the owner's reviewer policy. Host key changes require independent verification; do not substitute an unauthenticated ssh-keyscan result. Keep VPS SSH credentials in GitHub secrets, never repository files.

VPS runtime release is deliberately not invoked merely to test this workflow. Remote rollout and rollback execution must be reported separately from local tests and Vercel build readiness.
