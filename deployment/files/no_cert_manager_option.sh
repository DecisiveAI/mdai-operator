#!/bin/bash

HELM_CHART_ROOT='deployment'
FILES_DIR="${HELM_CHART_ROOT}/files"
TEMPLATES_DIR="${HELM_CHART_ROOT}/templates"

WEBHOOK_TEMPLATE_NAME="^.*-webhook-configuration\.yaml$"
ISSUER_TEMPLATE_NAME="^.*-issuer\.yaml$"
CERT_TEMPLATE_NAME="^.*-certs?\.yaml$"

CRT_MGR_ON_HEADER="{{- if and (.Values.admissionWebhooks.create) (.Values.admissionWebhooks.certManager.enabled) }}"
CRT_MGR_FOOTER="{{- and }}"

CA_BUNDLE="caBundle: {{ $caCertEnc }}"
CERT_MGR_ANNOTATION="cert-manager.io/inject-ca-from"

CRT_MGR_OFF_HEADER=$(cat <<'EOF'
{{- if and (.Values.admissionWebhooks.create) (not .Values.admissionWebhooks.certManager.enabled) }}
{{- $cert := fromYaml (include "mdai-operator.WebhookCert" .) }}
{{- $caCertEnc := $cert.ca }}
{{- $certCrtEnc := $cert.crt }}
{{- $certKeyEnc := $cert.key }}
EOF
)

#  copy secrets template to templates
cp ${FILES_DIR}/cert_secret.yaml ${TEMPLATES_DIR}/cert-secret.yaml

#  add volumes.yaml addition
cat ${FILES_DIR}/no_cm_values.yaml >> ${HELM_CHART_ROOT}/values.yaml

# delete previously added block
awk '
/{{\/\* __addon_cut_line__ \*\/}}/ {
    if (!skip) { skip = 1; next }
    else { skip = 0; next }
}
!skip
' ${TEMPLATES_DIR}/_helpers.tpl > ${TEMPLATES_DIR}/.tmp_file
mv ${TEMPLATES_DIR}/.tmp_file ${TEMPLATES_DIR}/_helpers.tpl

#  add _helpers.tpl block
cat ${FILES_DIR}/no_cm_helpers.tpl >> ${TEMPLATES_DIR}/_helpers.tpl


# - creates copies for webhook templates with no-cert-manager conditional
for file in ${TEMPLATES_DIR}/*; do
  base=$(basename "$file")
  name="${base%.*}"
  if [[ "$base" =~ $WEBHOOK_TEMPLATE_NAME ]]; then
    {
      printf "%s\n" "$CRT_MGR_OFF_HEADER"
      cat "$file"
      printf "\n%s" "$CRT_MGR_FOOTER"
    } > ${TEMPLATES_DIR}/.tmp_file

    awk -v key="$CERT_MGR_ANNOTATION" '
    # Skip lines matching the annotation
    index($0, key ":") { next }

    # Match "service:" line and insert "caBundle" before it
    /^[[:space:]]*service:[[:space:]]*$/ {
        n = match($0, /[^[:space:]]/)
        indent = substr($0, 1, n - 1)
        print indent "caBundle: {{ $caCertEnc }}"
        print
        next
    }

    # Default case: print the line
    { print }
    ' ${TEMPLATES_DIR}/.tmp_file > "${TEMPLATES_DIR}/${name}-no-cm.yaml"
  fi
done

# - add cert-manager conditional header/footer to webhook, issuer, certs templates
for file in ${TEMPLATES_DIR}/*; do
  base=$(basename "$file")
  if [[ "$base" =~ $WEBHOOK_TEMPLATE_NAME || "$base" =~ $ISSUER_TEMPLATE_NAME || "$base" =~ $CERT_TEMPLATE_NAME ]]; then
    {
      printf "%s\n" "$CRT_MGR_ON_HEADER"
      cat "$file"
      printf "\n%s" "$CRT_MGR_FOOTER"
    } > ${TEMPLATES_DIR}/.tmp_file
    mv ${TEMPLATES_DIR}/.tmp_file "$file"
  fi
done