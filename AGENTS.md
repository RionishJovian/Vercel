# Prototype Instructions

Run the local server yourself and open the preview in the browser available to this environment. Do not give the user server-start instructions when you can run it.

Before making substantial visual changes, use the Product Design plugin's `get-context` skill when the visual source is unclear or no longer matches the current goal. When the user gives durable prototype-specific design feedback, preferences, or decisions, record them in `AGENTS.md`.

When implementing from a selected generated mock, treat that image as the source of truth for layout, component anatomy, density, spacing, color, typography, visible content, and hierarchy.

Build app UI in `src/`. Keep `.openai/hosting.json`, `worker/index.js`, `scripts/prepare-sites-build.mjs`, and `tests/sites-worker.test.mjs` intact so the same local prototype can be handed to Sites. Before a Sites handoff, run `npm run build` and `npm run test:sites`; the build must leave `dist/client/index.html`, `dist/server/index.js`, and `dist/.openai/hosting.json`.

# Current development sequence

The user requested deferring live router configuration and verification on 2026-10-06. Continue web-app workflows without requiring VPS/router details. Preserve Magic Hub, MikroTik, WAN, captive-portal enforcement and telemetry as pending requirements; do not claim live connectivity.

SME sidebar guidance is named 'Did you know?'. Platform publishers manage sourced cards. Keep compatibility marked unchecked until trusted router discovery exists. VPN, HotSpot/portal and RADIUS are separate roles. Local User Manager is a proposed authentication option; preserve it without claiming integration exists.

The user requested continuing through remaining non-live work without pausing for approval after each feature on 2026-10-06. Implement and verify consecutive work items, retaining the whole product scope. Pause only for real permission/access blockers or material business choices. Do not imply background execution continues after a final reply. Site WAN/budget and topology editors share one revision; preserve both settings when saving. Manual estimates/topology never count as discovered telemetry or verified enforcement.

# Owner-confirmed revamp boundary — 2026-10-18

The user explicitly clarified that the planned workflow/navigation changes are a revamp of this existing app, NOT a full rebuild. Reuse and improve existing working screens, backend services, authentication, tenant isolation, database records, Hub integrations and deployment. Do not replace the stack, create a parallel product or reset data as part of this revamp. Make targeted changes and verify existing behavior remains intact.

Organize the owner experience around business tasks, not separate RouterOS engineering menus. Preserve the agreed top-level groups: Overview; Business (Revenue, Internet Vouchers, Jellyfin dashboards); Operations; Guest Portals; Store; Management. Operations includes Sites as the required step before router onboarding, Magic Hub one-paste connection, Devices, Site Topology with supported quick actions, Guests with verified statuses/actions, Fleet Health, and guided Internet Setup. Store lists platform items for sale. Internet Hotspot Portal is for internet data/time sales and vouchers only; keep Jellyfin Guest Portal and its server configuration separate. Never represent discovery, device actions or connectivity as working without corresponding live evidence.

For this live Go-app revamp, the existing UI source is `web/app.html` and `web/assets/`, served with the existing Go API. Preserve the separate `src/`/Sites prototype scaffolding; do not replace the production app with it. Read-only inventory is separately gated by `provisioner.inventory_enabled`; it must never enable the applier, guest disconnection or network writes implicitly. Friendly discovery labels are metadata, not device authentication or proof of online state.

Guests is an automatically loaded session dashboard across business sites, not router onboarding. Guests may arrive via downstream APs and switches; gateway Hotspot sessions do not prove physical AP attachment. Reuse saved Magic Hub management access throughout the app. Remember a valid working router across configuration pages and automatically use a single registered router; selecting a target in a multi-router business is context, not a new connection. Reserve review/apply for intentional configuration changes, never routine monitoring.

# Hermes local continuation — 2026-10-21

Before continuing this checkout, read `docs/HERMES-BUILD-CONTEXT.md` and
`docs/HERMES-CONTINUATION-20261021.md`. They record verified local environment,
source identity, build boundaries and the exact incomplete-commit resume point.
This checkout's observed origin is `RionishJovian/Mega`; similarly named repos
under `~/GitHub/` are separate and must not be substituted silently.

The role-vs-tier dependency set for `b5e7c0c` is now committed locally
(`d153af2`, plus stale-suite test alignments `2e6b5c4` and `f074134`); the
committed source passes `test:critical-ui`, the web audit and the Go suite
in an isolated export. Unrelated workflow-revamp work remains uncommitted —
keep separating it into coherent commits and preserve it. `npm run build`
checks the separate Sites prototype; validate production `web/app.html` and
the Go API independently. Older Hermes/Triage live-state notes are dated
reports, not fresh deployment or customer-router verification.
