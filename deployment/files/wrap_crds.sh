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

"$SED" -i '
  /^crds:/q            # If crds: exists, quit with no changes
  $a\
\
crds:\
  create: true
' deployment/values.yaml

for f in deployment/templates/*-crd.yaml; do
  "$SED" -i '
    0,/{{- if .Values.crds.create }}/{
      /{{- if .Values.crds.create }}/q
    }
    1s/^/{{- if .Values.crds.create }}\
&/
    $a\
{{- end }}
  ' "$f"
done
