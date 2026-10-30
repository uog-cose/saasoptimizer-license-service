package scenario

import "testing"

// TestPostgresStorageRequiresDurableConfiguration verifies durable mode cannot
// accidentally start without its database and protection secret.
func TestPostgresStorageRequiresDurableConfiguration(t *testing.T) {
	config := DefaultConfig()
	config.AuthToken = "development-service-token"
	config.StorageMode = StorageModePostgres
	if err := config.Validate(); err == nil {
		t.Fatal("Validate accepted PostgreSQL mode without configuration")
	}
	config.PostgresURL = "postgres://license-service"
	if err := config.Validate(); err == nil {
		t.Fatal("Validate accepted PostgreSQL mode without protection secret")
	}
	config.KeyProtectionSecret = "development-protection-secret"
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
}
