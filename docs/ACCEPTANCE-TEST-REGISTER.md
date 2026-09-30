# MikroTik Magic acceptance register

This is the release gate, not a manual checklist for the product owner. A feature is **not accepted** because its page renders or its endpoint returns `200`.

## Required proof for every mutable feature

1. The visible control responds to pointer and keyboard activation.
2. The browser sends the expected authenticated request and payload.
3. The backend validates authorization, tenant scope, and invalid input.
4. The intended database or RouterOS state changes exactly once.
5. A fresh read/reload shows the changed state.
6. A failed request leaves the page usable and displays a specific error.
7. No unrelated record, tenant, router, or configuration field changes.
8. The same critical path works at desktop and narrow viewport sizes.

Router operations have three additional gates: pre-change snapshot, post-change verification, and rollback proof. A database-only save must never be labelled as published to a router.

## Current automated coverage

| Area | Acceptance evidence | Status |
|---|---|---|
| Portal plan editor | Pointer Add/Remove/Save, PUT payload, fresh GET/readback, visible API failure and button recovery | Automated in `scripts/e2e-portal-ui.mjs` |
| Portal branding | Pointer Save, partial-field PUT, fresh GET/readback, persistent success confirmation | Automated in `scripts/e2e-portal-ui.mjs` |
| Portal preview language | English/Myanmar control state and iframe rerender | Automated in `scripts/e2e-portal-ui.mjs` |
| Payment preview | Plan selection reaches simulated MMQR confirmation flow without a production write | Automated in `scripts/e2e-portal-ui.mjs` |
| Voucher preview | Voucher entry reaches an explicit simulated result without a production write | Automated in `scripts/e2e-portal-ui.mjs` |
| Portal publish to RouterOS | Snapshot, upload, `html-directory` apply, probe, rollback | **Not implemented / not accepted** |

Run the portal gate with:

```sh
npm run test:portal-ui
```

`CHROME_BIN` may point to Chrome/Chromium on a non-macOS machine. The test uses a temporary profile, a loopback mock API, and no production data.

## Migration order and required gates

| Phase | Product areas | Required acceptance suites |
|---|---|---|
| 1 | Captive portal, voucher profiles, voucher redemption | Editor/browser suite; API integration; tenant isolation; RouterOS lab publish/rollback |
| 2 | Router enrollment, fleet, topology, live users | Enrollment and credential tests; topology readback; disconnect/ban safeguards; offline/error states |
| 3 | RouterBOARD settings, config panels, WebFig | RBAC; command allow-list; snapshot/apply/verify/rollback; session expiry; audit log |
| 4 | Magic Dude | Free-model enforcement; read-only default; tool authorization; prompt-injection and cross-tenant tests |
| 5 | Revenue, admin, team/commission, i18n | Ledger reconciliation; role matrix; pagination/filtering; locale completeness and fallback |

Each phase must pass locally, against the CHR lab where RouterOS is involved, and in a production smoke test before the next phase is called complete.

## Known gaps

- Saving portal branding currently persists cloud configuration only; it does not publish hotspot files to RouterOS.
- The project still needs authenticated browser suites for non-portal pages.
- The dormant Vite/TypeScript scaffold in `package.json` is not the deployed vanilla frontend and its declared lint/build/test commands are not presently a truthful validation gate.
- Cross-browser runs for Safari, Firefox, and Edge have not been automated yet.
- Hardware certification remains separate from CHR lab certification and must be run on supported physical RouterBOARD models.
