# Current guest voucher portal review — 2026-10-23

final result: blocked

Scope: the owner-selected emerald liquid-glass guest voucher screen. This is a new scope; the historical desktop result below does not certify this screen.

Source visual truth: owner attachment `codex-clipboard-104aa84b-5063-4bc3-831e-4c4b5617a548.png`, supplied in this conversation (770×1186 pixels). The local clipboard file has expired. The visible reference guided the implementation, but a saved, normalized source/implementation composite could not be produced. Formal image-to-code fidelity sign-off remains blocked by that missing source artifact; do not label this a pixel-exact clone.

Implementation: `http://127.0.0.1:50233/router-portal-design-review.html`.
Screenshot evidence: `/Users/aces/Documents/Codex/2026-10-21/asd/work/journey-integration-20261023/emerald-glass-phone.png`, `emerald-glass-desktop.png` and `emerald-glass-preview.png` in the same directory.
Viewport: 393×709 CSS px (268.53×360.10 px card); narrow 320 px viewport also checked, with scrollWidth equal to viewport width. Native browser screenshot density differs because the existing browser zoom is 90%; no change to the owner's zoom was made. Device bezel/notch excluded from actual app content.
State: unused voucher form; browser-only preview submit tested with PREVIEWONLY and explicitly refused consumption/real access. No application console warnings/errors observed.

Required fidelity surfaces inspected:
- Typography: system Apple-style sans stack, 28 px mobile heading, restrained centered supporting copy and 15 px input/button. Editable text is HTML, not baked into the wallpaper.
- Layout: centered translucent card, generous top icon spacing, rounded silver edges, 46 px input and 50 px action; responsive desktop/short-height and reduced-effects fallbacks.
- Colour: dark teal/emerald wallpaper, silver edges, luminous green action; owner-configured brand colours remain supported with computed contrasting button text.
- Assets: generated reference-directed optical-glass wallpaper and transparent dimensional Wi-Fi mark. Both are real WebP assets embedded in the same HTML document, not network resources or handcrafted asset approximations.
- Copy: Wi-Fi Access / Enter voucher code to connect / Connect to Wi-Fi. Saved business name, greeting, terms and required consent remain supported.

Iteration history: first rendered card was slightly narrow and the icon undersized; card width increased to 76% of the available mobile width and icon box to 98×80 px. Removed initial autofocus so the keyboard/focus ring does not obscure the first view. Recaptured the refined implementation above. Full normalized reference comparison remains outstanding; no fabricated comparison image or passing fidelity claim is provided.

Functional verification: shared rendering used for preview and actual deployment; both assets inline; entire default router page 18,358 bytes under the 24,000-byte preflight bound; actual generated login JavaScript passes CHAP/PAP, consent, duplicate submit and inert-preview tests. Published-site/tenant isolation tests pass. Physical RouterOS/iPhone/Android captive browser verification is pending while the spare router is unavailable.

Implementation checklist: retain the current local iteration; obtain a persistent reference artifact or owner selection of this rendered iteration for formal visual comparison; complete physical guest login/traffic/expiry/reconnect acceptance before declaring the full journey complete.

---

# Historical desktop QA (unchanged; separate scope)

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
