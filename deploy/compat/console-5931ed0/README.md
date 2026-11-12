# Temporary console asset compatibility

These 66 public files are from release `5931ed011342ec1f9fa94f84024df42c371c9a0a`.
Every byte was compared with the deployed `/v2/assets/` file before capture.
The SHA-256 manifest is verified by both the Docker and Vercel builds.

The old entry HTML is deliberately not included. Existing tabs can continue loading their original
JS/CSS/font chunks; new page visits migrate to `/app/`. Missing asset paths return 404, not HTML.
Keep this snapshot through the URL migration. Remove it only in a deliberate later compatibility cleanup.

To reproduce: build `app/` at the recorded commit with its lockfile, then run
`node scripts/capture-legacy-console.mjs PREVIOUS_DIST deploy/compat/console-5931ed0`
while that release is still served. Capture fails if any byte differs. Do not replace it with a new build
under old filenames or put private recovery records in this directory.
