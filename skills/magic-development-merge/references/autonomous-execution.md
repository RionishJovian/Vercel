# Execution boundaries for this workspace

This reference records operating preferences; it never overrides repository
security contracts, tool approval requirements, or explicit user instructions.

## Autonomous work that is safe by default

- Inspect code, configuration structure, logs with secrets redacted, and service
  status without changing external state.
- Run local formatting, tests, builds, linters, static checks, and disposable
  integration environments.
- Report concrete failures with relevant diagnostics. Never conceal a skipped
  integration test or describe it as passing.
- Preserve unrelated staged, unstaged, and untracked user work.

## Production boundary

- Never deploy, restart/stop a service, kill a process, modify a production
  database, or send a command to a customer/production router unless the user
  explicitly authorizes that production action in the current conversation.
- Resolve targets through configured SSH aliases or operator-provided inventory.
  Do not embed production IP addresses, usernames, credentials, or host keys in
  repository instructions.
- Verify SSH host keys. Never introduce `StrictHostKeyChecking=no` or an
  insecure fallback into a production path.
- Approved deployments use `./deploy.sh web|api|all`; do not replace binaries
  with raw `cp`, background processes, or ad-hoc `kill` commands.
- Keep a rollback artifact and verify the running version, health endpoint, and
  changed behavior after an approved deployment.

## Verification protocol

1. Run `go test -count=1 ./...`, `go build ./...`, and `go vet ./...`.
2. Run database/RLS integration tests against a disposable PostgreSQL instance
   when schema or tenant isolation changes.
3. Run real RouterOS tests only against a clearly identified disposable lab;
   enforce a target CIDR allowlist and pinned SSH host key.
4. Treat a skipped hardware/database test as **not verified**, never as success.
5. Before an approved deployment, inspect the actual diff and ensure no secret,
   temporary credential, or generated artifact is included.

## Routing safety

- Protected API compatibility routes belong under the authenticated,
  tenant-scoped `/api/v1` group.
- Gin rejects duplicate method/path registrations; route tests must prove both
  authentication behavior and the intended final path.
- Do not restore mock/fabricated endpoints merely to satisfy an old route name.
