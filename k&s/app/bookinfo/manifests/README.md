# Bookinfo manifests

## Purpose

This directory contains a complete MVP manifest set for running Bookinfo with mesh sidecar injection enabled in minikube.

## Resources

- Namespace `bookinfo` with label `mesh-injection: enabled`
- ServiceAccounts for `productpage`, `details`, `reviews`, `ratings`
- Service-discovery RBAC (`Role` + `RoleBinding`) for sidecar endpoint resolution
- Deployments:
  - `productpage-v1`
  - `details-v1`
  - `ratings-v1`
  - `reviews-v1`, `reviews-v2`, `reviews-v3`
- Services:
  - `productpage` (`NodePort: 31380`)
  - `details`, `ratings`, `reviews` (`ClusterIP`)
- Ingress:
  - `productpage` (class `nginx`, path `/` -> service `productpage:9080`)

## Apply

```bash
kubectl apply -k k\&s/app/bookinfo/manifests
```

## Verify

```bash
kubectl get pods -n bookinfo
kubectl get svc -n bookinfo
```

Application URL in minikube (Ingress):

```bash
minikube addons enable ingress
kubectl rollout status deployment/ingress-nginx-controller -n ingress-nginx --timeout=120s
minikube tunnel
```

В отдельном терминале:

```bash
echo "http://127.0.0.1/productpage"
```

NodePort fallback:

```bash
echo "http://$(minikube ip):31380/productpage"
```
