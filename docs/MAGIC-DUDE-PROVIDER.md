# Magic Dude provider and key setup

Magic Dude supports OpenRouter and Google Gemini. Its provider setting is
independent of the automated fleet scanner's `ai.mode`. Provider credentials are
platform configuration; customers still receive only their own tenant context.
The key is never sent to their browsers.

For Gemini, create a replacement key in Google AI Studio if the previous key was
shared in a screenshot or transcript. Enter it directly in an interactive VPS
terminal:

```sh
ssh mikromagic-app
cd /home/ubuntu/mega-magic-project
bash scripts/configure-magic-dude-gemini.sh
```

Paste the replacement key at the hidden prompt, press Enter, then choose the
model ID. The script checks Google's model endpoint, validates configuration,
stores the key in root-owned mode-600 `/etc/mega-magic/magic-dude.env`, adds a
systemd environment-file reference and restarts `mega-magic-api`. Existing
protected provider configuration is retained as `.previous` for recovery.
Do not paste the key into chat, commit it, or enter it as a command argument.

Settings are `AI_MAGIC_DUDE_PROVIDER=gemini`, `AI_GEMINI_API_KEY` (alias:
`GEMINI_API_KEY`) and `AI_GEMINI_MODEL`. Default model: `gemini-3.8-flash`.
Google project quota and billing determine availability and charges; selecting
Gemini does not guarantee free usage. The model endpoint check makes no generation
request. After activation, sign in and ask Magic Dude a test question; verify the
answer/model and its token usage under AI usage. A fresh browser reload must work.

If using OpenRouter, set `AI_MAGIC_DUDE_PROVIDER=openrouter` and keep its protected
`AI_OPENROUTER_API_KEY` and configured free-model ID. Do not put a Google key in
the OpenRouter field. Provider switching does not modify router configuration.

## Limits and diagnosis

One account may make 100 provider calls in 24 hours (`MagicDudeDailyLimit`, kept
in Redis; if Redis is unavailable the call is allowed and the failure is logged).
Stored directions for common "where is..." questions use no provider call and do
not count. Provider failures log `magic dude: provider failed`, with the account,
elapsed time and a status or fixed reason, never the question or prompt:

```sh
docker logs mega-magic-go-restored-api-1 2>&1 | grep 'magic dude:'
```

Device status in answers comes from the Hub's live tunnel state, read for up to 10
gateways within 4 seconds. Anything not read is reported as unknown. Page names in
the prompt are checked against the console by `TestMagicDudePagesMatchTheApp`.

Reference: [Google Gemini API](https://ai.google.dev/api).
