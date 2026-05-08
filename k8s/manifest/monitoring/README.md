# Monitoring manifests and values

## Purpose

This directory contains monitoring resources and Helm values used to run Prometheus and Grafana on minikube for mesh smoke checks.

## Files

- `00-monitoring-namespace.yaml`: namespace for monitoring stack
- `01-bookinfo-sidecar-podmonitor.yaml`: PodMonitor for Bookinfo sidecar metrics (`/metrics`, port `9090`)
- `02-grafana-ingress.yaml`: Ingress for Grafana UI (class `nginx`)
- `03-grafana-dashboard-sidecar-apps.yaml`: Grafana dashboard provisioning for sidecar application metrics
- `kube-prometheus-stack-values.yaml`: values overrides for `kube-prometheus-stack`

## Install

```bash
kubectl apply -f k\&s/manifest/monitoring/00-monitoring-namespace.yaml
helm upgrade --install mesh-monitoring oci://ghcr.io/prometheus-community/charts/kube-prometheus-stack \
  -n monitoring \
  -f k\&s/manifest/monitoring/kube-prometheus-stack-values.yaml \
  --wait --timeout 10m
kubectl apply -f k\&s/manifest/monitoring/01-bookinfo-sidecar-podmonitor.yaml
kubectl apply -f k\&s/manifest/monitoring/02-grafana-ingress.yaml
kubectl apply -f k\&s/manifest/monitoring/03-grafana-dashboard-sidecar-apps.yaml
```

## Access

```bash
minikube tunnel
echo "Grafana:    http://grafana.127.0.0.1.nip.io"
echo "Grafana fallback: http://$(minikube ip):32000"
echo "Prometheus: http://$(minikube ip):32001"
```

Grafana credentials:

- user: `admin`
- password: `admin`

## Verify dashboard provisioning

```bash
curl -s -u admin:admin http://grafana.127.0.0.1/api/search \
  | jq '.[] | select(.title == "Mesh Sidecar: Application Metrics")'
```
