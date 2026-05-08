#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-mesh-demo}"
VALUES_FILE="${ROOT_DIR}/k8s/manifest/monitoring/kube-prometheus-stack-values.kind.yaml"
CHART_DIR="${ROOT_DIR}/k8s/charts/kube-prometheus-stack"

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

  helm template mesh-monitoring "${chart}" \
    --namespace monitoring \
    --values "${values}" \
    2>/dev/null | grep 'image:' | sed 's/.*image:[ ]*//' | tr -d '"' | sort -u
}

require_binary() {
  local bin="$1"
  if ! command -v "${bin}" >/dev/null 2>&1; then
    echo "Missing required binary: ${bin}"
    exit 1
  fi
}

require_binary docker
require_binary kind
require_binary kubectl

check_kind_cluster() {
  local cluster_name="${KIND_CLUSTER_NAME:-mesh-demo}"
  if ! kind get clusters 2>/dev/null | grep -q "^${cluster_name}$"; then
    echo "ERROR: Kind cluster '${cluster_name}' does not exist"
    echo "Run create-cluster.sh first"
    exit 1
  fi
}

ensure_image_local() {
  local image="$1"
  if ! docker image inspect "${image}" >/dev/null 2>&1; then
    echo "MISSING: ${image}"
    echo "  Run 'make kind-images-preload' first to pull images into local Docker"
    return 1
  fi
}

image_loaded_in_kind() {
  local image="$1"
  local node_name="mesh-demo-control-plane"
  local img_name="${image#docker.io/}"
  img_name="${img_name#registry.k8s.io/}"
  img_name="${img_name#quay.io/}"
  img_name="${img_name#ghcr.io/}"

  docker exec "${node_name}" ctr -n k8s.io images ls 2>/dev/null | grep -q "${img_name}"
}

load_image() {
  local image="$1"
  local node_name="mesh-demo-control-plane"

  if image_loaded_in_kind "${image}"; then
    echo "[load-images] ${image} already in cluster, skipping"
    return 0
  fi

  echo "[load-images] Loading ${image}..."
  if docker save "${image}" 2>/dev/null | docker exec -i "${node_name}" ctr -n k8s.io images import - 2>/dev/null; then
    echo "  -> loaded"
    return 0
  else
    echo "  -> failed to load"
    return 1
  fi
}

process_images() {
  local images=("$@")
  local failed=0
  local loaded=0

  check_kind_cluster

  for image in "${images[@]}"; do
    if ! ensure_image_local "${image}"; then
      failed=$((failed + 1))
      continue
    fi

    if load_image "${image}"; then
      loaded=$((loaded + 1))
    else
      failed=$((failed + 1))
    fi
  done

  echo ""
  echo "[load-images] Summary: loaded=${loaded}, failed=${failed}"
  return ${failed}
}

process_images_string() {
  local images_str="$1"
  local failed=0
  local loaded=0

  check_kind_cluster

  for image in ${images_str}; do
    if ! ensure_image_local "${image}"; then
      failed=$((failed + 1))
      continue
    fi

    if load_image "${image}"; then
      loaded=$((loaded + 1))
    else
      failed=$((failed + 1))
    fi
  done

  echo ""
  echo "[load-images] Summary: loaded=${loaded}, failed=${failed}"
  return ${failed}
}

MODE="${1:-all}"

case "${MODE}" in
  --bookinfo|bookinfo)
    echo "[load-images] Loading Bookinfo images..."
    process_images "${BOOKINFO_IMAGES[@]}"
    ;;
  --monitoring|monitoring)
    echo "[load-images] Extracting monitoring images from helm chart..."
    MON_IMAGES=$(extract_monitoring_images "${CHART_DIR}" "${VALUES_FILE}")
    echo "[load-images] Found images:"
    echo "${MON_IMAGES}" | while read -r img; do
      echo "  ${img}"
    done
    echo ""
    process_images_string "${MON_IMAGES}"
    ;;
  --all|all)
    echo "[load-images] Loading Bookinfo images..."
    process_images "${BOOKINFO_IMAGES[@]}"
    echo ""
    echo "[load-images] Extracting monitoring images..."
    MON_IMAGES=$(extract_monitoring_images "${CHART_DIR}" "${VALUES_FILE}")
    echo "[load-images] Loading monitoring stack..."
    process_images_string "${MON_IMAGES}"
    ;;
  *)
    echo "Usage: $0 [bookinfo|monitoring|all]"
    exit 1
    ;;
esac