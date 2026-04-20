#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
MANIFEST_DIR="${ROOT_DIR}/app/bookinfo/manifests"

minikube addons enable ingress >/dev/null
kubectl rollout status deployment/ingress-nginx-controller -n ingress-nginx --timeout=120s

kubectl apply -k "${MANIFEST_DIR}"

# Sidecar mounts mesh-root-ca from workload namespace, so copy root CA there.
tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT

kubectl get secret mesh-root-ca -n mesh-system -o jsonpath='{.data.tls\.crt}' | base64 --decode > "${tmp_dir}/ca.crt"
kubectl get secret mesh-root-ca -n mesh-system -o jsonpath='{.data.tls\.key}' | base64 --decode > "${tmp_dir}/ca.key"

kubectl create secret tls mesh-root-ca \
	-n bookinfo \
	--cert="${tmp_dir}/ca.crt" \
	--key="${tmp_dir}/ca.key" \
	--dry-run=client -o yaml | kubectl apply -f -

kubectl rollout status deployment/productpage-v1 -n bookinfo --timeout=10s
kubectl rollout status deployment/details-v1 -n bookinfo --timeout=10s
kubectl rollout status deployment/ratings-v1 -n bookinfo --timeout=10s
kubectl rollout status deployment/reviews-v1 -n bookinfo --timeout=10s
kubectl rollout status deployment/reviews-v2 -n bookinfo --timeout=10s
kubectl rollout status deployment/reviews-v3 -n bookinfo --timeout=10s

MINIKUBE_IP="$(minikube ip)"
echo "[bookinfo] Ingress URL (with minikube tunnel): http://127.0.0.1/productpage"
echo "[bookinfo] Start tunnel in separate terminal if needed: minikube tunnel"
echo "[bookinfo] App URL (NodePort fallback): http://${MINIKUBE_IP}:31380/productpage"
