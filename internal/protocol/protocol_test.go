package protocol

import (
	"encoding/json"
	"errors"
	"testing"
)

// TestRequestValidationRequiresSideEffectIdempotency verifies activation
// requests cannot enter the service without a stable idempotency key.
func TestRequestValidationRequiresSideEffectIdempotency(t *testing.T) {
	request := Request{ProtocolVersion: Version, RequestID: "request-1", OperationID: "operation-1", Operation: OperationActivate, Activation: &ActivationRequest{LicenseKey: "fixture-key", UserID: "user-1", MachineID: "machine-1", ProductID: "saasoptimizer"}}
	if err := request.Validate(); err == nil {
		t.Fatal("Validate() accepted activation without idempotency key")
	}
	request.IdempotencyKey = "activation-1"
	if err := request.Validate(); err != nil {
		t.Fatalf("Validate() error=%v", err)
	}
}

// TestProtocolJSONRoundTripPreservesScope verifies the service wire shape
// retains complete identity and grant data.
func TestProtocolJSONRoundTripPreservesScope(t *testing.T) {
	original := Request{ProtocolVersion: Version, RequestID: "request-1", OperationID: "operation-1", Operation: OperationValidate, Subject: &Subject{AccountID: "account-1", UserID: "user-1", TenantID: "tenant-1", ProjectID: "project-1", ClientID: "client-1", MachineID: "machine-1", ProductID: "saasoptimizer"}}
	payload, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Request
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatal(err)
	}
	if decoded.Subject == nil || decoded.Subject.TenantID != "tenant-1" || decoded.Subject.MachineID != "machine-1" {
		t.Fatalf("decoded=%+v", decoded)
	}
}

// TestResponseValidationRejectsMixedPayload verifies an error cannot be
// combined with a successful grant.
func TestResponseValidationRejectsMixedPayload(t *testing.T) {
	response := Response{ProtocolVersion: Version, RequestID: "request-1", OperationID: "operation-1", Grant: &Grant{LicenseID: "license-1", Status: "valid", Product: "saasoptimizer"}, Error: &Error{Code: ErrorInternal}}
	if err := response.Validate(); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("Validate() error=%v, want ErrInvalidResponse", err)
	}
}
