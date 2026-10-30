package migrations

import "testing"

// TestEmbeddedMigrationsExposeCurrentVersion verifies the runtime migration
// package includes the schema expected by the PostgreSQL store.
func TestEmbeddedMigrationsExposeCurrentVersion(t *testing.T) {
	payload, err := Files.ReadFile("00001_activation_records.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) == 0 || currentVersion != 1 {
		t.Fatalf("payload length=%d version=%d", len(payload), currentVersion)
	}
}
