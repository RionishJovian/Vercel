#!/bin/sh
set -eu

required="MM_CERT_DSN MM_CERT_CRED_KEY MM_CERT_BUSINESS MM_CERT_DEVICE MM_CERT_LAB_CIDR MM_CERT_ROTATE_PORT"
missing=""
for name in $required; do
  eval "value=\${$name-}"
  if [ -z "$value" ]; then
    missing="$missing $name"
  fi
done

if [ -n "$missing" ]; then
  echo "hardware certification refused: missing:$missing" >&2
  echo "see docs/HARDWARE-CERTIFICATION.md; credentials must stay in environment variables or secret files" >&2
  exit 2
fi

exec go test -count=1 -v ./internal/provisioner/routeros \
  -run '^TestHardwareCertification_ProductionPathOnDisposableRouter$'
