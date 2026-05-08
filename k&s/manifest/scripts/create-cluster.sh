#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KIND_CONFIG="${SCRIPT_DIR}/kind-config.yaml"
KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-mesh-demo}"

require_binary() {
  local bin="$1"
  if ! command -v "${bin}" >/dev/null 2>&1; then
    echo "Missing required binary: ${bin}"
    exit 1
  fi
}

require_binary kind
require_binary kubectl

echo "[kind] Checking cluster..."
if kind get clusters 2>/dev/null | grep -q "^${KIND_CLUSTER_NAME}$"; then
  echo "[kind] Deleting existing cluster '${KIND_CLUSTER_NAME}'"
  kind delete cluster --name "${KIND_CLUSTER_NAME}"
fi

echo "[kind] Creating cluster '${KIND_CLUSTER_NAME}'"
kind create cluster --name "${KIND_CLUSTER_NAME}" --config "${KIND_CONFIG}" --wait 5m

echo "[kind] Cluster ready: $(kubectl cluster-info | head -n1)"