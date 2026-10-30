package migrations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestActivationMigrationProtectsSecretAndBindingInvariants verifies the
// migration contains the durable constraints required by Issue 177.
func TestActivationMigrationProtectsSecretAndBindingInvariants(t *testing.T) {
	path := filepath.Join("00001_activation_records.up.sql")
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	schema := string(payload)
	for _, required := range []string{
		"saasoptimizer_license_activations",
		"key_reference",
		"binding_fingerprint",
		"status IN ('active', 'deactivated')",
		"revision BIGINT NOT NULL CHECK (revision > 0)",
		"saasoptimizer_license_activations_active_binding_idx",
		"saasoptimizer_license_activation_operations",
		"PRIMARY KEY (operation_type, idempotency_key)",
	} {
		if !strings.Contains(schema, required) {
			t.Fatalf("migration does not contain %q", required)
		}
	}
	if strings.Contains(schema, "license_key TEXT") || strings.Contains(schema, "license_key VARCHAR") {
		t.Fatal("migration stores a raw license key")
	}
}
