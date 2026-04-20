# Repository Custom Instructions for GitHub Copilot

## Purpose

This repository documents and implements a service mesh MVP for Kubernetes.
Treat the documentation under `k&s/` as a contract-first source of requirements.

## Language and Style

- Write instruction content in English.
- Prioritize readability over cleverness.
- Keep files and functions small and focused. Do not enforce arbitrary numeric limits; enforce single responsibility.
- Prefer explicit naming and straightforward control flow.

## Comment Policy

- Keep comments minimal.
- Add comments only for critical, non-obvious intent or safety constraints.
- Write all comments in English.
- Do not add narrative comments for obvious code.

## Architecture Rules

- Use layered architecture and clear dependency direction.
- Separate domain logic from transport, infra, and framework code.
- Keep side effects at the edges.
- Parse runtime configuration in one dedicated place and pass typed config downward.

## Go Baseline

- Target Go 1.26.
- Handle errors explicitly and preserve context when returning errors.
- Avoid panic in normal runtime paths; return errors.
- Use context propagation and explicit shutdown paths for long-running components.

## Kubernetes Baseline

- Manifests must be complete and production-readable.
- Distribute manifests by resource (one resource per file by default).
- Keep labels, security, resources, and probes explicit when applicable.

## Service Mesh Domain Guardrails

- Respect control-plane and data-plane boundaries.
- Preserve documented contracts for sidecar listeners, discovery, reliability, and observability.
- Treat sidecar and webhook behavior documented in `k&s/mesh/**` as normative.

## README as Source of Truth

- README files are a contract, not optional prose.
- If a change alters behavior, API, configuration, lifecycle, install order, or operational guarantees, update the affected README files in the same change.
- Do not merge behavior changes that make README content stale.

## Validation Expectations

When relevant to generated changes, suggest and run appropriate checks:

- Go: `gofmt`/`goimports`, `go vet`, `staticcheck` (or `golangci-lint`), and tests.
- Kubernetes: `kubeconform` (or equivalent schema validator), `kubectl apply --dry-run=server`, deprecated API checks (for example `pluto`), and policy checks (for example `conftest` or Kyverno policies).
- Mesh behavior: smoke checks aligned with documented acceptance criteria.

## Instruction Precedence

- Use this file as repository-wide baseline.
- Combine with path-specific files under `.github/instructions/`.
- If rules conflict, prefer the more specific path-scoped instruction file.
