#!/usr/bin/env bash
# Run on mikromagic-app. Read the replacement key from the terminal without
# echo; never pass it through command arguments, chat, Git or shell history.
set +x
set -euo pipefail
umask 077

if [[ ! -t 0 ]]; then
  echo "Run this script interactively in the VPS terminal." >&2
  exit 2
fi
cd /home/ubuntu/mega-magic-project
read -r -s -p "Replacement Gemini API key (hidden): " magic_dude_key
printf '\n'
if [[ ! "$magic_dude_key" =~ ^[A-Za-z0-9_.-]{20,256}$ ]]; then
  echo "Key format is invalid; nothing changed." >&2
  exit 2
fi
read -r -p "Gemini model ID [gemini-3.8-flash]: " magic_dude_model
magic_dude_model="${magic_dude_model:-gemini-3.8-flash}"
if [[ ! "$magic_dude_model" =~ ^gemini-[A-Za-z0-9.-]+$ ]]; then
  echo "Model ID is invalid; nothing changed." >&2
  exit 2
fi

# Header comes from stdin so the API key is absent from the process list.
# GET model checks credentials/model visibility without a generation charge.
if ! printf 'silent\nfail\nconnect-timeout = 10\nmax-time = 30\nheader = "x-goog-api-key: %s"\nurl = "https://generativelanguage.googleapis.com/v1beta/models/%s"\n' "$magic_dude_key" "$magic_dude_model" | curl --config - >/dev/null; then
  echo "Google could not validate this key/model. Check API access and model availability; nothing changed." >&2
  exit 1
fi

magic_dude_setup_dir="$(mktemp -d /tmp/mm-gemini-setup.XXXXXX)"
trap 'unset magic_dude_key; rm -f "$magic_dude_setup_dir/provider.env" "$magic_dude_setup_dir/service.conf"; rmdir "$magic_dude_setup_dir"' EXIT
printf 'AI_MAGIC_DUDE_PROVIDER=gemini\nAI_GEMINI_API_KEY=%s\nAI_GEMINI_MODEL=%s\n' "$magic_dude_key" "$magic_dude_model" >"$magic_dude_setup_dir/provider.env"

# Validate the installed backend's configuration before changing the service.
AI_MAGIC_DUDE_PROVIDER=gemini AI_GEMINI_API_KEY="$magic_dude_key" AI_GEMINI_MODEL="$magic_dude_model" ENVIRONMENT=production /usr/bin/go run ./cmd/config-check --config config.yaml
unset magic_dude_key
sudo install -d -o root -g root -m 700 /etc/mega-magic
if sudo test -f /etc/mega-magic/magic-dude.env; then
  sudo cp -p /etc/mega-magic/magic-dude.env /etc/mega-magic/magic-dude.env.previous
fi
sudo install -o root -g root -m 600 "$magic_dude_setup_dir/provider.env" /etc/mega-magic/magic-dude.env
printf '[Service]\nEnvironmentFile=/etc/mega-magic/magic-dude.env\n' >"$magic_dude_setup_dir/service.conf"
sudo install -d -o root -g root -m 755 /etc/systemd/system/mega-magic-api.service.d
sudo install -o root -g root -m 644 "$magic_dude_setup_dir/service.conf" /etc/systemd/system/mega-magic-api.service.d/85-magic-dude.conf
sudo systemctl daemon-reload
sudo systemctl restart mega-magic-api
if ! sudo systemctl is-active --quiet mega-magic-api; then
  echo "API restart failed. Inspect service logs; the protected configuration is at /etc/mega-magic/magic-dude.env." >&2
  exit 1
fi
echo "Magic Dude now uses Gemini. Open the webapp and send a test question."
echo "Model access was checked; generation depends on your Google project quota/billing."
