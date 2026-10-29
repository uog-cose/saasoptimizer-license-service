package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/uog-cose/saasoptimizer-license-service/internal/protocol"
	"github.com/uog-cose/saasoptimizer-license-service/internal/scenario"
	"github.com/uog-cose/saasoptimizer-license-service/internal/store"
)

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
