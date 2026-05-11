#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
ARTIFACTS_DIR="${ROOT_DIR}/k8s/test/artifacts/bookinfo-four-way"
BOOKINFO_MANIFEST_DIR="${ROOT_DIR}/k8s/app/bookinfo/manifests"
INSTALLER_DIR="${ROOT_DIR}/k8s/mesh/installer"

VERSION="${VERSION:-v0.1.0}"
DOCKERHUB_NAMESPACE="${DOCKERHUB_NAMESPACE:-mesh}"
KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-mesh-demo}"
DURATION="${DURATION:-90s}"
RATE="${RATE:-120}"
WORKERS="${WORKERS:-40}"
MAX_WORKERS="${MAX_WORKERS:-200}"
TARGET_PATH="${TARGET_PATH:-/productpage}"
ISTIO_PROFILE="${ISTIO_PROFILE:-demo}"
ISTIOCTL="${ISTIOCTL:-istioctl}"
WARMUP_DURATION="${WARMUP_DURATION:-10s}"
REPEAT_COUNT="${REPEAT_COUNT:-1}"
MODE_LIST="${MODE_LIST:-nosidecar sidecar-buffered sidecar-zero-copy istio}"
STABILIZE_REVIEWS="${STABILIZE_REVIEWS:-false}"

read -r -a MODES <<< "${MODE_LIST}"

require_binary() {
  local bin="$1"
  if ! command -v "${bin}" >/dev/null 2>&1; then
    echo "Missing required binary: ${bin}"
    exit 1
  fi
}

require_binary kubectl
require_binary kind
require_binary docker
require_binary vegeta
require_binary jq
require_binary curl

RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)"
RUN_DIR="${ARTIFACTS_DIR}/${RUN_ID}"
mkdir -p "${RUN_DIR}"

wait_for_rollout() {
  local deployments=(productpage-v1 details-v1 ratings-v1 reviews-v1 reviews-v2 reviews-v3)
  for deployment in "${deployments[@]}"; do
    kubectl rollout status "deployment/${deployment}" -n bookinfo --timeout=240s
  done
}

stabilize_bookinfo_topology() {
  if [[ "${STABILIZE_REVIEWS}" != "true" ]]; then
    return
  fi

  kubectl scale deployment/reviews-v2 deployment/reviews-v3 -n bookinfo --replicas=0
}

wait_for_http_ready() {
  local url="http://127.0.0.1${TARGET_PATH}"
  local deadline=$((SECONDS + 120))

  until [[ "${SECONDS}" -ge "${deadline}" ]]; do
    if curl -fsS -o /dev/null "${url}"; then
      return
    fi
    sleep 2
  done

  echo "Bookinfo did not become HTTP-ready at ${url}"
  exit 1
}

warm_up_target() {
  local mode="$1"
  local mode_dir="${RUN_DIR}/${mode}"
  mkdir -p "${mode_dir}"

  printf "GET http://127.0.0.1%s\n" "${TARGET_PATH}" > "${mode_dir}/warmup-targets.txt"
  vegeta attack \
    -targets "${mode_dir}/warmup-targets.txt" \
    -rate "${RATE}" \
    -duration "${WARMUP_DURATION}" \
    -workers "${WORKERS}" \
    -max-workers "${MAX_WORKERS}" \
    -name "${mode}-warmup" \
    -timeout 30s \
    > "${mode_dir}/warmup-results.bin"
  vegeta report -type=text "${mode_dir}/warmup-results.bin" > "${mode_dir}/warmup-report.txt"
}

collect_evidence() {
  local mode="$1"
  local out_dir="$2"

  kubectl get pods -A -o wide > "${out_dir}/pods.txt" || true
  kubectl get pods -n bookinfo -o json > "${out_dir}/bookinfo-pods.json" || true
  kubectl get events -A --sort-by=.lastTimestamp > "${out_dir}/events.txt" || true
  kubectl describe pods -n bookinfo > "${out_dir}/bookinfo-pods.describe.txt" || true

  if [[ "${mode}" == sidecar-* ]]; then
    kubectl logs -n mesh-system deployment/mesh-webhook --tail=300 > "${out_dir}/mesh-webhook.log" 2>/dev/null || true
    kubectl logs -n mesh-system deployment/mesh-cert-manager --tail=300 > "${out_dir}/mesh-cert-manager.log" 2>/dev/null || true
  fi

  if [[ "${mode}" == "istio" ]]; then
    kubectl logs -n istio-system deployment/istiod --tail=300 > "${out_dir}/istiod.log" 2>/dev/null || true
  fi
}

reset_bookinfo() {
  kubectl delete namespace bookinfo --ignore-not-found
  kubectl wait --for=delete namespace/bookinfo --timeout=180s >/dev/null 2>&1 || true
}

uninstall_custom_mesh() {
  kubectl delete mutatingwebhookconfiguration mesh-sidecar-injector --ignore-not-found
  kubectl delete namespace mesh-system --ignore-not-found
  kubectl wait --for=delete namespace/mesh-system --timeout=180s >/dev/null 2>&1 || true
}

uninstall_istio() {
  if command -v "${ISTIOCTL}" >/dev/null 2>&1; then
    "${ISTIOCTL}" uninstall --purge -y >/dev/null 2>&1 || true
  fi
  kubectl delete namespace istio-system --ignore-not-found
  kubectl wait --for=delete namespace/istio-system --timeout=180s >/dev/null 2>&1 || true
}

ensure_kind_environment() {
  if ! docker info >/dev/null 2>&1; then
    echo "Docker daemon is not available"
    exit 1
  fi

  if ! kind get clusters 2>/dev/null | grep -q "^${KIND_CLUSTER_NAME}$"; then
    "${SCRIPT_DIR}/create-cluster.sh"
  fi

  kubectl cluster-info >/dev/null
  "${SCRIPT_DIR}/install-ingress-nginx.sh"
  "${SCRIPT_DIR}/load-images.sh" all
}

deploy_nosidecar() {
  uninstall_custom_mesh
  uninstall_istio
  reset_bookinfo

  kubectl apply -k "${BOOKINFO_MANIFEST_DIR}"
  kubectl label namespace bookinfo mesh-injection- istio-injection- --overwrite >/dev/null 2>&1 || true
  kubectl rollout restart deployment -n bookinfo
  stabilize_bookinfo_topology
  wait_for_rollout
}

deploy_custom_sidecar() {
  local copy_mode="$1"

  uninstall_istio
  reset_bookinfo

  FORCE_REBUILD=true "${SCRIPT_DIR}/build-and-load-mesh-images.sh" \
    --target kind \
    --version "${VERSION}" \
    --namespace "${DOCKERHUB_NAMESPACE}" \
    --force

  COPY_MODE="${copy_mode}" VERSION="${VERSION}" "${SCRIPT_DIR}/generate-mesh-config.sh"

  pushd "${INSTALLER_DIR}" >/dev/null
  GOCACHE="${GOCACHE:-/private/tmp/service-mesh-go-cache}" go run ./cmd/mesh install \
    -f ../../manifest/generated/mesh-config.kind.yaml \
    --wait --timeout 5m
  popd >/dev/null

  "${SCRIPT_DIR}/deploy-bookinfo.sh"
  stabilize_bookinfo_topology
  wait_for_rollout
}

deploy_istio() {
  require_binary "${ISTIOCTL}"

  uninstall_custom_mesh
  reset_bookinfo

  "${ISTIOCTL}" install -y --set "profile=${ISTIO_PROFILE}"

  kubectl apply -k "${BOOKINFO_MANIFEST_DIR}"
  kubectl label namespace bookinfo istio-injection=enabled mesh-injection- --overwrite >/dev/null 2>&1 || true
  kubectl rollout restart deployment -n bookinfo
  stabilize_bookinfo_topology
  wait_for_rollout
}

run_attack() {
	local mode="$1"
	local sample="$2"
	local mode_dir="${RUN_DIR}/${mode}/${sample}"
	mkdir -p "${mode_dir}"

  printf "GET http://127.0.0.1%s\n" "${TARGET_PATH}" > "${mode_dir}/targets.txt"

  local started_at ended_at
  started_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

  vegeta attack \
    -targets "${mode_dir}/targets.txt" \
    -rate "${RATE}" \
    -duration "${DURATION}" \
    -workers "${WORKERS}" \
    -max-workers "${MAX_WORKERS}" \
		-name "${mode}-${sample}" \
    -timeout 30s \
    > "${mode_dir}/results.bin"

  ended_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

  vegeta report -type=text "${mode_dir}/results.bin" > "${mode_dir}/report.txt"
  vegeta report -type=json "${mode_dir}/results.bin" > "${mode_dir}/report.json"
  vegeta report -type='hist[0,50ms,100ms,250ms,500ms,1s,2s,5s]' "${mode_dir}/results.bin" > "${mode_dir}/hist.txt"
  vegeta plot -title "Bookinfo ${mode}: ${RATE} rps for ${DURATION}" "${mode_dir}/results.bin" > "${mode_dir}/latency-plot.html"

collect_evidence "${mode}" "${mode_dir}"

  jq -n \
		--arg mode "${mode}" \
		--arg sample "${sample}" \
    --arg started_at "${started_at}" \
    --arg ended_at "${ended_at}" \
    --arg duration "${DURATION}" \
    --arg rate "${RATE}" \
		--arg workers "${WORKERS}" \
		--arg max_workers "${MAX_WORKERS}" \
		--arg stabilize_reviews "${STABILIZE_REVIEWS}" \
		--arg target "http://127.0.0.1${TARGET_PATH}" \
		'{mode:$mode,sample:$sample,started_at:$started_at,ended_at:$ended_at,duration:$duration,rate:$rate,workers:$workers,max_workers:$max_workers,stabilize_reviews:$stabilize_reviews,target:$target}' \
		> "${mode_dir}/meta.json"
}

write_sample_summary() {
	local mode="$1"
	local sample="$2"
	local sample_dir="${RUN_DIR}/${mode}/${sample}"
	local mode_dir="${RUN_DIR}/${mode}"
	local report="${sample_dir}/report.json"

	mkdir -p "${mode_dir}"
	jq -n \
		--arg mode "${mode}" \
		--arg sample "${sample}" \
		--slurpfile report "${report}" \
		'{
			mode: $mode,
			sample: $sample,
			throughput: ($report[0].throughput // 0),
			success: ($report[0].success // 0),
			mean_ms: (($report[0].latencies.mean // 0) / 1000000),
			p50_ms: (($report[0].latencies["50th"] // 0) / 1000000),
			p95_ms: (($report[0].latencies["95th"] // 0) / 1000000),
			p99_ms: (($report[0].latencies["99th"] // 0) / 1000000)
		}' > "${sample_dir}/summary.json"
}

write_summary() {
	local summary_file="${RUN_DIR}/summary.md"
	local summary_json="${RUN_DIR}/summary.json"

	{
		echo "["
		local first=true
		for mode in "${MODES[@]}"; do
			for sample_dir in "${RUN_DIR}/${mode}"/sample-*; do
				if [[ ! -f "${sample_dir}/summary.json" ]]; then
					continue
				fi
				if [[ "${first}" == "true" ]]; then
					first=false
				else
					echo ","
				fi
				cat "${sample_dir}/summary.json"
			done
		done
		echo
		echo "]"
	} > "${RUN_DIR}/samples.json"

	jq -n --slurpfile samples "${RUN_DIR}/samples.json" \
		'($samples[0] // []) as $all
		| [($all | group_by(.mode)[] | {
			mode: .[0].mode,
			samples: length,
			throughput: ([.[].throughput] | add / length),
			success: ([.[].success] | add / length),
			mean_ms: ([.[].mean_ms] | sort | if length % 2 == 1 then .[(length / 2 | floor)] else ((.[length / 2 - 1] + .[length / 2]) / 2) end),
			p50_ms: ([.[].p50_ms] | sort | if length % 2 == 1 then .[(length / 2 | floor)] else ((.[length / 2 - 1] + .[length / 2]) / 2) end),
			p95_ms: ([.[].p95_ms] | sort | if length % 2 == 1 then .[(length / 2 | floor)] else ((.[length / 2 - 1] + .[length / 2]) / 2) end),
			p99_ms: ([.[].p99_ms] | sort | if length % 2 == 1 then .[(length / 2 | floor)] else ((.[length / 2 - 1] + .[length / 2]) / 2) end)
		})]' > "${summary_json}"

	{
		echo "# Bookinfo four-way load test (${RUN_ID})"
		echo
		echo "| mode | samples | throughput rps | success | mean ms | p50 ms | p95 ms | p99 ms |"
		echo "| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |"
		jq -r '.[] | "| \(.mode) | \(.samples) | \(.throughput) | \(.success) | \(.mean_ms | tostring) | \(.p50_ms | tostring) | \(.p95_ms | tostring) | \(.p99_ms | tostring) |"' "${summary_json}"
	} > "${summary_file}"

	echo "[bench] Summary: ${summary_file}"
}

ensure_kind_environment

for mode in "${MODES[@]}"; do
  echo "[bench] Deploying ${mode}"
  case "${mode}" in
    nosidecar) deploy_nosidecar ;;
    sidecar-buffered) deploy_custom_sidecar buffered ;;
    sidecar-zero-copy) deploy_custom_sidecar zero-copy ;;
    istio) deploy_istio ;;
  esac

	echo "[bench] Running ${mode}"
	wait_for_http_ready
	for ((sample_idx = 1; sample_idx <= REPEAT_COUNT; sample_idx++)); do
		sample="sample-${sample_idx}"
		echo "[bench] Running ${mode}/${sample}"
		warm_up_target "${mode}/${sample}"
		run_attack "${mode}" "${sample}"
		write_sample_summary "${mode}" "${sample}"
	done
done

write_summary
echo "[bench] Artifacts: ${RUN_DIR}"
