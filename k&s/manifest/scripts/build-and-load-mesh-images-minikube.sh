#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
VERSION="${VERSION:-v0.1.0}"

build_component() {
  local component_dir="$1"
  local image_name="$2"

  echo "[mesh] Building ${image_name}:${VERSION}"
  make -C "${ROOT_DIR}/mesh/${component_dir}" docker-build \
    DOCKERHUB_NAMESPACE=mesh \
    IMAGE_NAME="${image_name}" \
    VERSION="${VERSION}"

  echo "[mesh] Loading mesh/${image_name}:${VERSION} into minikube"
  minikube image load "mesh/${image_name}:${VERSION}"
}

build_component "sidecar" "sidecar"
build_component "hook" "hook"
build_component "certmanager" "cert-manager"
build_component "iptables" "iptables-init"

echo "[mesh] All images are available in minikube image cache"
