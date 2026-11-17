---
name: megamagic-ui
description: ALWAYS invoke when generating or editing Mega Magic frontend: the React console in app/, public pages and guest portals in web/, router login pages, CSS, and English/Burmese copy.
---

# Mega Magic UI

Design decisions are dated owner decisions in `AGENTS.md` section 4 (Visual design). Read them before changing colours, type or layout; they supersede older notes.

- Console is `app/` (React, Vite, served at `/app/`). Public pages and portals are `web/`. The router-served login page is generated in `internal/provisioner/routeros/portal_deploy.go` and `portal_design.go`.
- Tokens live in `app/src/styles/tokens.css` (console), `web/assets/css/landing.css` (public pages) and inline in `web/portal.html` and `portal_deploy.go` (guest portals). Use tokens, not raw hex.
- Text 4.5:1, control edges 3:1, in Day and Night. Tap targets 44px on phones. Keep focus rings, reduced-motion and `aria-live` for status.
- Burmese: show only the active language, never mixed in one string; line height 1.75 (headings 1.6). Add every new string to both languages.
- Dates and ages go through the shared `Ago` component; compact data ages stay as decided.
- Sample data is labelled as sample data.
- Verify with `npm run test:critical-ui` and the `app/` tests (`npx vitest run`, `npx tsc --noEmit`, eslint). Look at the page in a browser when the change is visual.
- After a capability change or a device deletion, do the full-app copy sweep in `AGENTS.md` section 6a.
