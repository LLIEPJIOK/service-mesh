#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
OUT_DIR="${ROOT_DIR}/k8s/manifest/generated"
CA_CERT_FILE="${OUT_DIR}/root-ca.crt"
CA_KEY_FILE="${OUT_DIR}/root-ca.key"
VERSION="${VERSION:-v0.1.0}"
MODE="${1:-}"

if [[ -z "${MODE}" ]]; then
  echo "Usage: $0 <mode>"
  echo "Modes: baseline | mtls | patterns | full"
  exit 1
fi

mkdir -p "${OUT_DIR}"

if [[ ! -f "${CA_CERT_FILE}" || ! -f "${CA_KEY_FILE}" ]]; then
  openssl req -x509 -newkey rsa:4096 -sha256 -nodes \
    -keyout "${CA_KEY_FILE}" \
    -out "${CA_CERT_FILE}" \
    -days 365 \
    -subj "/CN=mesh-root-ca/O=service-mesh-mvp"
fi

indent_pem() {
  sed 's/^/        /'
}

mtls_enabled="true"
inbound_mtls_port="15001"
monitoring_enabled="true"
load_balancer_algorithm="roundRobin"
retry_attempts="3"
timeout_value="5s"
circuit_breaker_threshold="5"
circuit_breaker_recovery="30s"

case "${MODE}" in
  baseline)
    echo "[mesh] baseline mode does not require mesh config"
    exit 0
    ;;
  mtls)
    monitoring_enabled="false"
    load_balancer_algorithm="none"
    retry_attempts="1"
    timeout_value="0s"
    circuit_breaker_threshold="0"
    circuit_breaker_recovery="0s"
    ;;
  patterns)
    mtls_enabled="false"
    inbound_mtls_port="0"
    monitoring_enabled="true"
    load_balancer_algorithm="roundRobin"
    retry_attempts="3"
    timeout_value="5s"
    circuit_breaker_threshold="5"
    circuit_breaker_recovery="30s"
    ;;
  full)
    ;;
  *)
    echo "Unknown mode: ${MODE}"
    echo "Modes: baseline | mtls | patterns | full"
    exit 1
    ;;
esac

CONFIG_FILE="${OUT_DIR}/mesh-config.${MODE}.yaml"

cat > "${CONFIG_FILE}" <<EOF
apiVersion: mesh.io/v1alpha1
kind: MeshConfig
metadata:
  name: minikube-${MODE}
spec:
  namespace: mesh-system
  version: "${VERSION}"

  images:
    sidecar: mesh/sidecar:${VERSION}
    iptablesInit: mesh/iptables-init:${VERSION}
    certManager: mesh/cert-manager:${VERSION}

  certificates:
    rootCA:
      cert: |
$(indent_pem < "${CA_CERT_FILE}")
      key: |
$(indent_pem < "${CA_KEY_FILE}")
    validity: 8760h

  sidecar:
    inboundPlainPort: 15006
    outboundPort: 15002
    inboundMTLSPort: ${inbound_mtls_port}
    mtlsEnabled: ${mtls_enabled}
    metricsPort: 9090
    monitoringEnabled: ${monitoring_enabled}
    loadBalancerAlgorithm: ${load_balancer_algorithm}
    retryPolicy:
      attempts: ${retry_attempts}
      backoff:
        type: exponential
        baseInterval: 100ms
    timeout: ${timeout_value}
    circuitBreakerPolicy:
      failureThreshold: ${circuit_breaker_threshold}
      recoveryTime: ${circuit_breaker_recovery}
    excludeInboundPorts: "9090"
    excludeOutboundIPs: "169.254.169.254/32"

  injection:
    namespaceSelector:
      matchLabels:
        mesh-injection: enabled

  certManager:
    enabled: true
    resources: {}
EOF

echo "[mesh] Generated ${CONFIG_FILE}"
