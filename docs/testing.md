# Development License Service testing

The service is a deterministic fixture for development and CI. It is not a
production licensing authority. Every test must use a separate process or
explicitly constructed in-memory store so activation state cannot leak between
scenarios.

## Fast checks

Run these commands from this repository:

```bash
GOWORK=off go test ./...
GOWORK=off go test -race ./...
GOWORK=off go build ./cmd/license-service
```

The test suite covers the v1 envelope, authentication, idempotent activation,
health readiness, success responses, explicit denials, expiry, revocation,
quota, temporary unavailability, timeout cancellation, protocol mismatch, and
secret-free logging. The PostgreSQL repository contract test is opt-in so the
fast suite remains independent of external services.

## Durable activation test

Start a disposable PostgreSQL instance and run the repository contract test:

```bash
export LICENSE_SERVICE_POSTGRES_URL='postgres://user:password@127.0.0.1:5432/license_service?sslmode=disable'
export LICENSE_SERVICE_KEY_PROTECTION_SECRET='development-protection-secret'
GOWORK=off go test ./internal/activation/postgres -run TestStore -v
```

The test creates an activation, closes the connection pool, opens a new pool,
and verifies that the same activation can be read without storing the raw
license key. The runtime applies the same migration during startup:

```bash
LICENSE_SERVICE_STORAGE_MODE=postgres \
LICENSE_SERVICE_POSTGRES_URL="$LICENSE_SERVICE_POSTGRES_URL" \
LICENSE_SERVICE_KEY_PROTECTION_SECRET="$LICENSE_SERVICE_KEY_PROTECTION_SECRET" \
LICENSE_SERVICE_TOKEN=development-token \
GOWORK=off go run ./cmd/license-service
```

Use `LICENSE_SERVICE_STORAGE_MODE=memory` for isolated unit tests. Do not use
the in-memory mode as evidence that activation survives a process restart.

## Local service check

Start an explicitly authenticated development instance:

```bash
LICENSE_SERVICE_TOKEN=development-token \
  GOWORK=off go run ./cmd/license-service
```

Health is intentionally public:

```bash
curl --fail http://127.0.0.1:55154/v1/health
```

Operation requests require `Authorization: Bearer development-token` and the
versioned v1 envelope. Use the deployment integration test for a real
Server-to-License-Service call; do not treat a direct health response as proof
that Server remote-provider wiring works.

## Scenario matrix

Set `LICENSE_SERVICE_SCENARIO` before starting the service:

| Scenario | Expected behavior |
| --- | --- |
| `allow` | Valid grant and successful idempotent activation |
| `deny` | Non-retryable `invalid_license` |
| `expired` | Expired validation grant and `license_expired` activation failure |
| `revoked` | Non-retryable `license_revoked` |
| `quota_exceeded` | Non-retryable `quota_exceeded` |
| `unavailable` | Retryable 5xx `unavailable` |
| `timeout` | Deterministic delay honoring request cancellation |
| `protocol_mismatch` | Correlated incompatible response version |

Scenario data is process-local and disappears on shutdown. No production
license key, bearer token, or customer record belongs in this fixture.
