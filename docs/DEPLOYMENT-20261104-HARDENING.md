# Hardening batch deployment — 2026-11-04 (Myanmar time)

The owner explicitly authorized merging and deploying Claude's completed batch,
PRs #94–#98. All five were merged normally, retaining their commits.

- Release: `a87397fc7e65627f5e91f48e797fdc156602c2a9`.
- Workflow: https://github.com/RionishJovian/Vercel/actions/runs/41696087384
- Validation, build and deploy-vps jobs: successful.
- Previous running API/web release: `4290c360afbf8fcf4ab297edc753d0f25e006a97`.
- Rollback directory on the production server:
  `/root/mm-rollout-backups/ci-a87397fc7e65627f5e91f48e797fdc156602c2a9-20261103T184821Z`.

Included: per-account login lockout and 10-character minimum for new passwords;
CodeQL and ESLint; notifier/evaluator/cache tests; Home/Fleet/Guests tests;
external page scripts and removal of inline-script permission from CSP.

## Edge policy deployment

Post-deploy checks found that the host HTTPS Caddy proxy's existing
`/etc/caddy/security-headers.caddy` overrode the new container's stricter CSP.
The old host file was retained as `edge-security-headers.caddy` in the rollback
directory above. The policy from the deployed web image was staged alongside
the existing host Caddyfile and passed `caddy validate`, then installed on the
host and reloaded with `systemctl reload caddy`. The proxy remained active.
No running-container files were edited.

The host edge policy is separate from the image deployment. A future policy
change must account for both layers. A rollback needing the previous policy
must restore the backed-up host policy and reload Caddy as well as restoring
the previous images.

## Live verification

- Both running containers report the full release SHA above; web health is healthy.
- Internal and public readiness report database, Redis and NATS all OK.
- Landing, sign-in, pricing, guest portal, `/v2/` and theme bootstrap return 200.
- Static page and referenced script bytes match `origin/main` release source.
- Public console HTML and theme-bootstrap hashes match the running container.
- Public CSP contains `script-src 'self'` without inline-script permission.
- Chromium loaded landing, sign-in, pricing, guest preview and the signed-out
  console with no observed CSP violations or JavaScript exceptions.
- The existing public-only E2E smoke script passed its two checks.

No real-account lockout trial, signed-in workflow test, payment operation,
router configuration or physical-router acceptance was performed. Existing
router feature gates were not changed. Local study notes were preserved and
were not included in the production batch.
