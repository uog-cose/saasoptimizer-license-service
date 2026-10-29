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
go run ./cmd/license-service
```

The HTTP configuration and scenario catalog will be documented as the runtime
implementation is added.
