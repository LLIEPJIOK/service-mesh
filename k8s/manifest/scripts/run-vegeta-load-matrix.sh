#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
SCRIPTS_DIR="${ROOT_DIR}/k8s/manifest/scripts"
ARTIFACTS_DIR="${ROOT_DIR}/k8s/test/artifacts/load"
INSTALLER_DIR="${ROOT_DIR}/k8s/mesh/installer"
BOOKINFO_NAMESPACE="bookinfo"
MESH_NAMESPACE="mesh-system"
MONITORING_NAMESPACE="monitoring"
RELEASE_NAME="${RELEASE_NAME:-mesh-monitoring}"

MODES=(baseline mtls patterns full)
DURATION="${DURATION:-90s}"
RATE="${RATE:-120}"
WORKERS="${WORKERS:-40}"
MAX_WORKERS="${MAX_WORKERS:-200}"
TARGET_PATH="${TARGET_PATH:-/productpage}"

require_binary() {
  local bin="$1"
  if ! command -v "${bin}" >/dev/null 2>&1; then
    echo "Missing required binary: ${bin}"
    exit 1
  fi
}

require_binary kubectl
require_binary minikube
require_binary vegeta
require_binary jq

MINIKUBE_IP="$(minikube ip)"
TARGET_URL="http://${MINIKUBE_IP}:31380${TARGET_PATH}"
PROM_URL="http://${MINIKUBE_IP}:32001"
RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)"
RUN_DIR="${ARTIFACTS_DIR}/${RUN_ID}"
mkdir -p "${RUN_DIR}"

query_prometheus_range() {
  local query="$1"
  local start_ts="$2"
  local end_ts="$3"
  local step="$4"
  local out_file="$5"

  curl -fsS -G "${PROM_URL}/api/v1/query_range" \
    --data-urlencode "query=${query}" \
    --data-urlencode "start=${start_ts}" \
    --data-urlencode "end=${end_ts}" \
    --data-urlencode "step=${step}" > "${out_file}"
}

deploy_mode() {
  local mode="$1"

  kubectl delete namespace "${BOOKINFO_NAMESPACE}" --ignore-not-found
  if [[ "${mode}" == "baseline" ]]; then
    kubectl delete mutatingwebhookconfiguration mesh-sidecar-injector --ignore-not-found
    kubectl delete namespace "${MESH_NAMESPACE}" --ignore-not-found
    kubectl wait --for=delete namespace "${MESH_NAMESPACE}" --timeout=180s >/dev/null 2>&1 || true

    kubectl apply -k "${ROOT_DIR}/app/bookinfo/manifests"
    kubectl label namespace "${BOOKINFO_NAMESPACE}" mesh-injection- --overwrite >/dev/null 2>&1 || true
    return
  fi

  "${SCRIPTS_DIR}/build-and-load-mesh-images-minikube.sh"
  "${SCRIPTS_DIR}/generate-mesh-config-mode.sh" "${mode}"

  pushd "${INSTALLER_DIR}" >/dev/null
  go run ./cmd/mesh install -f "../../manifest/generated/mesh-config.${mode}.yaml" --wait --timeout 120s
  popd >/dev/null

  "${SCRIPTS_DIR}/deploy-bookinfo-minikube.sh"

  if [[ "${mode}" == "patterns" || "${mode}" == "full" ]]; then
    "${SCRIPTS_DIR}/install-monitoring-minikube.sh"
  fi
}

collect_kube_evidence() {
  local out_dir="$1"

  kubectl get pods -A -o wide > "${out_dir}/pods.txt" || true
  kubectl get events -A --sort-by=.lastTimestamp > "${out_dir}/events.txt" || true
  kubectl get events -A --field-selector reason=Unhealthy --sort-by=.lastTimestamp > "${out_dir}/events-unhealthy.txt" || true
  kubectl get pods -n "${BOOKINFO_NAMESPACE}" -o json > "${out_dir}/bookinfo-pods.json" || true
  kubectl describe pods -n "${BOOKINFO_NAMESPACE}" > "${out_dir}/bookinfo-pods.describe.txt" || true

  kubectl logs -n "${MESH_NAMESPACE}" deployment/mesh-webhook --tail=400 > "${out_dir}/mesh-webhook.log" 2>/dev/null || true
  kubectl logs -n "${MESH_NAMESPACE}" deployment/mesh-cert-manager --tail=400 > "${out_dir}/mesh-cert-manager.log" 2>/dev/null || true

  local sidecar_pod
  sidecar_pod="$(kubectl get pods -n "${BOOKINFO_NAMESPACE}" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
  if [[ -n "${sidecar_pod}" ]]; then
    kubectl logs -n "${BOOKINFO_NAMESPACE}" "${sidecar_pod}" -c sidecar --tail=400 > "${out_dir}/sidecar.log" 2>/dev/null || true
  fi
}

run_mode_attack() {
  local mode="$1"
  local mode_dir="${RUN_DIR}/${mode}"
  mkdir -p "${mode_dir}"

  echo "[load] Deploy mode=${mode}"
  deploy_mode "${mode}"

  kubectl rollout status deployment/productpage-v1 -n "${BOOKINFO_NAMESPACE}" --timeout=180s
  kubectl rollout status deployment/details-v1 -n "${BOOKINFO_NAMESPACE}" --timeout=180s
  kubectl rollout status deployment/ratings-v1 -n "${BOOKINFO_NAMESPACE}" --timeout=180s
  kubectl rollout status deployment/reviews-v1 -n "${BOOKINFO_NAMESPACE}" --timeout=180s

  echo "GET ${TARGET_URL}" > "${mode_dir}/targets.txt"

  local started_at ended_at
  started_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

  vegeta attack \
    -targets "${mode_dir}/targets.txt" \
    -rate "${RATE}" \
    -duration "${DURATION}" \
    -workers "${WORKERS}" \
    -max-workers "${MAX_WORKERS}" \
    -name "${mode}" \
    -timeout 30s \
    > "${mode_dir}/results.bin"

  ended_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

  vegeta report -type=text "${mode_dir}/results.bin" > "${mode_dir}/report.txt"
  vegeta report -type=json "${mode_dir}/results.bin" > "${mode_dir}/report.json"
  vegeta report -type='hist[0,50ms,100ms,250ms,500ms,1s,2s]' "${mode_dir}/results.bin" > "${mode_dir}/hist.txt"
  vegeta plot -title "Mode ${mode}: ${RATE} rps for ${DURATION}" "${mode_dir}/results.bin" > "${mode_dir}/latency-plot.html"

  local start_ts end_ts
  start_ts="$(date -j -f "%Y-%m-%dT%H:%M:%SZ" "${started_at}" +%s)"
  end_ts="$(date -j -f "%Y-%m-%dT%H:%M:%SZ" "${ended_at}" +%s)"

  if [[ "${mode}" == "patterns" || "${mode}" == "full" ]]; then
    query_prometheus_range 'sum(rate(mesh_requests_total[1m]))' "${start_ts}" "${end_ts}" "15s" "${mode_dir}/prom_rps.json"
    query_prometheus_range 'histogram_quantile(0.95, sum by (le) (rate(mesh_request_duration_seconds_bucket[1m])))' "${start_ts}" "${end_ts}" "15s" "${mode_dir}/prom_p95.json"
    query_prometheus_range '100 * sum(rate(mesh_request_errors_total[1m])) / clamp_min(sum(rate(mesh_requests_total[1m])),0.001)' "${start_ts}" "${end_ts}" "15s" "${mode_dir}/prom_error_rate.json"
    query_prometheus_range 'sum(rate(mesh_retry_attempts_total[1m]))' "${start_ts}" "${end_ts}" "15s" "${mode_dir}/prom_retries.json"
    query_prometheus_range 'max(mesh_circuit_breaker_state)' "${start_ts}" "${end_ts}" "15s" "${mode_dir}/prom_circuit_breaker_state.json"
  fi

  collect_kube_evidence "${mode_dir}"

  jq '[.items[]?.status.containerStatuses[]? | .restartCount] | add // 0' \
    "${mode_dir}/bookinfo-pods.json" > "${mode_dir}/pod_restarts_total.txt" 2>/dev/null || echo 0 > "${mode_dir}/pod_restarts_total.txt"

  jq -n \
    --arg mode "${mode}" \
    --arg started_at "${started_at}" \
    --arg ended_at "${ended_at}" \
    --arg duration "${DURATION}" \
    --arg rate "${RATE}" \
    --arg workers "${WORKERS}" \
    --arg max_workers "${MAX_WORKERS}" \
    --arg target "${TARGET_URL}" \
    '{mode:$mode,started_at:$started_at,ended_at:$ended_at,duration:$duration,rate:$rate,workers:$workers,max_workers:$max_workers,target:$target}' > "${mode_dir}/run-meta.json"
}

echo "[load] Starting run ${RUN_ID}"
echo "[load] Target: ${TARGET_URL}"

echo "[load] Installing monitoring prerequisites"
"${SCRIPTS_DIR}/install-monitoring-minikube.sh" || true

for mode in "${MODES[@]}"; do
  run_mode_attack "${mode}"
done

SUMMARY_FILE="${RUN_DIR}/summary.md"
{
  echo "# Load Test Summary (${RUN_ID})"
  echo
  echo "| mode | throughput | success | p95 latency (ms) | pod restarts |"
  echo "| --- | --- | --- | --- | --- |"

  for mode in "${MODES[@]}"; do
    mode_dir="${RUN_DIR}/${mode}"
    throughput="$(jq -r '.throughput' "${mode_dir}/report.json")"
    success="$(jq -r '.success' "${mode_dir}/report.json")"
    p95_ns="$(jq -r '.latencies["95th"]' "${mode_dir}/report.json")"
    p95_ms="$(awk -v ns="${p95_ns}" 'BEGIN { printf "%.2f", ns/1000000 }')"
    restarts="$(cat "${mode_dir}/pod_restarts_total.txt")"
    echo "| ${mode} | ${throughput} | ${success} | ${p95_ms} | ${restarts} |"
  done
} > "${SUMMARY_FILE}"

echo "[load] Complete"
echo "[load] Artifacts: ${RUN_DIR}"
echo "[load] Summary: ${SUMMARY_FILE}"
