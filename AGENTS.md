# AGENTS.md - license-service

## Purpose

`license-service` is the standalone non-production License Service used by
SaasOptimizer development, contract tests, and CI. It implements the v1
HTTP/JSON contract consumed by `license-adapter/pkg/providers/remote`.

It is deliberately not a production licensing authority. It must not be used
for commercial key issuance, billing, customer administration, or production
entitlement storage.

## Boundaries

- Keep protocol and scenario behavior local to this service.
- Do not import Server internals, optimizer code, Cadence code, or deploy-infra.
- `license-adapter` remains the client-side Provider boundary.
- deploy-infra owns container and local orchestration assets, not service logic.
- PostgreSQL and Redis are not required for the default deterministic fixture
  mode. PostgreSQL is the durable activation backend when restart recovery is
  being tested; Redis is not used as activation storage.

## Development Rules

- Go 1.26.3 or newer.
- Every function must have a concise Go doc comment.
- Never log bearer tokens, license keys, or other secret material.
- Inject time and scenario state so tests do not depend on wall-clock timing.
- Preserve the v1 wire contract from Issue 175.

## Build and Test

```bash
go build ./...
go test ./...
go test -race ./...
```

## Runtime Contract

The service exposes:

```text
GET  /v1/health
POST /v1/licenses/validate
POST /v1/licenses/refresh
POST /v1/licenses/activate
POST /v1/licenses/deactivate
```

The default service mode is explicitly non-production and uses an isolated
in-memory scenario store. Set `LICENSE_SERVICE_STORAGE_MODE=postgres`, a
PostgreSQL DSN, and `LICENSE_SERVICE_KEY_PROTECTION_SECRET` to exercise the
durable activation backend. Container and CI integration are maintained by the
workspace deployment layer.
