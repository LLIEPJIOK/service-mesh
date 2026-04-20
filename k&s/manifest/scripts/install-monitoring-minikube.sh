#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
VALUES_FILE="${ROOT_DIR}/manifest/monitoring/kube-prometheus-stack-values.yaml"
RELEASE_NAME="${RELEASE_NAME:-mesh-monitoring}"

kubectl apply -f "${ROOT_DIR}/manifest/monitoring/00-monitoring-namespace.yaml"

helm upgrade --install "${RELEASE_NAME}" oci://ghcr.io/prometheus-community/charts/kube-prometheus-stack \
  --namespace monitoring \
  --values "${VALUES_FILE}" \
  --wait \
  --timeout 10m

kubectl apply -f "${ROOT_DIR}/manifest/monitoring/01-bookinfo-sidecar-podmonitor.yaml"
kubectl apply -f "${ROOT_DIR}/manifest/monitoring/02-grafana-ingress.yaml"
kubectl apply -f "${ROOT_DIR}/manifest/monitoring/03-grafana-dashboard-sidecar-apps.yaml"

echo "[monitoring] Grafana URL (Ingress, with minikube tunnel): http://grafana.127.0.0.1.nip.io"
echo "[monitoring] Start tunnel in separate terminal if needed: minikube tunnel"
echo "[monitoring] Grafana URL (NodePort fallback): http://$(minikube ip):32000"
echo "[monitoring] Prometheus URL: http://$(minikube ip):32001"
echo "[monitoring] Grafana credentials: admin/admin"
