# Database migrations

Production schema changes are explicit operator actions. `deploy.sh api` checks
the schema before replacing the binary and stops safely when the required
device SSH objects are absent; it never mutates the database automatically.

## Device SSH host-key pinning rollout

Use a database backup and maintenance window. Run commands from the repository
root with the production environment explicitly selected:

```sh
ENVIRONMENT=production go run ./cmd/device-ssh-schema --action apply --config config.yaml
```

`apply` is additive and idempotent. It adds the five SSH columns, the lookup
index, and `NOT VALID` checks. Those checks protect new and updated rows while
allowing legacy credential rows to remain temporarily available for operator
review.

Enroll a verified `SHA256:` host-key fingerprint for every device that retains
SSH credentials. If a legacy credential should not be retained, clear all four
credential fields together. Then complete the tracked second stage:

```sh
ENVIRONMENT=production go run ./cmd/device-ssh-schema --action validate --config config.yaml
ENVIRONMENT=production go run ./cmd/device-ssh-schema --action check --config config.yaml
```

`validate` audits every row first and refuses to alter constraint state while
any incomplete credential record remains. The `check` action is deliberately a
deploy-ready check: it verifies the fingerprint-aware definitions and requires
both constraints to be validated. Only after `validate` and `check` pass should
the API be deployed with `./deploy.sh api`. Re-running `apply` afterward is safe
and preserves the validated state.
