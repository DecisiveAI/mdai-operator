if ! grep -q '^crds:' deployment/values.yaml 2>/dev/null; then
  cat <<EOF >> deployment/values.yaml

crds:
  enabled: true
EOF
fi

for f in deployment/templates/*-crd.yaml; do
  grep -q '{{- if .Values.crds.create }}' "$f" && continue
  gsed -i '1s/^/{{- if .Values.crds.create }}\n/' "$f"
  echo -e '\n{{- end }}' >> "$f"
done
