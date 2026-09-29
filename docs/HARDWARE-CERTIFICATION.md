# RouterOS hardware certification

## Latest certified run

**PASS — 2026-09-29.** The production per-device path was certified against
the disposable `routeros-test` CHR on `mikromagic-hub`, running RouterOS 7.15.3
(stable). The final re-certification used one immutable bound operation for
snapshot, apply, read-back, and restore, proving that a mid-operation database
target change cannot redirect work to another router. It also proved tenant
isolation, AES-GCM credential resolution, pinned SSH identity, listen-port
mutation to `13241`, live read-back, and restoration to the sandbox baseline
`13231`. An independent post-run read confirmed `wg-magichub` at
`10.99.0.3/30` on port `13231`.

The temporary database, local SSH tunnel, plaintext credential file, and
temporary encryption key were removed after the run. This certifies the control
engine and CHR behavior; customer hardware models should still receive their
own compatibility runs before broad rollout.

Mega Magic is hardware-certified only when the production per-device control
path passes against a disposable CHR or sacrificial MikroTik router. Unit tests
and a reachable SSH port are not sufficient.

The certification test proves, end to end:

1. The database resolves the exact `(business_id, device_id)` pair.
2. A different tenant cannot resolve the same device.
3. The AES-GCM credential decrypts and the pinned SSH host key matches.
4. The router returns a real RouterOS version and WireGuard state.
5. A backup/export snapshot is captured before mutation.
6. A lab-only WireGuard listen-port change lands and is read back.
7. The original state is restored and read back again.

The test refuses hostnames, public IPs, targets outside an explicitly supplied
private/loopback lab CIDR, and the known production network. Loopback is
supported for a pinned SSH tunnel to a remote disposable lab. It never accepts
insecure host-key mode. Do not point it at a customer or production router.

## Prerequisites

- A disposable RouterOS v7 CHR with a WireGuard interface (default
  `wg-magichub`).
- A throwaway/test PostgreSQL database migrated through
  `20260929120000_device_ssh_host_key_pinning.sql`.
- One `devices` row populated through `cmd/router-register`, including the
  router's real OpenSSH `SHA256:...` host-key fingerprint.
- `provisioner.allowed_target_cidrs` configured with the lab/management network;
  production router reads and writes refuse every target outside this list.
- A different unused UDP listen port for the temporary mutation.

Do not paste database passwords, router secrets, or the credential encryption
key into chat, source files, or shell history. Load them from a local protected
environment/secret manager, then run:

```sh
export MM_CERT_DSN='host=127.0.0.1 port=5433 user=... password=... dbname=... sslmode=disable'
export MM_CERT_CRED_KEY='...'
export MM_CERT_BUSINESS='...'
export MM_CERT_DEVICE='...'
export MM_CERT_LAB_CIDR='172.20.0.0/16'
export MM_CERT_ROTATE_PORT='13241'
export MM_CERT_WG_INTERFACE='wg-magichub' # optional
./scripts/certify-routeros-hardware.sh
```

The only passing terminal line that counts is the test log beginning with
`CERTIFIED`, followed by Go's `PASS`. A skipped test is not a certification.

## Database migration rollout

Apply the migration during a low-traffic window. Its short lock/statement
timeouts make lock contention fail safely rather than stall the service. The
credential constraint is installed `NOT VALID`: it protects new/updated rows
immediately but preserves legacy unpinned rows for operator review.

Run the pre-validation audit included at the bottom of the migration. For every
returned device, obtain the fingerprint through a trusted out-of-band channel
and re-register it, or explicitly clear obsolete SSH credentials. Never infer a
fingerprint from the first application connection. Once the audit returns zero
rows, run the two `VALIDATE CONSTRAINT` statements from the migration.

Take a database backup before applying to a non-disposable database. This
repository does not automatically apply the migration to a live environment.
