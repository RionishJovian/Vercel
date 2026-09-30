#!/usr/bin/env bash
# Securely configure the single operator Telegram bot on the VPS. The bot token
# is read without echo and never appears in argv, shell history, Git, or output.
set -euo pipefail

if [[ ! -t 0 ]]; then
  echo "Run this script interactively from the VPS terminal." >&2
  exit 2
fi

read -r -s -p "New Telegram bot token: " bot_token
echo
read -r -p "Allowed operator chat ID (numeric): " chat_id

if [[ ! "$bot_token" =~ ^[0-9]+:[A-Za-z0-9_-]{20,}$ ]]; then
  echo "Token format is invalid." >&2
  exit 2
fi
if [[ ! "$chat_id" =~ ^-?[0-9]+$ ]] || [[ "$chat_id" == "0" ]]; then
  echo "Chat ID must be a non-zero integer." >&2
  exit 2
fi

# Validate through stdin-fed curl configuration so the token is not exposed in
# the process list. Do not print Telegram's response because it contains bot
# identity details that are unnecessary for this setup.
if ! printf 'silent\nshow-error\nfail\nurl = "https://api.telegram.org/bot%s/getMe"\n' "$bot_token" | curl --config - >/dev/null; then
  echo "Telegram rejected the token; nothing was changed." >&2
  exit 1
fi

secret_tmp="$(mktemp)"
trap 'rm -f "$secret_tmp"' EXIT
chmod 600 "$secret_tmp"
printf 'BILLING_TELEGRAM_ENABLED=true\nBILLING_TELEGRAM_BOT_TOKEN=%s\nBILLING_TELEGRAM_CHAT_ID=%s\n' "$bot_token" "$chat_id" >"$secret_tmp"
sudo install -d -o root -g root -m 700 /etc/mega-magic
sudo install -o root -g root -m 600 "$secret_tmp" /etc/mega-magic/billing-telegram.env

dropin_tmp="$(mktemp)"
trap 'rm -f "$secret_tmp" "$dropin_tmp"' EXIT
printf '[Service]\nEnvironmentFile=-/etc/mega-magic/billing-telegram.env\n' >"$dropin_tmp"
sudo install -d -o root -g root -m 755 /etc/systemd/system/mega-magic-api.service.d
sudo install -o root -g root -m 644 "$dropin_tmp" /etc/systemd/system/mega-magic-api.service.d/80-billing-telegram.conf
sudo systemctl daemon-reload
sudo systemctl restart mega-magic-api
sudo systemctl is-active --quiet mega-magic-api
echo "Telegram Store approvals enabled for the allowed operator chat."
