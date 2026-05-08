#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
SCRIPTS_DIR="${ROOT_DIR}/k8s/manifest/scripts"
ARTIFACTS_DIR="${ROOT_DIR}/k8s/test/artifacts"
MODE="${1:-}"

if [[ -z "${MODE}" ]]; then
  echo "Usage: $0 <nosidecar|sidecar>"
  exit 1
fi

require_binary() {
  local bin="$1"
  if ! command -v "${bin}" >/dev/null 2>&1; then
    echo "Missing required binary: ${bin}"
    exit 1
  fi
}

require_binary vegeta
require_binary jq

DURATION="${DURATION:-60s}"
RATE="${RATE:-100}"
WORKERS="${WORKERS:-20}"
TARGET_PATH="${TARGET_PATH:-/productpage}"

RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)"
MODE_DIR="${ARTIFACTS_DIR}/${MODE}"
mkdir -p "${MODE_DIR}"

INGRESS_HOST="127.0.0.1"
TARGET_URL="http://${INGRESS_HOST}${TARGET_PATH}"

echo "[bench] Mode: ${MODE}"
echo "[bench] Target via port-forward: http://localhost${TARGET_PATH}"
echo "[bench] Duration: ${DURATION}, Rate: ${RATE}, Workers: ${WORKERS}"

kubectl rollout status deployment/productpage-v1 -n bookinfo --timeout=120s || true
kubectl rollout status deployment/details-v1 -n bookinfo --timeout=120s || true
kubectl rollout status deployment/ratings-v1 -n bookinfo --timeout=120s || true
kubectl rollout status deployment/reviews-v1 -n bookinfo --timeout=120s || true
kubectl rollout status deployment/reviews-v2 -n bookinfo --timeout=120s || true
kubectl rollout status deployment/reviews-v3 -n bookinfo --timeout=120s || true

sleep 5

echo "GET http://localhost${TARGET_PATH}" > "${MODE_DIR}/targets.txt"

STARTED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

echo "[bench] Starting vegeta attack..."
vegeta attack \
  -targets "${MODE_DIR}/targets.txt" \
  -rate "${RATE}" \
  -duration "${DURATION}" \
  -workers "${WORKERS}" \
  -name "${MODE}" \
  -timeout 30s \
  > "${MODE_DIR}/results.bin"

ENDED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

echo "[bench] Generating reports..."
vegeta report -type=text "${MODE_DIR}/results.bin" > "${MODE_DIR}/report.txt"
vegeta report -type=json "${MODE_DIR}/results.bin" > "${MODE_DIR}/report.json"
vegeta report -type='hist[0,50ms,100ms,250ms,500ms,1s,2s,5s]' "${MODE_DIR}/results.bin" > "${MODE_DIR}/hist.txt"

cat > "${MODE_DIR}/meta.json" <<EOF
{
  "mode": "${MODE}",
  "started_at": "${STARTED_AT}",
  "ended_at": "${ENDED_AT}",
  "duration": "${DURATION}",
  "rate": "${RATE}",
  "workers": "${WORKERS}",
  "target": "http://localhost${TARGET_PATH}",
  "target_path": "${TARGET_PATH}"
}
EOF

THROUGHPUT=$(jq -r '.throughput // 0' "${MODE_DIR}/report.json")
SUCCESS=$(jq -r '.success // 0' "${MODE_DIR}/report.json")
P95_NS=$(jq -r '.latencies["95th"] // 0' "${MODE_DIR}/report.json")
P95_MS=$(awk -v ns="${P95_NS}" 'BEGIN { printf "%.2f", ns/1000000 }')
P99_NS=$(jq -r '.latencies["99th"] // 0' "${MODE_DIR}/report.json")
P99_MS=$(awk -v ns="${P99_NS}" 'BEGIN { printf "%.2f", ns/1000000 }')
MEAN_NS=$(jq -r '.latencies["mean"] // 0' "${MODE_DIR}/report.json")
MEAN_MS=$(awk -v ns="${MEAN_NS}" 'BEGIN { printf "%.2f", ns/1000000 }')

cat > "${MODE_DIR}/summary.json" <<EOF
{
  "mode": "${MODE}",
  "throughput": ${THROUGHPUT},
  "success_rate": ${SUCCESS},
  "latency_p50_ms": $(jq -r '.latencies["50th"] // 0' "${MODE_DIR}/report.json" | awk '{printf "%.2f", $1/1000000}'),
  "latency_p95_ms": ${P95_MS},
  "latency_p99_ms": ${P99_MS},
  "latency_mean_ms": ${MEAN_MS}
}
EOF

echo "[bench] Done. Results in ${MODE_DIR}/"
echo ""
echo "Summary:"
echo "  Throughput: ${THROUGHPUT} requests/sec"
echo "  Success:   ${SUCCESS}%"
echo "  P95:       ${P95_MS} ms"
echo "  P99:       ${P99_MS} ms"
echo "  Mean:      ${MEAN_MS} ms"