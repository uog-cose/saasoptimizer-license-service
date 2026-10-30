-- Durable License Service activation records. Raw license keys are never stored.
CREATE TABLE IF NOT EXISTS saasoptimizer_license_activations (
    id TEXT PRIMARY KEY,
    license_id TEXT NOT NULL,
    product_id TEXT NOT NULL,
    key_reference TEXT NOT NULL,
    account_id TEXT NOT NULL DEFAULT '',
    tenant_id TEXT NOT NULL DEFAULT '',
    project_id TEXT NOT NULL DEFAULT '',
    user_id TEXT NOT NULL,
    client_id TEXT NOT NULL DEFAULT '',
    machine_id TEXT NOT NULL,
    binding_fingerprint TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('active', 'deactivated')),
    revision BIGINT NOT NULL CHECK (revision > 0),
    request_fingerprint TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    deactivated_at TIMESTAMPTZ,
    CHECK (length(trim(id)) > 0),
    CHECK (length(trim(license_id)) > 0),
    CHECK (length(trim(product_id)) > 0),
    CHECK (length(trim(key_reference)) > 0),
    CHECK (length(trim(user_id)) > 0),
    CHECK (length(trim(machine_id)) > 0),
    CHECK (length(trim(binding_fingerprint)) > 0),
    CHECK (length(trim(request_fingerprint)) > 0),
    CHECK (updated_at >= created_at),
    CHECK (deactivated_at IS NULL OR deactivated_at >= created_at),
    CHECK ((status = 'active' AND deactivated_at IS NULL)
        OR (status = 'deactivated' AND deactivated_at IS NOT NULL))
);

CREATE UNIQUE INDEX IF NOT EXISTS saasoptimizer_license_activations_active_binding_idx
    ON saasoptimizer_license_activations (license_id, product_id, binding_fingerprint)
    WHERE status = 'active';

CREATE INDEX IF NOT EXISTS saasoptimizer_license_activations_identity_idx
    ON saasoptimizer_license_activations (product_id, tenant_id, client_id, machine_id, status);

CREATE TABLE IF NOT EXISTS saasoptimizer_license_activation_operations (
    operation_type TEXT NOT NULL CHECK (operation_type IN ('activate', 'deactivate')),
    idempotency_key TEXT NOT NULL,
    request_fingerprint TEXT NOT NULL,
    activation_id TEXT NOT NULL REFERENCES saasoptimizer_license_activations(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (operation_type, idempotency_key),
    CHECK (length(trim(idempotency_key)) > 0),
    CHECK (length(trim(request_fingerprint)) > 0)
);

CREATE INDEX IF NOT EXISTS saasoptimizer_license_activation_operations_activation_idx
    ON saasoptimizer_license_activation_operations (activation_id, operation_type);
