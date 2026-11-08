# RouterOS backup capture — opt-in rollout (PR #167)

**Status:** developed on a draft branch, **not live** and **not hardware accepted**. Do not enable on a customer's router until a controlled hAP ax² SFTP transfer trial passes.

## What this implementation does

`POST /api/v1/devices/:id/backups` (existing owner/admin-protected route) can use an independently enabled capture-only service. It creates a RouterOS configuration export (`.rsc`) and an **AES-SHA256 password-encrypted** binary `.backup` on the selected router over pinned, per-device SSH; reads both RouterOS-reported file sizes; downloads each over the SFTP subsystem of a host-key-pinned SSH connection; checks lengths; calculates SHA-256 checksums; and AES-256-GCM seals the two byte streams under a dedicated server archive key. The sealed blobs and digests are stored in the existing tenant-scoped `device_config_backups` row. The browser receives **metadata only**, never file bytes, router passwords or archive secrets.

It does **not** permit Apply, Restore, reboot or network changes. The usual full live applier/OTP gating remains separate. The network configuration review/backup/rollback contract is not certified by merely producing an archive.

## Configuration (default OFF)

```yaml
provisioner:
  allowed_target_cidrs:
    - "10.77.0.0/24" # replace with the real narrow management network
  backup_capture:
    enabled: false
    credential_key: "" # same encrypted-device-credential key, optional applier-key fallback
    archive_key: ""    # separate randomly generated 32-byte key; NEVER commit
    password: ""       # separate strong RouterOS binary-backup password, 24–256 characters
    timeout: 30s
```

- Enabling requires a database, allowlisted target CIDR, tenant-scoped encrypted device SSH credentials, pinned SHA-256 host-key fingerprint, the strong RouterOS backup password, and a separate archive AES-GCM key. Failure prevents startup or backup, never falls back to unencrypted data.
- Export cap: **2 MiB**. Binary backup cap: **16 MiB**. Size differences after transfer are fatal. Nothing is accepted until both sealed artifacts and digests are present.
- RouterOS filesystem is not a durable offsite location. The app database is **off-router but on the app server**. Its scheduled encrypted off-server database copies (Cloudflare R2) must be independently enabled and verified for these rows to survive loss of the VPS. Consult `docs/BACKUPS.md`.
- The backup keys and password are essential recovery secrets. Losing the archive key means the application database copy cannot be decrypted. Losing the RouterOS backup password means the binary file cannot be restored.
- The sealed archive is not yet exposed by a recovery-download endpoint. Independent restore from the two encrypted artifacts **has not been demonstrated**. The existing app Restore action still performs only its older targeted WireGuard/service restore, not a full RouterOS binary restore.
- The RouterOS command/SSH and SFTP behavior must be tested on the real RouterOS version. CI fake SSH tests are not hardware acceptance.

## Hardware acceptance gates still required

1. Verify a real hAP ax² with a pinned SSH key and management IP inside the allowlist.
2. Enable capture only in a protected maintenance window, leaving the full applier disabled.
3. Capture; verify both files exist and are nonzero on the router, and both sealed blobs/checksums exist off-router in the application DB. No secrets in logs or API.
4. Deliberately disconnect SSH/SFTP, corrupt the size, and verify no successful archive row is created.
5. Verify a separate encrypted database offsite copy, restore the DB onto an isolated VPS, decrypt the artifacts with the held key and verify their SHA-256 digests. Do not restore the binary backup onto production hardware as a test.
6. Only after that: separately qualify owner-confirmed configuration Apply with verified rollback.

**Never merge this PR as evidence of complete production recovery.**
