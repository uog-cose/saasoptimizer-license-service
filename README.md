# SaasOptimizer Development License Service

This repository provides a deterministic, standalone License Service for
development and CI. It implements the versioned v1 HTTP/JSON contract consumed
by the Server-side remote license Provider.

It supports controlled allow, deny, expiry, revocation, quota, outage, delay,
and protocol-compatibility scenarios. It is not a production License Server
and must never receive production license keys or customer data.

## Current boundary

```text
Server -> license-adapter Remote Provider -> license-service
```

The service owns development scenario evaluation and the activation persistence
boundary used by the non-production deployment. Production billing, key
issuance, and customer administration are outside this repository. Memory mode
remains the default for fast unit tests; PostgreSQL mode is required when an
activation must survive a service restart.

## Local development

```bash
go test ./...
LICENSE_SERVICE_TOKEN=development-token go run ./cmd/license-service
```

The service listens on `:55154` by default. The important development settings
are:

| Variable | Default | Meaning |
| --- | --- | --- |
| `LICENSE_SERVICE_ADDR` | `:55154` | HTTP listen address |
| `LICENSE_SERVICE_TOKEN` | empty | Required development bearer token |
| `LICENSE_SERVICE_REQUIRE_AUTH` | `true` | Require service authentication |
| `LICENSE_SERVICE_SCENARIO` | `allow` | `allow`, `deny`, `expired`, `revoked`, `quota_exceeded`, `unavailable`, `timeout`, or `protocol_mismatch` |
| `LICENSE_SERVICE_DELAY` | `2s` | Delay used by the timeout scenario |
| `LICENSE_SERVICE_FAULT_STATUS` | `503` | HTTP status for unavailable scenarios |
| `LICENSE_SERVICE_LICENSE_ID` | `dev-license-001` | Fixture license identifier |
| `LICENSE_SERVICE_PRODUCT_ID` | `saasoptimizer` | Fixture product identifier |
| `LICENSE_SERVICE_ENTITLEMENTS` | `optimization,multi-objective,optimizer.essabpro,optimizer.ga,optimizer.random-search` | Comma-separated capability IDs; append `=false` to deny one explicitly |
| `LICENSE_SERVICE_STORAGE_MODE` | `memory` | `memory` for fixtures or `postgres` for durable activations |
| `LICENSE_SERVICE_POSTGRES_URL` | empty | PostgreSQL DSN required in `postgres` mode |
| `LICENSE_SERVICE_KEY_PROTECTION_SECRET` | empty | Secret of at least 16 bytes used to protect stored key references |

Health is public for container orchestration:

```bash
curl http://127.0.0.1:55154/v1/health
```

The service also exposes `GET /metrics` in Prometheus text format. It records
only bounded operation/outcome categories and never includes bearer tokens,
license keys, request IDs, or identity values. Use the endpoint for service
availability and scenario trends; use JSONL logs for request-level diagnosis.

License operations require the configured bearer token. Memory-mode activation
state is ephemeral and process-local. PostgreSQL mode applies the embedded
activation migration and stores only protected key references, never raw
license keys. Keep the protection secret in the deployment secret store and
reuse it across restarts. Do not use this development service for production
licensing.

## Container development

The image is intentionally non-production: it contains the development
fixtures and runs as an unprivileged user. It defaults to in-memory activation
state. Docker and rootless Podman use the same image definition. To exercise
restart persistence, provide PostgreSQL and configure the storage variables:

```bash
docker build -t saasoptimizer-license-service:dev .
docker run --rm --name saasoptimizer-license-service \
  -e LICENSE_SERVICE_TOKEN=development-token \
  -e LICENSE_SERVICE_STORAGE_MODE=postgres \
  -e LICENSE_SERVICE_POSTGRES_URL='postgres://user:password@host:5432/license_service?sslmode=disable' \
  -e LICENSE_SERVICE_KEY_PROTECTION_SECRET='development-protection-secret' \
  -p 55154:55154 \
  saasoptimizer-license-service:dev
```

With Podman, replace `docker` with `podman`; the service does not require
privileged container features.
