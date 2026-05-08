.PHONY: help kind-env kind-env-nosidecar kind-env-sidecar kind-down kind-clean kind-images-preload kind-images-load \
	kind-mesh-install kind-bookinfo kind-bookinfo-nosidecar kind-monitoring kind-status kind-create \
	mesh-build-all mesh-build-sidecar mesh-build-hook mesh-build-certmanager mesh-build-iptables \
	vegeta-bench vegeta-bench-nosidecar vegeta-bench-sidecar vegeta-plot \
	vegeta-ramp vegeta-ramp-nosidecar vegeta-ramp-sidecar vegeta-ramp-plot

SHELL := /bin/bash
ROOT_DIR := $(shell pwd)
MANIFEST_SCRIPTS_DIR := $(ROOT_DIR)/k8s/manifest/scripts
MESH_DIR := $(ROOT_DIR)/k8s/mesh
VERSION ?= v0.1.0
DOCKERHUB_NAMESPACE ?= mesh
KIND_CLUSTER_NAME ?= mesh-demo
GOARCH ?= $(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')

export VERSION
export DOCKERHUB_NAMESPACE
export KIND_CLUSTER_NAME
export GOARCH

help:
	@echo "Service Mesh Demo - Make targets for local development"
	@echo ""
	@echo "Kind cluster targets:"
	@echo "  make kind-env              - Full environment setup (cluster + ingress + images + mesh + bookinfo + monitoring)"
	@echo "  make kind-env-nosidecar    - Environment without sidecar (bookinfo only)"
	@echo "  make kind-env-sidecar      - Environment with mesh + sidecar"
	@echo "  make kind-down             - Delete kind cluster"
	@echo "  make kind-clean            - Delete cluster and clean up generated files"
	@echo ""
	@echo "  make kind-images-preload   - Pull all external images into local Docker (one-time)"
	@echo "  make kind-images-load      - Load images from local Docker into kind cluster"
	@echo "  make kind-status           - Show cluster and deployment status"
	@echo ""
	@echo "Vegeta benchmark targets:"
	@echo "  make vegeta-bench          - Run benchmarks (without sidecar, then with sidecar) and plot"
	@echo "  make vegeta-bench-nosidecar - Run benchmark WITHOUT sidecar"
	@echo "  make vegeta-bench-sidecar  - Run benchmark WITH sidecar"
	@echo "  make vegeta-plot           - Generate comparative plots"
	@echo ""
	@echo "Vegeta ramp test targets:"
	@echo "  make vegeta-ramp           - Run ramp tests (nosidecar -> sidecar -> plot)"
	@echo "  make vegeta-ramp-nosidecar - Run ramp test WITHOUT sidecar"
	@echo "  make vegeta-ramp-sidecar   - Run ramp test WITH sidecar"
	@echo "  make vegeta-ramp-plot       - Generate ramp comparison plots"
	@echo ""
	@echo "Mesh component targets:"
	@echo "  make mesh-build-all        - Build all mesh images (sidecar, hook, certmanager, iptables)"
	@echo "  make mesh-build-<name>     - Build specific component (sidecar, hook, certmanager, iptables)"
	@echo ""
	@echo "Deployment targets:"
	@echo "  make kind-mesh-install     - Install mesh into cluster"
	@echo "  make kind-bookinfo         - Deploy bookinfo"
	@echo "  make kind-monitoring       - Install monitoring stack"
	@echo ""
	@echo "Environment variables:"
	@echo "  VERSION=$(VERSION)"
	@echo "  DOCKERHUB_NAMESPACE=$(DOCKERHUB_NAMESPACE)"
	@echo "  KIND_CLUSTER_NAME=$(KIND_CLUSTER_NAME)"
	@echo "  GOARCH=$(GOARCH) (auto-detected: amd64 or arm64)"

kind-env: kind-create install-ingress kind-images-load mesh-build-and-load kind-mesh-install kind-bookinfo kind-monitoring
	@echo ""
	@echo "=========================================="
	@echo "Environment ready!"
	@echo "Bookinfo: http://127.0.0.1/productpage"
	@echo "Grafana:  http://grafana.127.0.0.1.nip.io (admin/admin)"
	@echo "Prometheus: kubectl port-forward -n monitoring svc/mesh-monitoring-prometheus 9090:9090"
	@echo "=========================================="

kind-env-sidecar: kind-create install-ingress kind-images-load mesh-build-and-load kind-mesh-install kind-bookinfo kind-monitoring
	@echo ""
	@echo "=========================================="
	@echo "Environment with sidecar ready!"
	@echo "Bookinfo: http://127.0.0.1/productpage"
	@echo "Grafana:  http://grafana.127.0.0.1.nip.io (admin/admin)"
	@echo "=========================================="

kind-env-nosidecar: kind-create install-ingress kind-images-load kind-bookinfo-nosidecar
	@echo ""
	@echo "=========================================="
	@echo "Environment without sidecar ready!"
	@echo "Bookinfo: http://127.0.0.1/productpage"
	@echo "(mesh components NOT installed)"
	@echo "=========================================="

install-ingress:
	@echo "[make] Installing NGINX Ingress..."
	@bash "$(MANIFEST_SCRIPTS_DIR)/install-ingress-nginx.sh"

kind-images-preload:
	@echo "[make] Pre-loading external images into local Docker..."
	@bash "$(MANIFEST_SCRIPTS_DIR)/preload-images.sh"

kind-images-load:
	@echo "[make] Loading images into kind cluster..."
	@bash "$(MANIFEST_SCRIPTS_DIR)/load-images.sh" all

mesh-build-and-load:
	@echo "[make] Building and loading mesh images..."
	@bash "$(MANIFEST_SCRIPTS_DIR)/build-and-load-mesh-images.sh" --target kind --version $(VERSION) --namespace $(DOCKERHUB_NAMESPACE)

kind-generate-config:
	@echo "[make] Generating mesh config..."
	@bash "$(MANIFEST_SCRIPTS_DIR)/generate-mesh-config.sh"

kind-mesh-install: kind-generate-config
	@echo "[make] Installing mesh..."
	@cd "$(MESH_DIR)/installer" && go run ./cmd/mesh install \
		-f ../../manifest/generated/mesh-config.kind.yaml \
		--wait --timeout 5m

kind-bookinfo:
	@echo "[make] Deploying Bookinfo..."
	@bash "$(MANIFEST_SCRIPTS_DIR)/deploy-bookinfo.sh"

kind-bookinfo-nosidecar:
	@echo "[make] Deploying Bookinfo (no mesh)..."
	@bash "$(MANIFEST_SCRIPTS_DIR)/deploy-bookinfo-nosidecar.sh"

kind-monitoring:
	@echo "[make] Installing monitoring..."
	@bash "$(MANIFEST_SCRIPTS_DIR)/install-monitoring.sh"

kind-down:
	@echo "[make] Deleting kind cluster '$(KIND_CLUSTER_NAME)'..."
	@kind delete cluster --name "$(KIND_CLUSTER_NAME)" 2>/dev/null || true

kind-create:
	@echo "[make] Creating kind cluster '$(KIND_CLUSTER_NAME)'..."
	@bash "$(MANIFEST_SCRIPTS_DIR)/create-cluster.sh"

kind-clean: kind-down
	@echo "[make] Cleaning up generated files..."
	@rm -rf "$(ROOT_DIR)/k8s/manifest/generated" 2>/dev/null || true
	@rm -rf "$(MESH_DIR)/installer/bin" 2>/dev/null || true

kind-status:
	@echo "=== Kind Clusters ==="
	@kind get clusters
	@echo ""
	@echo "=== Nodes ==="
	@kubectl get nodes -o wide
	@echo ""
	@echo "=== Deployments (mesh-system) ==="
	@kubectl get deployments -n mesh-system 2>/dev/null || echo "mesh-system not found"
	@echo ""
	@echo "=== Deployments (bookinfo) ==="
	@kubectl get deployments -n bookinfo 2>/dev/null || echo "bookinfo not found"
	@echo ""
	@echo "=== Deployments (monitoring) ==="
	@kubectl get deployments -n monitoring 2>/dev/null || echo "monitoring not found"

mesh-build-all: mesh-build-sidecar mesh-build-hook mesh-build-certmanager mesh-build-iptables

mesh-build-sidecar:
	@echo "[make] Building sidecar image for $(GOARCH)..."
	@make -C "$(MESH_DIR)/sidecar" docker-build \
		DOCKERHUB_NAMESPACE=$(DOCKERHUB_NAMESPACE) IMAGE_NAME=sidecar VERSION=$(VERSION) GOARCH=$(GOARCH)

mesh-build-hook:
	@echo "[make] Building hook image for $(GOARCH)..."
	@make -C "$(MESH_DIR)/hook" docker-build \
		DOCKERHUB_NAMESPACE=$(DOCKERHUB_NAMESPACE) IMAGE_NAME=hook VERSION=$(VERSION) GOARCH=$(GOARCH)

mesh-build-certmanager:
	@echo "[make] Building cert-manager image for $(GOARCH)..."
	@make -C "$(MESH_DIR)/certmanager" docker-build \
		DOCKERHUB_NAMESPACE=$(DOCKERHUB_NAMESPACE) IMAGE_NAME=cert-manager VERSION=$(VERSION) GOARCH=$(GOARCH)

mesh-build-iptables:
	@echo "[make] Building iptables-init image for $(GOARCH)..."
	@make -C "$(MESH_DIR)/iptables" docker-build \
		DOCKERHUB_NAMESPACE=$(DOCKERHUB_NAMESPACE) IMAGE_NAME=iptables-init VERSION=$(VERSION) GOARCH=$(GOARCH)

vegeta-bench-nosidecar:
	@echo "[make] Running vegeta benchmark WITHOUT sidecar..."
	@bash "$(MANIFEST_SCRIPTS_DIR)/run-vegeta-comparison.sh" nosidecar

vegeta-bench-sidecar:
	@echo "[make] Running vegeta benchmark WITH sidecar..."
	@bash "$(MANIFEST_SCRIPTS_DIR)/run-vegeta-comparison.sh" sidecar

vegeta-bench: vegeta-bench-nosidecar vegeta-bench-sidecar vegeta-plot
	@echo ""
	@echo "=========================================="
	@echo "Benchmark complete! Results in k8s/test/artifacts/"
	@echo "=========================================="

vegeta-plot:
	@echo "[make] Generating comparative plots..."
	@bash "$(MANIFEST_SCRIPTS_DIR)/plot-vegeta-results.sh"

vegeta-ramp-nosidecar:
	@echo "[make] Running ramp test WITHOUT sidecar..."
	@mkdir -p "$(ROOT_DIR)/k8s/test/artifacts/ramp/nosidecar"
	@bash "$(MANIFEST_SCRIPTS_DIR)/run-vegeta-ramp.sh" nosidecar

vegeta-ramp-sidecar:
	@echo "[make] Running ramp test WITH sidecar..."
	@mkdir -p "$(ROOT_DIR)/k8s/test/artifacts/ramp/sidecar"
	@bash "$(MANIFEST_SCRIPTS_DIR)/run-vegeta-ramp.sh" sidecar

vegeta-ramp: vegeta-ramp-nosidecar vegeta-ramp-sidecar vegeta-ramp-plot
	@echo ""
	@echo "=========================================="
	@echo "Ramp test complete!"
	@echo "=========================================="

vegeta-ramp-plot:
	@echo "[make] Generating ramp comparison plots..."
	@bash "$(MANIFEST_SCRIPTS_DIR)/plot-vegeta-ramp.sh"