# Live port diagram

The router page (Router tab, open a router) shows MikroTik's own back-panel picture of the model with each Ethernet port lit by what the router
reports: green (link up), grey (no link), red (disabled), dashed (the router did not report that port). Selecting a port shows its name, state, link rate,
duplex and live speed; each marker is a button with the same text as its label. The port list below stays as the text version.

## Which models
Only exact models listed in `app/src/lib/portPanels.ts` receive a diagram. A model without an entry shows the reported interface list; it never borrows another model's picture.
The reported RouterOS board identity takes precedence, including an unsupported identity. Only when that identity is absent can an exact saved model name select a diagram. Customer labels containing a model name do not qualify.

Coverage expanded on 2026-11-12 from two to fifteen diagrams:

| Model | Ethernet markers |
| --- | --- |
| hAP ax², hAP ax³ | 5 each (existing) |
| hAP ax lite | 4 |
| hAP ac³ | 5 |
| hEX S **RB760iGS** | 5 |
| L009UiGS-RM | 8 |
| RB4011iGS+RM | 10 |
| CCR2004-16G-2S+PC | 16 |
| CCR2116-12G-4S+ | 13 (includes ETH/BOOT) |
| CCR2216-1G-12XS-2XQ | 1 (excludes console) |
| CRS112-8P-4S-IN | 8 |
| CRS305-1G-4S+IN | 1 |
| CRS309-1G-8S+IN | 1 (excludes console) |
| CRS310-8G+2S+IN | 8 |
| CRS328-24P-4S+RM | 24 |

This is Ethernet diagram coverage, not a hardware compatibility certification. SFP/QSFP and renamed interfaces remain available in the reported text list; optical port overlays are not implemented. Missing or renamed default Ethernet names remain “Not reported” on the image. Dense diagrams scroll inside their card instead of compressing adjacent controls. Existing review/apply gates remain unchanged.

Similar model variants are deliberately excluded. In particular, the hEX S artwork is for RB760iGS, not newer E60iUGS hardware, so the ambiguous saved name “hEX S” alone does not select it. RB5009UG artwork is not substituted with the available RB5009UPr artwork. The other product photos do not by themselves establish a usable port view. Remaining catalogue entries retain the text fallback until suitable exact artwork and positions are checked.

### Verification — 2026-11-12

App lint, typecheck/production build and all 399 app tests passed (57 files). Focused diagram/image tests: 37 passed. Browser fixture rendered all 15 real components with explicitly synthetic states, loaded every image at its declared dimensions, and verified keyboard selection of last ports and no page overflow at 390 px. Desktop and mobile artwork were visually inspected. A temporary fixture page was removed before commit; no production demo route was added.

No router was contacted or changed. Physical acceptance is **BLOCKED**: owner reports the RouterBOARD cannot power on. Socket-to-live-interface correspondence, renamed interfaces, actual states and control actions require device evidence; none is certified by these UI tests. No merge or deployment is authorized.

## Adding a model
1. Take the model's back-panel picture (see source below). The files are large transparent PNGs wrapped in SVG: extract the embedded PNG, crop to the part that shows the ports (a closer crop makes the markers bigger to tap), resize to about 1000 px wide and save as WebP (`cwebp -q 82 -alpha_q 90`): 20 to 50 KB. Put it in `app/public/models/panel/<id>.webp`.
2. Add an entry to `portPanels` with the picture size, a board-name pattern, a marker size (percent of picture width) and each port's centre as percentages of the picture. Read the centres off a picture with a percentage grid, then check the markers on the picture in a browser.
3. The tests check that the picture exists, port names are unique and in range, and that other models do not get a panel.

## Source and licence
The pictures come from the "Visio and SVG files for homepage" share that MikroTik published, linked by [MikroTik Downloads → Drawings for VISIO](https://mikrotik.com/download/tools): [official share](https://box.mikrotik.com/d/9268c785266147a9b83c/). The additions use Ethernet routers, Wireless for home and office, CCR and CRS folders. Exact source paths, original SVG hashes, crop rectangles and output hashes are in `docs/PORT-DIAGRAM-SOURCES-20261112.json`. Images are bundled locally; no third-party request is required to render them. The share states no usage terms; the artwork belongs to MikroTik (see also the note in `mikrotikProducts.ts`).
