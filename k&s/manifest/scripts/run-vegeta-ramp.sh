#!/usr/bin/env bash

ROOT_DIR="/Users/denis/Projects/service-mesh"
SCRIPTS_DIR="${ROOT_DIR}/k&s/manifest/scripts"
ARTIFACTS_DIR="${ROOT_DIR}/k&s/test/artifacts/ramp"

MODE="${1:-}"

check_vegeta() {
  if ! command -v vegeta >/dev/null 2>&1; then
    echo "Missing required binary: vegeta"
    exit 1
  fi
}

check_kubectl() {
  if ! command -v kubectl >/dev/null 2>&1; then
    echo "Missing required binary: kubectl"
    exit 1
  fi
}

check_vegeta
check_kubectl

START_RATE=50
MAX_RATE=500
STEP=50
DURATION="20s"
TARGET_PATH="/productpage"

ERROR_THRESHOLD=1
LATENCY_THRESHOLD=10000
COOLDOWN=30

mkdir -p "${ARTIFACTS_DIR}"

TARGET_URL="http://localhost${TARGET_PATH}"

run_ramp_test() {
  local mode="$1"
  local run_dir="${ARTIFACTS_DIR}/${mode}"
  rm -rf "${run_dir}"
  mkdir -p "${run_dir}"

  echo "[ramp] Running ${mode} ramp test..."
  echo "[ramp] Start rate: ${START_RATE}, max: ${MAX_RATE}, step: ${STEP}"
  echo "[ramp] Stop conditions: error_rate > ${ERROR_THRESHOLD}% OR P95 > ${LATENCY_THRESHOLD}ms"
  echo ""

  local rate=${START_RATE}
  local results=()
  local should_stop=false

  while [[ ${rate} -le ${MAX_RATE} ]] && [[ "${should_stop}" == "false" ]]; do
    local test_dir="${run_dir}/${rate}rps"
    mkdir -p "${test_dir}"

    echo "[ramp] Testing ${rate} rps..."

    echo "GET ${TARGET_URL}" > "${test_dir}/targets.txt"

    vegeta attack \
      -targets "${test_dir}/targets.txt" \
      -rate "${rate}" \
      -duration "${DURATION}" \
      -name "${rate}rps" \
      -timeout 5s \
      > "${test_dir}/results.bin"

    vegeta report -type=json "${test_dir}/results.bin" > "${test_dir}/report.json"

    local throughput success p95 p99 mean error_rate
    throughput=$(jq -r '.throughput // 0' "${test_dir}/report.json")
    success=$(jq -r '.success // 1' "${test_dir}/report.json")
    error_rate=$(echo "scale=2; (1 - ${success}) * 100" | bc)
    p95=$(jq -r '.latencies["95th"] // 0' "${test_dir}/report.json" | awk '{printf "%.2f", $1/1000000}')
    p99=$(jq -r '.latencies["99th"] // 0' "${test_dir}/report.json" | awk '{printf "%.2f", $1/1000000}')
    mean=$(jq -r '.latencies["mean"] // 0' "${test_dir}/report.json" | awk '{printf "%.2f", $1/1000000}')

    cat > "${test_dir}/summary.json" <<EOF
{
  "rate": ${rate},
  "throughput": ${throughput},
  "success": ${success},
  "error_rate": ${error_rate},
  "p95_ms": ${p95},
  "p99_ms": ${p99},
  "mean_ms": ${mean}
}
EOF

    echo "  -> throughput=${throughput}, error_rate=${error_rate}%, P95=${p95}ms, P99=${p99}ms, mean=${mean}ms"

    results+=("${rate}:${throughput}:${error_rate}:${p95}:${p99}:${mean}")

    local error_check=$(echo "${error_rate} > ${ERROR_THRESHOLD}" | bc)
    local latency_check=$(echo "${p95} > ${LATENCY_THRESHOLD}" | bc)

    if [[ "${error_check}" == "1" ]] || [[ "${latency_check}" == "1" ]]; then
      echo "[ramp] Stop condition reached!"
      echo "[ramp]   error_rate (${error_rate}%) ${error_check} > ${ERROR_THRESHOLD}"
      echo "[ramp]   P95 latency (${p95}ms) ${latency_check} > ${LATENCY_THRESHOLD}ms"
      should_stop=true
    fi

    rate=$((rate + STEP))
    sleep ${COOLDOWN}
    echo ""
  done

  local rates="" throughputs="" error_rates="" p95s="" p99s="" means=""
  for r in "${results[@]}"; do
    IFS=':' read -r rate tp err p95 p99 mean <<< "${r}"
    rates="${rates}${rate},"
    throughputs="${throughputs}${tp},"
    error_rates="${error_rates}${err},"
    p95s="${p95s}${p95},"
    p99s="${p99s}${p99},"
    means="${means}${mean},"
  done

  rates="${rates%,}"
  throughputs="${throughputs%,}"
  error_rates="${error_rates%,}"
  p95s="${p95s%,}"
  p99s="${p99s%,}"
  means="${means%,}"

  cat > "${run_dir}/summary.json" <<EOF
{
  "mode": "${mode}",
  "start_rate": ${START_RATE},
  "step": ${STEP},
  "error_threshold": ${ERROR_THRESHOLD},
  "latency_threshold_ms": ${LATENCY_THRESHOLD},
  "rates": [${rates}],
  "throughputs": [${throughputs}],
  "error_rates": [${error_rates}],
  "p95_ms": [${p95s}],
  "p99_ms": [${p99s}],
  "mean_ms": [${means}]
}
EOF

  echo "[ramp] ${mode} complete. Results: ${run_dir}/summary.json"
}

case "${MODE}" in
  nosidecar|sidecar)
    run_ramp_test "${MODE}"
    ;;
  plot)
    echo "[ramp] Generating ramp plots..."
    ;;
  *)
    echo "Usage: $0 <nosidecar|sidecar|plot>"
    echo "  nosidecar - Run ramp test WITHOUT sidecar"
    echo "  sidecar   - Run ramp test WITH sidecar"
    echo "  plot      - Generate ramp plots"
    exit 1
    ;;
esac