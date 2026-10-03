# App workspace layout

The final shared layer is `web/assets/css/layout.css`. It owns workspace
spacing and control geometry; `redesign.css` owns the palette and visual skin.
Network workspace spacing was moved out of inline HTML into this shared layer.
Do not restore a centered/narrower card or add another page-specific override.

- Desktop panel inset: 24px; mobile inset: 16px.
- Sibling workspace gap: 20px; mobile gap: 16px.
- Text controls and selects: at least 44px; phone text: 16px.
- Checkboxes/radios remain compact; hidden review/apply controls remain hidden.
- Page titles, leads and help actions use the common route header.
- Structured consoles keep their header/body separation; form workspaces use
  the same inset and full content width.

`deploy.sh web` content-hashes layout.css so immutable asset caches cannot
serve the previous geometry against current HTML.

Validation: run `scripts/test-workspace-consistency.mjs`, the network UI
regressions, and inspect authenticated core routes at desktop and phone widths.
This contract changes presentation, not router writes or business permissions.
