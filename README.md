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

The service owns only development scenario evaluation and ephemeral test
activation state. Production activation persistence, billing, key issuance,
and customer administration are outside this repository.

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

Health is public for container orchestration:

```bash
curl http://127.0.0.1:55154/v1/health
```

License operations require the configured bearer token. The fixture store is
ephemeral and process-local; do not use this service for production licensing.
