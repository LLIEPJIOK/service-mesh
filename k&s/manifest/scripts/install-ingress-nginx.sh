#!/usr/bin/env bash
set -euo pipefail

INGRESS_NAMESPACE="ingress-nginx"
INGRESS_APP="ingress-nginx"

require_binary() {
  local bin="$1"
  if ! command -v "${bin}" >/dev/null 2>&1; then
    echo "Missing required binary: ${bin}"
    exit 1
  fi
}

require_binary kubectl

INGRESS_YAML_URL="https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/kind/deploy.yaml"

echo "[ingress] Downloading ingress-nginx manifest from ${INGRESS_YAML_URL}..."
if ! kubectl apply -f "${INGRESS_YAML_URL}"; then
  echo "[ingress] Failed to apply ingress, trying alternative method..."
  kubectl apply -f "https://raw.githubusercontent.com/kubernetes/ingress-nginx/refs/heads/main/deploy/static/provider/kind/deploy.yaml"
fi

kubectl wait --namespace "${INGRESS_NAMESPACE}" \
  --for=condition=ready pod \
  --selector=app.kubernetes.io/component=controller \
  --timeout=120s

echo "[ingress] NGINX Ingress ready at http://127.0.0.1"