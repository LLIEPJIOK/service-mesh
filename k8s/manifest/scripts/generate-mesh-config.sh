#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
OUT_DIR="${ROOT_DIR}/k8s/manifest/generated"
CONFIG_FILE="${OUT_DIR}/mesh-config.kind.yaml"
CA_CERT_FILE="${OUT_DIR}/root-ca.crt"
CA_KEY_FILE="${OUT_DIR}/root-ca.key"
VERSION="${VERSION:-v0.1.0}"

mkdir -p "${OUT_DIR}"

if [[ ! -f "${CA_CERT_FILE}" ]] || [[ ! -f "${CA_KEY_FILE}" ]]; then
  echo "[mesh] Generating new root CA..."
  openssl req -x509 -newkey rsa:4096 -sha256 -nodes \
    -keyout "${CA_KEY_FILE}" \
    -out "${CA_CERT_FILE}" \
    -days 365 \
    -subj "/CN=mesh-root-ca/O=service-mesh-mvp"
else
  echo "[mesh] Using existing root CA (delete files to regenerate)"
fi

indent_pem() {
  sed 's/^/        /'
}

cat > "${CONFIG_FILE}" <<EOF
apiVersion: mesh.io/v1alpha1
kind: MeshConfig
metadata:
  name: kind
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
    inboundMTLSPort: 15001
    mtlsEnabled: true
    metricsPort: 9090
    monitoringEnabled: true
    loadBalancerAlgorithm: roundRobin
    copyMode: ${COPY_MODE:-buffered}
    retryPolicy:
      attempts: 3
      backoff:
        type: exponential
        baseInterval: 100ms
    timeout: 5s
    circuitBreakerPolicy:
      failureThreshold: 5
      recoveryTime: 30s
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
