package service

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/uog-cose/saasoptimizer-license-service/internal/protocol"
	"github.com/uog-cose/saasoptimizer-license-service/internal/scenario"
	"github.com/uog-cose/saasoptimizer-license-service/internal/store"
)

// newScenarioServer creates a service for one explicit scenario.
func newScenarioServer(t *testing.T, kind scenario.Kind) *Server {
	t.Helper()
	config := scenario.DefaultConfig()
	config.AuthToken = "development-token"
	config.Scenario = kind
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

// TestActivationUsesStableDurableRecordSemantics verifies duplicate requests
// reuse one activation ID while returning a replay result.
func TestActivationUsesStableDurableRecordSemantics(t *testing.T) {
	server := testHTTPServer(t)
	request := protocol.Request{ProtocolVersion: protocol.Version, RequestID: "request-1", OperationID: "operation-1", IdempotencyKey: "activation-1", Operation: protocol.OperationActivate, Activation: &protocol.ActivationRequest{LicenseKey: "fixture-key", UserID: "user-1", MachineID: "machine-1", ProductID: "saasoptimizer"}}
	status, first := sendLicenseRequest(t, server, "/v1/licenses/activate", request)
	if status != http.StatusOK || first.Activation == nil || !first.Activation.IsNew {
		t.Fatalf("status=%d response=%+v", status, first)
	}
	request.RequestID = "request-2"
	request.OperationID = "operation-2"
	status, second := sendLicenseRequest(t, server, "/v1/licenses/activate", request)
	if status != http.StatusOK || second.Activation == nil || second.Activation.IsNew || second.Activation.ActivationID != first.Activation.ActivationID {
		t.Fatalf("status=%d first=%+v second=%+v", status, first, second)
	}
}

// TestActivationRejectsUnexpectedProduct verifies activation cannot bind a
// configured license to another product.
func TestActivationRejectsUnexpectedProduct(t *testing.T) {
	server := testHTTPServer(t)
	request := protocol.Request{ProtocolVersion: protocol.Version, RequestID: "request-1", OperationID: "operation-1", IdempotencyKey: "activation-1", Operation: protocol.OperationActivate, Activation: &protocol.ActivationRequest{LicenseKey: "fixture-key", UserID: "user-1", MachineID: "machine-1", ProductID: "other-product"}}
	status, response := sendLicenseRequest(t, server, "/v1/licenses/activate", request)
	if status != http.StatusForbidden || response.Error == nil || response.Error.Code != protocol.ErrorInvalidLicense {
		t.Fatalf("status=%d response=%+v", status, response)
	}
}

// TestDenyScenarioReturnsTypedLicenseError verifies explicit denial is not
// represented as a transport outage.
func TestDenyScenarioReturnsTypedLicenseError(t *testing.T) {
	server := newScenarioServer(t, scenario.KindDeny)
	request := protocol.Request{ProtocolVersion: protocol.Version, RequestID: "request-1", OperationID: "operation-1", Operation: protocol.OperationValidate, Subject: &protocol.Subject{UserID: "user-1", MachineID: "machine-1", ProductID: "saasoptimizer"}}
	status, response := sendLicenseRequest(t, server, "/v1/licenses/validate", request)
	if status != http.StatusForbidden || response.Error == nil || response.Error.Code != protocol.ErrorInvalidLicense || response.Error.Retryable {
		t.Fatalf("status=%d response=%+v", status, response)
	}
}

// TestExpiredScenarioReturnsExpiredGrant verifies expiry remains a valid
// provider result rather than becoming an outage error.
func TestExpiredScenarioReturnsExpiredGrant(t *testing.T) {
	server := newScenarioServer(t, scenario.KindExpired)
	request := protocol.Request{ProtocolVersion: protocol.Version, RequestID: "request-1", OperationID: "operation-1", Operation: protocol.OperationValidate, Subject: &protocol.Subject{UserID: "user-1", MachineID: "machine-1", ProductID: "saasoptimizer"}}
	status, response := sendLicenseRequest(t, server, "/v1/licenses/validate", request)
	if status != http.StatusOK || response.Grant == nil || response.Grant.Status != "expired" {
		t.Fatalf("status=%d response=%+v", status, response)
	}
}

// TestRefreshAndDeactivateScenariosVerifyFullOperationSurface verifies the
// non-validation operations use the same envelope and authentication rules.
func TestRefreshAndDeactivateScenariosVerifyFullOperationSurface(t *testing.T) {
	server := testHTTPServer(t)
	refresh := protocol.Request{ProtocolVersion: protocol.Version, RequestID: "request-1", OperationID: "operation-1", Operation: protocol.OperationRefresh, LicenseID: "dev-license-001"}
	status, response := sendLicenseRequest(t, server, "/v1/licenses/refresh", refresh)
	if status != http.StatusOK || response.Grant == nil || response.Grant.Capacity.MaxConcurrent != 2 {
		t.Fatalf("refresh status=%d response=%+v", status, response)
	}
	deactivate := protocol.Request{ProtocolVersion: protocol.Version, RequestID: "request-2", OperationID: "operation-2", IdempotencyKey: "deactivation-1", Operation: protocol.OperationDeactivate, Deactivation: &protocol.DeactivationRequest{LicenseID: "dev-license-001", MachineID: "machine-1"}}
	status, response = sendLicenseRequest(t, server, "/v1/licenses/deactivate", deactivate)
	if status != http.StatusOK || !response.Accepted {
		t.Fatalf("deactivate status=%d response=%+v", status, response)
	}
}

// TestRevokedAndQuotaScenariosReturnTypedDenials verifies non-retryable
// policy outcomes are distinguishable from provider outages.
func TestRevokedAndQuotaScenariosReturnTypedDenials(t *testing.T) {
	request := protocol.Request{ProtocolVersion: protocol.Version, RequestID: "request-1", OperationID: "operation-1", Operation: protocol.OperationValidate, Subject: &protocol.Subject{UserID: "user-1", MachineID: "machine-1", ProductID: "saasoptimizer"}}
	for _, testCase := range []struct {
		name string
		kind scenario.Kind
		code protocol.ErrorCode
	}{
		{name: "revoked", kind: scenario.KindRevoked, code: protocol.ErrorLicenseRevoked},
		{name: "quota", kind: scenario.KindQuotaExceeded, code: protocol.ErrorQuotaExceeded},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			server := newScenarioServer(t, testCase.kind)
			status, response := sendLicenseRequest(t, server, "/v1/licenses/validate", request)
			if status != http.StatusForbidden || response.Error == nil || response.Error.Code != testCase.code || response.Error.Retryable {
				t.Fatalf("status=%d response=%+v", status, response)
			}
		})
	}
}

// TestUnavailableScenarioIsRetryable verifies temporary outage classification.
func TestUnavailableScenarioIsRetryable(t *testing.T) {
	server := newScenarioServer(t, scenario.KindUnavailable)
	request := protocol.Request{ProtocolVersion: protocol.Version, RequestID: "request-1", OperationID: "operation-1", Operation: protocol.OperationValidate, Subject: &protocol.Subject{UserID: "user-1", MachineID: "machine-1", ProductID: "saasoptimizer"}}
	status, response := sendLicenseRequest(t, server, "/v1/licenses/validate", request)
	if status != http.StatusServiceUnavailable || response.Error == nil || response.Error.Code != protocol.ErrorUnavailable || !response.Error.Retryable {
		t.Fatalf("status=%d response=%+v", status, response)
	}
}

// TestProtocolMismatchScenarioReturnsIncompatibleVersion verifies clients can
// distinguish a compatibility failure from a valid service error response.
func TestProtocolMismatchScenarioReturnsIncompatibleVersion(t *testing.T) {
	server := newScenarioServer(t, scenario.KindProtocolMismatch)
	request := protocol.Request{ProtocolVersion: protocol.Version, RequestID: "request-1", OperationID: "operation-1", Operation: protocol.OperationValidate, Subject: &protocol.Subject{UserID: "user-1", MachineID: "machine-1", ProductID: "saasoptimizer"}}
	status, response := sendLicenseRequest(t, server, "/v1/licenses/validate", request)
	if status != http.StatusConflict || response.ProtocolVersion != "v0" || response.Error == nil || response.Error.Code != protocol.ErrorProtocolIncompatible {
		t.Fatalf("status=%d response=%+v", status, response)
	}
}

// TestTimeoutScenarioHonorsClientCancellation verifies delayed scenarios stop
// when the caller cancels instead of continuing background work.
func TestTimeoutScenarioHonorsClientCancellation(t *testing.T) {
	config := scenario.DefaultConfig()
	config.AuthToken = "development-token"
	config.Scenario = scenario.KindTimeout
	config.Delay = 100 * time.Millisecond
	fixtureStore, err := store.NewMemoryStore(config)
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(config, fixtureStore, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := protocol.Request{ProtocolVersion: protocol.Version, RequestID: "request-1", OperationID: "operation-1", Operation: protocol.OperationValidate, Subject: &protocol.Subject{UserID: "user-1", MachineID: "machine-1", ProductID: "saasoptimizer"}}
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	httpRequest := httptest.NewRequest(http.MethodPost, "/v1/licenses/validate", bytes.NewReader(payload)).WithContext(ctx)
	httpRequest.Header.Set("Authorization", "Bearer development-token")
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httpRequest)
	if recorder.Body.Len() != 0 {
		t.Fatalf("body=%s, want no response after cancellation", recorder.Body.String())
	}
}

// TestServiceLogsDoNotContainSecrets verifies operational logs contain only
// safe correlation and outcome fields.
func TestServiceLogsDoNotContainSecrets(t *testing.T) {
	config := scenario.DefaultConfig()
	config.AuthToken = "secret-service-token"
	fixtureStore, err := store.NewMemoryStore(config)
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	server, err := New(config, fixtureStore, slog.New(slog.NewTextHandler(&logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	request := protocol.Request{ProtocolVersion: protocol.Version, RequestID: "request-1", OperationID: "operation-1", IdempotencyKey: "activation-1", Operation: protocol.OperationActivate, Activation: &protocol.ActivationRequest{LicenseKey: "secret-license-key", UserID: "user-1", MachineID: "machine-1", ProductID: "saasoptimizer"}}
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	httpRequest := httptest.NewRequest(http.MethodPost, "/v1/licenses/activate", bytes.NewReader(payload))
	httpRequest.Header.Set("Authorization", "Bearer secret-service-token")
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httpRequest)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(logs.String(), "secret-service-token") || strings.Contains(logs.String(), "secret-license-key") || !strings.Contains(logs.String(), "license_request") {
		t.Fatalf("unsafe or missing logs=%s", logs.String())
	}
}
