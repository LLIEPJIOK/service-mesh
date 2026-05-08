#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
VALUES_FILE="${ROOT_DIR}/k&s/manifest/monitoring/kube-prometheus-stack-values.kind.yaml"
CHART_DIR="${ROOT_DIR}/k&s/charts/kube-prometheus-stack"

BOOKINFO_IMAGES=(
  "istio/examples-bookinfo-productpage-v1:1.20.3"
  "istio/examples-bookinfo-details-v1:1.20.3"
  "istio/examples-bookinfo-ratings-v1:1.20.3"
  "istio/examples-bookinfo-reviews-v1:1.20.3"
  "istio/examples-bookinfo-reviews-v2:1.20.3"
  "istio/examples-bookinfo-reviews-v3:1.20.3"
)

extract_monitoring_images() {
  local chart="$1"
  local values="$2"

  if [[ ! -d "${chart}" ]]; then
    echo "ERROR: chart dir ${chart} not found" >&2
    return 1
  fi

  helm template mesh-monitoring "${chart}" \
    --namespace monitoring \
    --values "${values}" \
    2>/dev/null | grep 'image:' | sed 's/.*image:[ ]*//' | tr -d '"' | sort -u
}

pull_image() {
  local image="$1"
  if docker image inspect "${image}" >/dev/null 2>&1; then
    echo "[preload] Already cached: ${image}"
    return 0
  fi
  echo "[preload] Pulling: ${image}"
  if docker pull "${image}"; then
    echo "[preload] -> success"
    return 0
  else
    echo "[preload] -> FAILED"
    return 1
  fi
}

pull_images() {
  local images="$1"
  local failed=0
  for img in ${images}; do
    if ! pull_image "${img}"; then
      failed=$((failed + 1))
    fi
  done
  return ${failed}
}

usage() {
  cat <<EOF
Usage: $0 [--bookinfo|--monitoring|--all]

Pulls images from registries into local Docker cache.
This is a ONE-TIME operation - subsequent runs will skip already-cached images.

Examples:
  $0 --all           # Pull all required images
  $0 --bookinfo       # Pull only Bookinfo images
  $0 --monitoring     # Pull only monitoring stack images
EOF
}

MODE="${1:-all}"

case "${MODE}" in
  --bookinfo|bookinfo)
    echo "[preload] Pulling Bookinfo images..."
    pull_images "${BOOKINFO_IMAGES[*]}"
    ;;
  --monitoring|monitoring)
    echo "[preload] Extracting monitoring images from helm chart..."
    MON_IMAGES=$(extract_monitoring_images "${CHART_DIR}" "${VALUES_FILE}")
    echo "[preload] Monitoring images to pull:"
    echo "${MON_IMAGES}" | while read -r img; do
      echo "  ${img}"
    done
    echo ""
    pull_images "${MON_IMAGES}"
    ;;
  --all|all)
    echo "[preload] Pulling Bookinfo images..."
    pull_images "${BOOKINFO_IMAGES[*]}"
    echo ""
    echo "[preload] Extracting monitoring images from helm chart..."
    MON_IMAGES=$(extract_monitoring_images "${CHART_DIR}" "${VALUES_FILE}")
    echo "[preload] Monitoring images to pull:"
    echo ""
    echo "[preload] Pulling monitoring stack images..."
    pull_images "${MON_IMAGES}"
    ;;
  *)
    usage
    exit 1
    ;;
esac