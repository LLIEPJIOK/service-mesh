#!/usr/bin/env bash

ROOT_DIR="/Users/denis/Projects/service-mesh"
SCRIPTS_DIR="${ROOT_DIR}/k8s/manifest/scripts"
ARTIFACTS_DIR="${ROOT_DIR}/k8s/test/artifacts"
BENCH_SCRIPT="${SCRIPTS_DIR}/run-vegeta-bench.sh"
PLOT_SCRIPT="${SCRIPTS_DIR}/plot-vegeta-results.sh"

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

mkdir -p "${ARTIFACTS_DIR}"

case "${MODE}" in
  nosidecar)
    echo "[benchmark] Running benchmark WITHOUT sidecar..."
    bash "${BENCH_SCRIPT}" "nosidecar"
    ;;
  sidecar)
    echo "[benchmark] Running benchmark WITH sidecar..."
    bash "${BENCH_SCRIPT}" "sidecar"
    ;;
  plot)
    echo "[benchmark] Generating plots..."
    bash "${PLOT_SCRIPT}"
    ;;
  all|"")
    echo "[benchmark] Running FULL comparison (nosidecar -> sidecar -> plot)..."

    echo ""
    echo "=========================================="
    echo "Phase 1: Benchmark WITHOUT sidecar"
    echo "=========================================="
    make kind-down 2>/dev/null || true
    make kind-env-nosidecar
    bash "${BENCH_SCRIPT}" "nosidecar"

    echo ""
    echo "=========================================="
    echo "Phase 2: Benchmark WITH sidecar"
    echo "=========================================="
    make kind-env-sidecar
    bash "${BENCH_SCRIPT}" "sidecar"

    echo ""
    echo "=========================================="
    echo "Phase 3: Generating comparative plots"
    echo "=========================================="
    bash "${PLOT_SCRIPT}"

    echo ""
    echo "=========================================="
    echo "Benchmark complete!"
    echo "Results: ${ARTIFACTS_DIR}"
    echo "=========================================="
    ;;
  *)
    echo "Usage: $0 [nosidecar|sidecar|plot|all]"
    echo "  nosidecar - Run benchmark without sidecar"
    echo "  sidecar   - Run benchmark with sidecar"
    echo "  plot      - Generate plots from existing results"
    echo "  all       - Full pipeline (default)"
    exit 1
    ;;
esac