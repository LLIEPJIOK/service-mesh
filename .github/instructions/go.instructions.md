---
applyTo: "k&s/**/*.go,**/*.go"
---

# Go Implementation Instructions

## Runtime Baseline

- Target Go 1.26.

## Design Principles

- Implement layered architecture with clear boundaries.
- Keep domain logic independent from adapters and infrastructure.
- Keep each file and function focused on one responsibility.
- Favor composition and small interfaces.

## Project Layout Guidance

Use a structure that scales cleanly for services and CLIs:

- `cmd/` for binaries and process wiring.
- `internal/domain` for core business rules.
- `internal/app` for use cases and orchestration.
- `internal/adapters` for Kubernetes, HTTP, TLS, storage, and external APIs.
- `internal/config` (or equivalent) as the single config parsing entrypoint.

## Configuration

- Parse env/file config in one place only.
- Validate config centrally.
- Pass typed config structs to components; do not read env vars deep inside business logic.

## Error Handling

- Return errors with actionable context.
- Preserve causality when wrapping lower-level errors.
- Do not log and return the same error in multiple layers.
- Avoid panic in normal execution paths.

## Concurrency and Lifecycle

- No fire-and-forget goroutines without ownership and shutdown signaling.
- Every background worker must have a stop path and a wait path.
- Propagate `context.Context` through call chains.
- Implement deterministic graceful shutdown for listeners and workers.

## Readability

- Prefer guard clauses and shallow nesting.
- Keep naming explicit and domain-oriented.
- Use `gofmt`/`goimports` and static checks (`go vet`, `staticcheck`, or `golangci-lint`).

## Comment Policy

- Comments are allowed only for critical, non-obvious rationale.
- Keep comments short and factual.
- Write comments in English only.

## Tests

- Add focused unit tests for behavior and edge cases.
- Prefer table-driven tests where they improve clarity.
- Test middleware and adapters in isolation with mocks/fakes.

## Documentation Sync

- If Go changes alter behavior, public interfaces, runtime flags/env, or operational flow, update the related README files in the same change.
