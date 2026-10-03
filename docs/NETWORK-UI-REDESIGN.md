# Network workspace presentation

The network navigation distinguishes New network setup, Internet connections,
Customer internet, Wi-Fi networks, and LAN and Hotspot. Existing route IDs remain
unchanged. Each workspace has a distinct shared-system icon.

The network editor uses descriptive action cards with reported record counts,
a live-inventory context strip, grouped WAN/PPPoE service fields and a separate
review surface. Forms become single-column on narrow screens.

This is a frontend-only presentation change. Inventory loading remains read-only;
role checks, hardware rollout holds, preview confirmation, stale-review invalidation,
explicit apply consent and backup/verified readback are unchanged. It does not
enable Wi-Fi writes, alter routers or deploy paid-media changes.

Validation: `node scripts/test-network-workspaces.mjs` and
`node scripts/test-network-layout.mjs` (requires the existing jsdom dev dependency).
The layout regression covers action switching, field grouping, consent, stale
review invalidation and denied-write/physical-radio holds.
