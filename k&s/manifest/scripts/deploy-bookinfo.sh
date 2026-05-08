#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
MANIFEST_DIR="${ROOT_DIR}/k&s/app/bookinfo/manifests"

kubectl apply -k "${MANIFEST_DIR}"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT

kubectl get secret mesh-root-ca -n mesh-system -o jsonpath='{.data.tls\.crt}' | base64 --decode > "${tmp_dir}/ca.crt"
kubectl get secret mesh-root-ca -n mesh-system -o jsonpath='{.data.tls\.key}' | base64 --decode > "${tmp_dir}/ca.key"

kubectl create secret tls mesh-root-ca \
  -n bookinfo \
  --cert="${tmp_dir}/ca.crt" \
  --key="${tmp_dir}/ca.key" \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl rollout status deployment/productpage-v1 -n bookinfo --timeout=180s
kubectl rollout status deployment/details-v1 -n bookinfo --timeout=180s
kubectl rollout status deployment/ratings-v1 -n bookinfo --timeout=180s
kubectl rollout status deployment/reviews-v1 -n bookinfo --timeout=180s
kubectl rollout status deployment/reviews-v2 -n bookinfo --timeout=180s
kubectl rollout status deployment/reviews-v3 -n bookinfo --timeout=180s

echo "[bookinfo] Bookinfo deployed"
echo "[bookinfo] Application URL (Ingress): http://127.0.0.1/productpage"