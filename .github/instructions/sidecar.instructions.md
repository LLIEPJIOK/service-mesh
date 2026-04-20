---
applyTo: "k&s/mesh/sidecar/**,k&s/mesh/hook/**,k&s/mesh/iptables/**,k&s/mesh/certmanager/**,k&s/mesh/installer/**"
---

# Service Mesh Domain Instructions

## Contract-First Implementation

- Treat docs in `k&s/mesh/**` and `k&s/docs/**` as normative contracts.
- Do not introduce behavior that contradicts MVP documents unless docs are updated in the same change.

## Sidecar Core Contracts

- Preserve the three-listener model:
  - inbound plain port
  - outbound port
  - inbound mTLS port
- Keep transparent proxy behavior aligned with iptables and `SO_ORIGINAL_DST` rules.
- Keep fallback behavior when discovery cannot resolve endpoints.

## Service Discovery

- Maintain LIST bootstrap + WATCH updates behavior.
- Keep ready-endpoint filtering.
- Keep relist/rewatch recovery behavior after watcher disruption.
- Preserve key mappings needed for `serviceName:port` and `clusterIP:port` lookups.

## mTLS and Identity

- Identity source for cert issuance is ServiceAccount token validation, not CSR CN/SAN claims.
- Preserve incoming mTLS verification on inbound mTLS port.
- For mesh endpoints, set outbound TLS `ServerName` from service identity (FQDN), not endpoint IP.

## Reliability Boundaries

- Retry only where documented for connection-establishment failures.
- Keep timeout and circuit-breaker semantics aligned with docs.
- Do not silently broaden retries to application-level status codes unless explicitly documented.

## Observability

- Preserve `/metrics` contract and required metric families.
- Ensure metrics path/port behavior remains compatible with documented scrape settings.
- Keep metrics port excluded from redirection loops.

## Lifecycle and Shutdown

- Sidecar must not serve traffic before required certificate bootstrap completes.
- Implement graceful shutdown: stop accepting new connections, then drain active work within timeout.

## Webhook and Installer

- Keep webhook mutation idempotent.
- Keep install/uninstall ordering deterministic as documented.
- Preserve namespace and injection gating rules.

## Comment Policy

- Keep comments minimal and limited to critical rationale.
- Comments must be in English.

## Documentation Sync (Mandatory)

When changing any of these contracts, update the relevant docs in the same change:

- `k&s/mesh/sidecar/README.md`
- `k&s/mesh/sidecar/docs/*.md`
- `k&s/mesh/hook/README.md`
- `k&s/mesh/installer/README.md`
- `k&s/docs/cert/README.md`
- `k&s/docs/role/README.md`
- `k&s/docs/service/account/README.md`
- `k&s/manifest/README.md`

No behavior-contract change is complete without corresponding README/doc updates.
