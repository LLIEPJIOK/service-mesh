---
applyTo: "k&s/**/*.yaml,k&s/**/*.yml,**/*.yaml,**/*.yml"
---

# Kubernetes Manifest Instructions

## Manifest Structure

- Keep manifests distributed by resource.
- Default rule: one Kubernetes resource per file.
- Use stable, predictable file names by kind and resource name.
- Group files by responsibility (platform, workload, monitoring) when directories exist.

## Completeness Requirements

For each resource, include complete and explicit fields relevant to that kind:

- `apiVersion`, `kind`, `metadata.name`, `metadata.namespace` (when namespaced).
- Consistent labels, including `app.kubernetes.io/*` recommended labels where applicable.
- Explicit selectors that match pod template labels.
- Explicit container ports, probes, resource requests/limits, and security context where applicable.

## Workload Security and Runtime

- Prefer non-root execution and explicit security context settings.
- Avoid privileged configuration unless strictly required and documented.
- Keep host-level permissions off by default.

## Service Mesh Specific Rules

- Respect mesh namespace and injection conventions documented in repo docs.
- Keep `serviceAccountName` explicit for workloads.
- Ensure required RBAC for service discovery is present when sidecar logic needs Kubernetes API watch/list access.
- Avoid application container port conflicts with reserved mesh ports (`15001`, `15002`, `15006`, `9090`).
- Keep monitoring annotations aligned with mesh observability contract when enabled.

## Validation Expectations

Before apply, require manifest validation workflow appropriate to the change:

- Schema validation (for example `kubeconform`, including CRD-aware validation when needed).
- `kubectl apply --dry-run=server` checks.
- Deprecated API scanning for target Kubernetes versions (for example `pluto detect-files`).
- Policy checks for security and tenancy constraints (for example `conftest` or Kyverno policy sets).

## Comment Policy

- Avoid inline comments unless they explain a critical safety or compatibility constraint.
- If comments are needed, they must be in English.

## Documentation Sync

- If manifest behavior, install order, security posture, labels/annotations, or runtime guarantees change, update related README documentation in the same change.
