# Prototype Instructions

Run the local server yourself and open the preview in the browser available to this environment. Do not give the user server-start instructions when you can run it.

Before making substantial visual changes, use the Product Design plugin's `get-context` skill when the visual source is unclear or no longer matches the current goal. When the user gives durable prototype-specific design feedback, preferences, or decisions, record them in `AGENTS.md`.

When implementing from a selected generated mock, treat that image as the source of truth for layout, component anatomy, density, spacing, color, typography, visible content, and hierarchy.

Build app UI in `src/`. Keep `.openai/hosting.json`, `worker/index.js`, `scripts/prepare-sites-build.mjs`, and `tests/sites-worker.test.mjs` intact so the same local prototype can be handed to Sites. Before a Sites handoff, run `npm run build` and `npm run test:sites`; the build must leave `dist/client/index.html`, `dist/server/index.js`, and `dist/.openai/hosting.json`.

# Current development sequence

The user requested deferring live router configuration and verification on 2026-10-06. Continue web-app workflows without requiring VPS/router details. Preserve Magic Hub, MikroTik, WAN, captive-portal enforcement and telemetry as pending requirements; do not claim live connectivity.

SME sidebar guidance is named 'Did you know?'. Platform publishers manage sourced cards. Keep compatibility marked unchecked until trusted router discovery exists. VPN, HotSpot/portal and RADIUS are separate roles. Local User Manager is a proposed authentication option; preserve it without claiming integration exists.

The user requested continuing through remaining non-live work without pausing for approval after each feature on 2026-10-06. Implement and verify consecutive work items, retaining the whole product scope. Pause only for real permission/access blockers or material business choices. Do not imply background execution continues after a final reply. Site WAN/budget and topology editors share one revision; preserve both settings when saving. Manual estimates/topology never count as discovered telemetry or verified enforcement.
