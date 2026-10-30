package metrics

import (
	"strings"
	"testing"
)

// TestRegistryRecordsOnlyBoundedOperationLabels verifies no identity leakage.
func TestRegistryRecordsOnlyBoundedOperationLabels(t *testing.T) {
	registry := NewRegistry()
	registry.Record("activate", "success")
	registry.Record("activate", "success")
	registry.Record("activate", "user-1")
	text := registry.Text()
	if !strings.Contains(text, `operation="activate",outcome="success"} 2`) || strings.Contains(text, "user-1") {
		t.Fatalf("metrics=%s", text)
	}
}
