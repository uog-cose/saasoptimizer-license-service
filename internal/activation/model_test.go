package activation

import (
	"strings"
	"testing"
	"time"
)

// validCreateRequest returns a deterministic valid activation request.
func validCreateRequest() CreateRequest {
	at := time.Date(2026, time.October, 30, 10, 0, 0, 0, time.UTC)
	return CreateRequest{
		IdempotencyKey:     "activation-operation-1",
		RequestFingerprint: "request-fingerprint-1",
		LicenseID:          "license-1",
		ProductID:          "saasoptimizer",
		KeyReference:       "hmac-sha256:key-1",
		Binding:            IdentityBinding{AccountID: "account-1", TenantID: "tenant-1", ProjectID: "project-1", UserID: "user-1", ClientID: "client-1", MachineID: "machine-1"},
		At:                 at,
	}
}

// TestCreateRequestRequiresProtectedKeyReference verifies raw key data is not
// accepted as a durable activation field.
func TestCreateRequestRequiresProtectedKeyReference(t *testing.T) {
	request := validCreateRequest()
	request.KeyReference = ""
	if err := request.Validate(); err == nil {
		t.Fatal("Validate accepted an empty key reference")
	}
}

// TestIdentityBindingFingerprintIsStableAndSecretFree verifies the binding key
// is deterministic and does not contain the original identity values.
func TestIdentityBindingFingerprintIsStableAndSecretFree(t *testing.T) {
	binding := validCreateRequest().Binding
	first, err := binding.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	second, err := binding.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if first != second || first == "" {
		t.Fatalf("first=%q second=%q", first, second)
	}
	for _, value := range []string{binding.AccountID, binding.TenantID, binding.ProjectID, binding.UserID, binding.ClientID, binding.MachineID} {
		if strings.Contains(first, value) {
			t.Fatalf("fingerprint=%q contains identity value %q", first, value)
		}
	}
}

// TestDeactivateIsIdempotent verifies repeated deactivation does not advance
// the revision or mutate the terminal timestamp.
func TestDeactivateIsIdempotent(t *testing.T) {
	request := validCreateRequest()
	record := Record{ID: "activation-1", LicenseID: request.LicenseID, ProductID: request.ProductID, KeyReference: request.KeyReference, Binding: request.Binding, Status: StatusActive, Revision: 1, RequestFingerprint: request.RequestFingerprint, CreatedAt: request.At, UpdatedAt: request.At}
	first, err := record.Deactivate(request.At.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	second, err := first.Deactivate(request.At.Add(2 * time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if first.DeactivatedAt == nil || second.DeactivatedAt == nil || second.Revision != first.Revision || !second.UpdatedAt.Equal(first.UpdatedAt) || !second.DeactivatedAt.Equal(*first.DeactivatedAt) {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
}
