# Live port diagram

The router page (Router tab, open a router) shows MikroTik's own back-panel picture of the model with each Ethernet port lit by what the router
reports: green (link up), grey (no link), red (disabled), dashed (the router did not report that port). Selecting a port shows its name, state, link rate,
duplex and live speed; each marker is a button with the same text as its label. The port list below stays as the text version.

## Which models
Only models listed in `app/src/lib/portPanels.ts`: hAP ax³ (`C53UiG…`) and hAP ax² (`C52iG…`). A model without an entry shows no diagram; it never borrows another
model's picture. The router is identified by its RouterOS board name first, and by the owner's typed name only on an exact model match.

## Adding a model
1. Take the model's back-panel picture (see source below). The files are large transparent PNGs wrapped in SVG: extract the embedded PNG, crop to the part that shows the ports (a closer crop makes the markers bigger to tap), resize to about 1000 px wide and save as WebP (`cwebp -q 82 -alpha_q 90`): 20 to 50 KB. Put it in `app/public/models/panel/<id>.webp`.
2. Add an entry to `portPanels` with the picture size, a board-name pattern, a marker size (percent of picture width) and each port's centre as percentages of the picture. Read the centres off a picture with a percentage grid, then check the markers on the picture in a browser.
3. The tests check that the picture exists, port names are unique and in range, and that other models do not get a panel.

## Source and licence
The pictures come from the "Visio and SVG files for homepage" share that MikroTik published (box.mikrotik.com), folders "Ethernet routers" and "Wireless for home and office". The share states no usage terms; the artwork belongs to MikroTik (see also the note in `mikrotikProducts.ts`).
