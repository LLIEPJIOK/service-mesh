#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
VALUES_FILE="${ROOT_DIR}/k&s/manifest/monitoring/kube-prometheus-stack-values.kind.yaml"
RELEASE_NAME="${RELEASE_NAME:-mesh-monitoring}"

require_binary() {
  local bin="$1"
  if ! command -v "${bin}" >/dev/null 2>&1; then
    echo "Missing required binary: ${bin}"
    exit 1
  fi
}

require_binary helm

kubectl apply -f "${ROOT_DIR}/k&s/manifest/monitoring/00-monitoring-namespace.yaml"

helm upgrade --install "${RELEASE_NAME}" "${ROOT_DIR}/k&s/charts/kube-prometheus-stack" \
  --namespace monitoring \
  --values "${VALUES_FILE}" \
  --wait \
  --timeout 10m

kubectl apply -f "${ROOT_DIR}/k&s/manifest/monitoring/01-bookinfo-sidecar-podmonitor.yaml"
kubectl apply -f "${ROOT_DIR}/k&s/manifest/monitoring/02-grafana-ingress.yaml"
kubectl apply -f "${ROOT_DIR}/k&s/manifest/monitoring/03-grafana-dashboard-sidecar-apps.yaml"

echo "[monitoring] Grafana URL (Ingress): http://grafana.127.0.0.1.nip.io"
echo "[monitoring] Grafana credentials: admin/admin"
echo "[monitoring] Prometheus: kubectl port-forward -n monitoring svc/mesh-monitoring-prometheus 9090:9090"