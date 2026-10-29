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
secret-free logging.

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
