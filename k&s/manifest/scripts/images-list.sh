#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
BOOKINFO_MANIFESTS="${ROOT_DIR}/app/bookinfo/manifests"

BOOKINFO_IMAGES=(
  "docker.io/istio/examples-bookinfo-productpage-v1:1.20.3"
  "docker.io/istio/examples-bookinfo-details-v1:1.20.3"
  "docker.io/istio/examples-bookinfo-ratings-v1:1.20.3"
  "docker.io/istio/examples-bookinfo-reviews-v1:1.20.3"
  "docker.io/istio/examples-bookinfo-reviews-v2:1.20.3"
  "docker.io/istio/examples-bookinfo-reviews-v3:1.20.3"
)

KUBE_PROM_STACK_IMAGES=(
  "ghcr.io/prometheus-operator/prometheus-operator:v0.73.2"
  "quay.io/prometheus/prometheus:v2.50.1"
  "quay.io/prometheus/alertmanager:v0.27.0"
  "registry.k8s.io/kube-state-metrics/kube-state-metrics:v2.13.0"
  "registry.k8s.io/prometheus-node-exporter/prometheus-node-exporter:v1.7.0"
  "docker.io/grafana/grafana:10.4.0"
  "quay.io/brancz/kube-rbac-proxy:v0.16.0"
  "jimmidyson/configmap-reload:v0.12.0"
  "registry.k8s.io/ingress-nginx/controller:v1.11.1"
)

usage() {
  cat <<EOF
Usage: $0 [--bookinfo|--monitoring|--all] [--format=plain|json]

Lists container images required for mesh demo.

Examples:
  $0 --all              # List all images
  $0 --bookinfo          # List only Bookinfo images
  $0 --monitoring        # List only monitoring stack images
  $0 --format=json       # Output as JSON array
  $0 --check             # Check if images exist locally
EOF
}

list_images() {
  local type="$1"
  local format="${2:-plain}"
  local images=()

  case "${type}" in
    bookinfo)
      images=("${BOOKINFO_IMAGES[@]}")
      ;;
    monitoring)
      images=("${KUBE_PROM_STACK_IMAGES[@]}")
      ;;
    all)
      images=("${BOOKINFO_IMAGES[@]}" "${KUBE_PROM_STACK_IMAGES[@]}")
      ;;
  esac

  if [[ "${format}" == "json" ]]; then
    printf '%s\n' "${images[@]}" | jq -R . | jq -s .
  else
    for img in "${images[@]}"; do
      echo "${img}"
    done
  fi
}

check_images() {
  local missing=0
  local images=("$@")

  for img in "${images[@]}"; do
    if ! docker image inspect "${img}" >/dev/null 2>&1; then
      echo "MISSING: ${img}"
      missing=$((missing + 1))
    else
      echo "PRESENT: ${img}"
    fi
  done

  if [[ ${missing} -gt 0 ]]; then
    echo ""
    echo "ERROR: ${missing} image(s) missing. Pull them first with:"
    echo "  docker pull <image>"
    return 1
  fi
  echo "All images present locally."
}

TYPE="all"
FORMAT="plain"
CHECK=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --bookinfo) TYPE="bookinfo"; shift ;;
    --monitoring) TYPE="monitoring"; shift ;;
    --all) TYPE="all"; shift ;;
    --format=*) FORMAT="${1#*=}"; shift ;;
    --check) CHECK=true; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1"; usage; exit 1 ;;
  esac
done

images=()
while IFS= read -r img; do
  images+=("${img}")
done < <(list_images "${TYPE}" "${FORMAT}")

if [[ "${CHECK}" == "true" ]]; then
  check_images "${images[@]}"
else
  list_images "${TYPE}" "${FORMAT}"
fi