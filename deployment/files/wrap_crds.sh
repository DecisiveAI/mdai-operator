#!/usr/bin/env bash
set -euo pipefail

OS=$(uname -s)

if [[ "$OS" == "Darwin" ]]; then
  if ! command -v gsed >/dev/null 2>&1; then
    echo "❌ GNU sed (gsed) is required on macOS. Install it via: brew install gnu-sed" >&2
    exit 1
  fi
  SED="gsed"
else
  if command -v gsed >/dev/null 2>&1; then
    SED="gsed"
  else
    SED="sed"
  fi
fi

if ! grep -q '^crds:' deployment/values.yaml 2>/dev/null; then
  cat <<EOF >> deployment/values.yaml

crds:
  create: true
EOF
fi

for f in deployment/templates/*-crd.yaml; do
  grep -q '{{- if .Values.crds.create }}' "$f" && continue
  "$SED" -i '1s/^/{{- if .Values.crds.create }}\n/' "$f"
  echo -e '\n{{- end }}' >> "$f"
done
