#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
MANIFEST_DIR="${ROOT_DIR}/k&s/app/bookinfo/manifests"

kubectl apply -k "${MANIFEST_DIR}"

kubectl rollout status deployment/productpage-v1 -n bookinfo --timeout=180s
kubectl rollout status deployment/details-v1 -n bookinfo --timeout=180s
kubectl rollout status deployment/ratings-v1 -n bookinfo --timeout=180s
kubectl rollout status deployment/reviews-v1 -n bookinfo --timeout=180s
kubectl rollout status deployment/reviews-v2 -n bookinfo --timeout=180s
kubectl rollout status deployment/reviews-v3 -n bookinfo --timeout=180s

echo "[bookinfo] Bookinfo deployed (no mesh)"
echo "[bookinfo] Application URL (Ingress): http://127.0.0.1/productpage"