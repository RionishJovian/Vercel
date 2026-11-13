# MegaMagic logo — "Wi-Fi Spell · Glass" (proposal, 2026-11-13)

This is a vector rebuild of the owner's Gemini render (`gemini-reference.webp`), which was itself based on the "Wi-Fi Spell" concept. It keeps Gemini's look:
- a clear dark glass tile with a thick bevelled edge and a soft gold and mint floor glow;
- a chrome wand that reaches past the tile's lower-left corner;
- the three Wi-Fi arcs as glowing tubes in gold, lime and mint, each with a shadow edge, a body and a hot core;
- a cloud of gold dust fanning from the star;
- faint circuit traces in the glass;
- the wand's star tip standing in for the Wi-Fi dot.

Unlike the reference render, the vector has:
- a transparent background (no baked-in backdrop);
- no lossy compression;
- sharp output at every size, including the 16 px browser tab.

**Status: awaiting owner approval.** The app, web pages, icons and manifest still use the current logo.

Files:
- `megamagic-logo.svg`: vector master (512 × 512 viewBox).
- `megamagic-logo-512.png`, `-180.png`, `-64.png`: renders of the master with a transparent background.
- `megamagic-logo-review.png`: review sheet comparing the current logo, the Gemini reference and the vector.
- `gemini-reference.webp`: the owner's reference image.
- `make-logo.cjs`: the script that wrote the SVG. The dust is drawn from a fixed random seed, so the output is reproducible. The script is kept as a record only.
- `SHA256SUMS`: checksums of the generated files.

Once adopted, ship these exact files and check them against the sums. Do not regenerate or retouch them.
