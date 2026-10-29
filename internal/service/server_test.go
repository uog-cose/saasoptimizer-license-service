package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/uog-cose/saasoptimizer-license-service/internal/protocol"
	"github.com/uog-cose/saasoptimizer-license-service/internal/scenario"
	"github.com/uog-cose/saasoptimizer-license-service/internal/store"
)

// sendLicenseRequest invokes one authenticated service operation.
func sendLicenseRequest(t *testing.T, server *Server, path string, request protocol.Request) (int, protocol.Response) {
	t.Helper()
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	httpRequest := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(payload))
	httpRequest.Header.Set("Authorization", "Bearer development-token")
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httpRequest)
	var response protocol.Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("status=%d body=%s error=%v", recorder.Code, recorder.Body.String(), err)
	}
	return recorder.Code, response
}

// testHTTPServer creates a service with a valid development fixture.
func testHTTPServer(t *testing.T) *Server {
	t.Helper()
	config := scenario.DefaultConfig()
	config.AuthToken = "development-token"
	fixtureStore, err := store.NewMemoryStore(config)
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(config, fixtureStore, nil)
	if err != nil {
		t.Fatal(err)
	}
	return server
}

// TestHealthEndpointReportsDevelopmentBoundary verifies health is public and
// explicitly identifies the non-production environment.
func TestHealthEndpointReportsDevelopmentBoundary(t *testing.T) {
	server := testHTTPServer(t)
	request := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response protocol.HealthResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Status != "ok" || response.Environment != "development" || response.ProtocolVersion != protocol.Version {
		t.Fatalf("response=%+v", response)
	}
}

// TestLicenseEndpointRequiresAuthentication verifies service credentials are
// required independently from public health checks.
func TestLicenseEndpointRequiresAuthentication(t *testing.T) {
	server := testHTTPServer(t)
	request := httptest.NewRequest(http.MethodPost, "/v1/licenses/validate", nil)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

// TestLicenseEndpointValidatesEnvelope verifies authenticated malformed
// requests are rejected using the typed protocol response.
func TestLicenseEndpointValidatesEnvelope(t *testing.T) {
	server := testHTTPServer(t)
	request := httptest.NewRequest(http.MethodPost, "/v1/licenses/validate", nil)
	request.Header.Set("Authorization", "Bearer development-token")
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response protocol.Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Error == nil || response.Error.Code != protocol.ErrorInvalidRequest {
		t.Fatalf("response=%+v", response)
	}
}

// TestAllowScenarioServesValidationAndActivation verifies the real service
// operations return provider-compatible successful envelopes.
func TestAllowScenarioServesValidationAndActivation(t *testing.T) {
	server := testHTTPServer(t)
	validate := protocol.Request{ProtocolVersion: protocol.Version, RequestID: "request-1", OperationID: "operation-1", Operation: protocol.OperationValidate, Subject: &protocol.Subject{UserID: "user-1", MachineID: "machine-1", ProductID: "saasoptimizer"}}
	status, response := sendLicenseRequest(t, server, "/v1/licenses/validate", validate)
	if status != http.StatusOK || response.Grant == nil || response.Grant.Status != "valid" {
		t.Fatalf("status=%d response=%+v", status, response)
	}
	activate := protocol.Request{ProtocolVersion: protocol.Version, RequestID: "request-2", OperationID: "operation-2", IdempotencyKey: "activation-1", Operation: protocol.OperationActivate, Activation: &protocol.ActivationRequest{LicenseKey: "fixture-key", UserID: "user-1", MachineID: "machine-1", ProductID: "saasoptimizer"}}
	status, response = sendLicenseRequest(t, server, "/v1/licenses/activate", activate)
	if status != http.StatusOK || response.Activation == nil || response.Activation.ActivationID == "" {
		t.Fatalf("status=%d response=%+v", status, response)
	}
}

// TestDenyScenarioReturnsTypedLicenseError verifies explicit denial is not
// represented as a transport outage.
func TestDenyScenarioReturnsTypedLicenseError(t *testing.T) {
	config := scenario.DefaultConfig()
	config.AuthToken = "development-token"
	config.Scenario = scenario.KindDeny
	fixtureStore, err := store.NewMemoryStore(config)
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(config, fixtureStore, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := protocol.Request{ProtocolVersion: protocol.Version, RequestID: "request-1", OperationID: "operation-1", Operation: protocol.OperationValidate, Subject: &protocol.Subject{UserID: "user-1", MachineID: "machine-1", ProductID: "saasoptimizer"}}
	status, response := sendLicenseRequest(t, server, "/v1/licenses/validate", request)
	if status != http.StatusForbidden || response.Error == nil || response.Error.Code != protocol.ErrorInvalidLicense || response.Error.Retryable {
		t.Fatalf("status=%d response=%+v", status, response)
	}
}

// TestExpiredScenarioReturnsExpiredGrant verifies expiry remains a valid
// provider result rather than becoming an outage error.
func TestExpiredScenarioReturnsExpiredGrant(t *testing.T) {
	config := scenario.DefaultConfig()
	config.AuthToken = "development-token"
	config.Scenario = scenario.KindExpired
	fixtureStore, err := store.NewMemoryStore(config)
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(config, fixtureStore, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := protocol.Request{ProtocolVersion: protocol.Version, RequestID: "request-1", OperationID: "operation-1", Operation: protocol.OperationValidate, Subject: &protocol.Subject{UserID: "user-1", MachineID: "machine-1", ProductID: "saasoptimizer"}}
	status, response := sendLicenseRequest(t, server, "/v1/licenses/validate", request)
	if status != http.StatusOK || response.Grant == nil || response.Grant.Status != "expired" {
		t.Fatalf("status=%d response=%+v", status, response)
	}
}
