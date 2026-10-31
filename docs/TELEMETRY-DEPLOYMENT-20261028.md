# MegaMagic telemetry deployment — 2026-10-28

> **Note (2026-11-01):** the old console this document describes (`web/app.html`, the vanilla `web/v2` and their scripts) was removed. The console is now the React app in `app/`, served at `/v2/`. This is a dated record; file names and UI steps below may no longer exist.

Deployed with the owner's explicit request. Live page:
[Router telemetry](https://megamagic.tech/app.html?release=telemetry-20261028#internet).
The release query avoids an old browser HTML cache.

## Source and preservation

The live API source `/root/mm-reseller-60fb7bb` had a binary SHA-256 identical to
the running executable: `6bba665c43dcc39b96d2a4bada30c59a46a0828755e216c81f95b11f30f496af`.
The collector, service and handler were added to that source in
`/root/mm-telemetry-20261028`. Only the handler read-service wiring and one server
route were integrated into existing backend files. The deployed role model has
no Operator role; the route uses its existing Admin gate. Its existing live
configurator supplies the read interface; no flag or role model was changed.

The frontend was merged from the unchanged public baseline, preserving account,
pass, voucher-batch identity/retry/verified-print protections, protection/QoS and
authoritative saved-site lookup. Seven frontend files were included. No fixture
or synthetic data was included. The canonical local repository remains older
than the running backend; do not replace production with a full local build.

## Release and checks

- API image: `mega-magic-api:telemetry-20261028`.
- Web image: `mega-magic-web:telemetry-20261028`.
- API binary SHA-256: `df1d24935534d61a635cbc09f5fe486f2c953f7baf06cf6811b957c469e5f8d`.
- Public HTML SHA-256: `8798bbe2c2f088d202978e3390e5fb9b3da90e99d14b160f571dcc8db1164a0f`.
- Persistent Compose: `/opt/mega-magic-go-restored-20261018T100554Z/deploy/compose.go-restore.yaml`, with both new image references saved.
- Current-source Go suites passed on the VPS: routertelemetry, provisioner/routeros, handler and server. Local targeted race and five UI suites had also passed.
- API health HTTP 200; all seven file hashes matched; anonymous telemetry rejected HTTP 401; authenticated real-device telemetry requests returned HTTP 200.
- Runtime environment, user/entrypoint/command, ports, mounts, security options, restart policy, Caddy config and application configuration matched the prior deployment. Other data-service containers stayed running with the same identities.
- Signed-in browser showed the saved hAP ax2 site/device, the new telemetry monitor and the pre-existing voucher summary: one unused and ten awaiting installation verification. No voucher was created/retried and no payment or router configuration was changed.

## Remaining boundary

The real UI reports **Reconnecting to RouterBOARD**. There is no fresh physical
reading while its saved management path is unreachable. The owner will connect
the router later; this deployment does not certify a new physical-router test.
RouterBOARD remains responsible for local captive login and access enforcement.

## Recovery material

The VPS directory `/root/mm-telemetry-20261028` retains source, binary, test/build
logs, `deployment-plan.json`, original Compose backup, release/rollback overrides
and runtime snapshots. Runtime snapshots stay private on the VPS and must not be
copied into Git or chat. Original image IDs had been removed from Docker, so exact
running containers were captured as local rollback bases first.

Rollback images are `mega-magic-api:telemetry-base-20261028` and
`mega-magic-web:before-telemetry-20261028`. The deployment switch had automatic
rollback for failed verification; rollback was not needed. A future rollback must
pin both saved images in Compose and recreate only API/web with `--no-deps`,
preserving all existing configuration and external volumes.

Local reproducible preparation and switch helpers are in
`work/telemetry-deploy-20261028/`. They are historical release tools with exact
preimage guards, not a command to rerun a completed deployment.

![Deployed telemetry monitor, showing the real disconnected-router state](megamagic-telemetry-deployed-20261028.png)
