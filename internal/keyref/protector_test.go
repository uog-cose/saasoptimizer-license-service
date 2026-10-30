package keyref

import (
	"strings"
	"testing"
)

// TestProtectorCreatesStableSecretFreeReference verifies the protected format.
func TestProtectorCreatesStableSecretFreeReference(t *testing.T) {
	protector, err := NewProtector("development-protection-secret")
	if err != nil {
		t.Fatal(err)
	}
	first, err := protector.Protect("fixture-license-key")
	if err != nil {
		t.Fatal(err)
	}
	second, err := protector.Protect("fixture-license-key")
	if err != nil {
		t.Fatal(err)
	}
	if first != second || !strings.HasPrefix(first, AlgorithmHMACSHA256+":") {
		t.Fatalf("first=%q second=%q", first, second)
	}
	if strings.Contains(first, "fixture-license-key") || strings.Contains(first, "development-protection-secret") {
		t.Fatalf("reference exposes secret material: %q", first)
	}
}

// TestProtectorSeparatesDifferentKeys verifies different raw keys do not share
// a protected reference under the same secret.
func TestProtectorSeparatesDifferentKeys(t *testing.T) {
	protector, err := NewProtector("development-protection-secret")
	if err != nil {
		t.Fatal(err)
	}
	first, err := protector.Protect("fixture-license-key-1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := protector.Protect("fixture-license-key-2")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("different keys produced reference %q", first)
	}
}

// TestProtectorRejectsWeakConfiguration verifies accidental weak secrets fail
// during configuration rather than during a license request.
func TestProtectorRejectsWeakConfiguration(t *testing.T) {
	if _, err := NewProtector("short"); err == nil {
		t.Fatal("NewProtector accepted a weak secret")
	}
}
