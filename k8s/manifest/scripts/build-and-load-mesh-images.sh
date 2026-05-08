#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
SCRIPTS_DIR="${ROOT_DIR}/k8s/manifest/scripts"
KIND_DIR="${SCRIPTS_DIR}"
VERSION="${VERSION:-v0.1.0}"
DOCKERHUB_NAMESPACE="${DOCKERHUB_NAMESPACE:-mesh}"
TARGET="${TARGET:-kind}"
FORCE_REBUILD="${FORCE_REBUILD:-false}"
SKIP_LOAD="${SKIP_LOAD:-false}"
KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-mesh-demo}"

SUPPORTED_TARGETS="kind minikube"

usage() {
  cat <<EOF
Usage: $0 [OPTIONS]

Options:
  --target TARGET   Target platform: kind or minikube (default: kind)
  --version VERSION  Image version tag (default: v0.1.0)
  --namespace NS     Docker namespace for images (default: mesh)
  --force            Force rebuild even if image exists locally
  --skip-load        Build only, skip loading into cluster
  --help             Show this help

Environment variables:
  VERSION              Image version (default: v0.1.0)
  DOCKERHUB_NAMESPACE  Docker namespace (default: mesh)
  TARGET               kind or minikube (default: kind)
  FORCE_REBUILD        Set to 'true' to force rebuild
  KIND_CLUSTER_NAME    kind cluster name (default: mesh-demo)

Examples:
  $0                           Build and load into kind
  $0 --target minikube         Build and load into minikube
  VERSION=v0.2.0 $0 --force     Force rebuild with new version
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --target) TARGET="$2"; shift 2 ;;
    --version) VERSION="$2"; shift 2 ;;
    --namespace) DOCKERHUB_NAMESPACE="$2"; shift 2 ;;
    --force) FORCE_REBUILD="true"; shift ;;
    --skip-load) SKIP_LOAD="true"; shift ;;
    --help) usage; exit 0 ;;
    *) echo "Unknown option: $1"; usage; exit 1 ;;
  esac
done

if [[ ! " ${SUPPORTED_TARGETS} " =~ " ${TARGET} " ]]; then
  echo "ERROR: target must be one of: ${SUPPORTED_TARGETS}"
  exit 1
fi

require_binary() {
  local bin="$1"
  if ! command -v "${bin}" >/dev/null 2>&1; then
    echo "Missing required binary: ${bin}"
    exit 1
  fi
}

check_kind_cluster() {
  if ! kind get clusters 2>/dev/null | grep -q "^${KIND_CLUSTER_NAME}$"; then
    echo "ERROR: Kind cluster '${KIND_CLUSTER_NAME}' does not exist"
    return 1
  fi
  return 0
}

get_goarch() {
  local arch
  arch="$(uname -m)"
  case "${arch}" in
    x86_64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *) echo "amd64" ;;
  esac
}

image_exists_locally() {
  local image="$1"
  docker image inspect "${image}" >/dev/null 2>&1
}

image_loaded_in_kind() {
  local image="$1"
  local node_name="${KIND_CLUSTER_NAME}-control-plane"
  local img_name="${image#docker.io/}"
  img_name="${img_name#registry.k8s.io/}"
  img_name="${img_name#quay.io/}"
  img_name="${img_name#ghcr.io/}"

  docker exec "${node_name}" ctr -n k8s.io images ls 2>/dev/null | grep -q "${img_name}"
}

build_component() {
  local component_dir="$1"
  local image_name="$2"
  local full_image="${DOCKERHUB_NAMESPACE}/${image_name}:${VERSION}"
  local latest_image="${DOCKERHUB_NAMESPACE}/${image_name}:latest"
  local goarch="$(get_goarch)"

  echo "[mesh] Building ${full_image} for linux/${goarch}"

  make -C "${ROOT_DIR}/mesh/${component_dir}" docker-build \
    DOCKERHUB_NAMESPACE="${DOCKERHUB_NAMESPACE}" \
    IMAGE_NAME="${image_name}" \
    VERSION="${VERSION}" \
    GOARCH="${goarch}"

  echo "[mesh] Built ${full_image}"
}

load_into_kind() {
  local image="$1"
  local node_name="${KIND_CLUSTER_NAME}-control-plane"

  if image_loaded_in_kind "${image}"; then
    echo "[mesh] ${image} already in cluster, skipping"
    return 0
  fi

  echo "[mesh] Loading ${image} into kind cluster '${KIND_CLUSTER_NAME}'..."
  if docker save "${image}" 2>/dev/null | docker exec -i "${node_name}" ctr -n k8s.io images import - 2>/dev/null; then
    echo "[mesh] -> loaded"
    return 0
  else
    echo "[mesh] -> failed to load"
    return 1
  fi
}

load_into_minikube() {
  local image="$1"

  echo "[mesh] Loading ${image} into minikube"
  if minikube image load "${image}"; then
    echo "[mesh] -> loaded"
    return 0
  else
    echo "[mesh] -> failed to load"
    return 1
  fi
}

process_components() {
  local component_dir image_name full_image latest_image goarch
  local failed=0

  for comp in "sidecar:sidecar" "hook:hook" "certmanager:cert-manager" "iptables:iptables-init"; do
    IFS=':' read -r component_dir image_name <<< "${comp}"

    full_image="${DOCKERHUB_NAMESPACE}/${image_name}:${VERSION}"
    latest_image="${DOCKERHUB_NAMESPACE}/${image_name}:latest"

    if [[ "${FORCE_REBUILD}" != "true" ]] && image_exists_locally "${full_image}"; then
      echo "[mesh] Image ${full_image} already exists locally, skipping build (use --force to rebuild)"
    else
      build_component "${component_dir}" "${image_name}"
    fi

    if [[ "${SKIP_LOAD}" == "true" ]]; then
      echo "[mesh] Skipping load (--skip-load flag set)"
      continue
    fi

    if [[ "${TARGET}" == "kind" ]]; then
      if ! check_kind_cluster; then
        echo "[mesh] Kind cluster not available, skipping load"
        continue
      fi

      load_into_kind "${full_image}" || failed=$((failed + 1))
      load_into_kind "${latest_image}" || failed=$((failed + 1))
    elif [[ "${TARGET}" == "minikube" ]]; then
      load_into_minikube "${full_image}" || failed=$((failed + 1))
      load_into_minikube "${latest_image}" || failed=$((failed + 1))
    fi
  done

  return ${failed}
}

process_components

echo "[mesh] All images processed"
if [[ "${TARGET}" == "kind" ]]; then
  echo "[mesh] Run 'kubectl get nodes' to verify cluster is ready"
fi