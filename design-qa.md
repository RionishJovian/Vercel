# Design QA — development workspace

final result: passed

Scope: desktop adaptation of the user's visual style, not a pixel clone of a mobile marketing page and not production acceptance.

Source visual truth: ../upload/IMG_5659.jpeg and ../upload/IMG_5660.jpeg (1170x2532). They are mobile landing screenshots with browser chrome. The user authorized carrying their palette/typography into a desktop webapp. No source dashboard exists at the same viewport, so region proportions/copy are intentional adaptations, not asserted 1:1 fidelity.

Implementation screenshot: /workspace/scratch/mikromagic-preview.jpg, browser viewport 1363x936. Full-view comparison: design-comparison.jpg combines user source and rendered workspace in one input. Source was fitted to 360x820, implementation fitted to 1170x820, preserving ratios. Density differences are documented; no pixel-exact match claimed.

State: SME Overview, dark mode, activated test pass, MMK 1,000 confirmed collections, floating Magic Dude open. Light mode was also inspected in browser.

## Findings and comparison history
- Initial P2: brand crop cut off the supplied logo. Corrected crop using actual 1170-pixel source coordinates. Post-fix screenshot displays the complete source Wi-Fi/wand mark.
- Initial functional blocker: development API ran outside managed preview network context. Unified API/frontend dev launcher fixed reachability; subsequent browser purchase journey passed.
- P3: source logo has ivory background and is placed as a small tile in dark mode; a dedicated transparent brand asset can improve integration later.
- P3: exact source font is unavailable. System Arial fallback approximates bold sans hierarchy; app scales body text for management tables.

## Required surfaces
- Typography: bold sans display title, muted body, readable product tables and labels. Different mobile landing type sizes intentionally adapted for desktop density.
- Spacing: generous title/section rhythm, restrained two-column work area, consistent 12px panel radii and thin separators. Desktop screenshot has no horizontal overflow; persistent navigation and chat launcher remain visible.
- Colors: dark forest, sage accents, ivory light palette and evergreen text mapped through theme tokens; no neon gradients. Appearance shows selected text/background contrast ratio; full semantic/accessibility palette checks remain functional follow-up.
- Image quality: real user-supplied logo crop, recognizable mark at native raster size; standard Phosphor icons rather than fabricated artwork. No media artwork invented for unconnected catalogue.
- Copy: honest disconnected states, demo telemetry, pending connection and non-payment money-transfer labels. Source marketing copy intentionally replaced with task content.

Focused inspection: brand crop, hero hierarchy, status indicators and chat overlay inspected in full-resolution browser screenshots; no precision-critical photo areas or densely rendered illustration needs extra crops.

## Browser interactions tested
Role selection; guest evidence submission; owner review/approve; guest pass activation/access check; revenue reconciliation; portal preview; light/dark switch; palette publish/restore; floating assistant open, send, context and close. No application-origin console errors in checked logs. Extension metadata errors excluded.

## Limits
Mobile device verification remains pending. This QA pass approves desktop development preview presentation only. Production auth, router enforcement, hub connectivity, media playback and complete business features are tracked in STATUS.md; no production completion implied.
