// Package service implements the development License Service HTTP boundary.
package service

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/uog-cose/saasoptimizer-license-service/internal/activation"
	"github.com/uog-cose/saasoptimizer-license-service/internal/keyref"
	"github.com/uog-cose/saasoptimizer-license-service/internal/protocol"
	"github.com/uog-cose/saasoptimizer-license-service/internal/scenario"
	"github.com/uog-cose/saasoptimizer-license-service/internal/store"
)

const maxRequestBodyBytes = 1 << 20

var errOperationNotReady = errors.New("license operation is not implemented")

// Server exposes the deterministic License Service over HTTP.
type Server struct {
	config          scenario.Config
	store           *store.MemoryStore
	activationStore activation.Store
	keyProtector    KeyProtector
	logger          *slog.Logger
}

// KeyProtector converts a raw license key into a safe persistence reference.
type KeyProtector interface {
	// Protect returns a non-reversible reference without retaining the raw key.
	Protect(string) (string, error)
}

// New creates an HTTP service with one isolated validated scenario store.
func New(config scenario.Config, fixtureStore *store.MemoryStore, logger *slog.Logger) (*Server, error) {
	return NewWithActivationStore(config, fixtureStore, activation.NewMemoryActivationStore(), nil, logger)
}

// NewWithActivationStore creates an HTTP service with an explicit activation
// persistence backend and key protection boundary.
func NewWithActivationStore(config scenario.Config, fixtureStore *store.MemoryStore, activationStore activation.Store, protector KeyProtector, logger *slog.Logger) (*Server, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if fixtureStore == nil {
		return nil, ErrStoreRequired
	}
	if activationStore == nil {
		return nil, ErrActivationStoreRequired
	}
	if protector == nil {
		var err error
		secret := config.KeyProtectionSecret
		if secret == "" {
			secret = "development-only-key-protection-secret"
		}
		protector, err = keyref.NewProtector(secret)
		if err != nil {
			return nil, fmt.Errorf("create license key protector: %w", err)
		}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{config: config, store: fixtureStore, activationStore: activationStore, keyProtector: protector, logger: logger}, nil
}

// Handler returns the HTTP handler for health and License Service endpoints.
func (server *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/health", server.handleHealth)
	mux.HandleFunc("/v1/health/live", server.handleHealthLive)
	mux.HandleFunc("/v1/health/ready", server.handleHealthReady)
	mux.HandleFunc("/v1/licenses/validate", func(writer http.ResponseWriter, request *http.Request) {
		server.handleOperation(protocol.OperationValidate, writer, request)
	})
	mux.HandleFunc("/v1/licenses/refresh", func(writer http.ResponseWriter, request *http.Request) {
		server.handleOperation(protocol.OperationRefresh, writer, request)
	})
	mux.HandleFunc("/v1/licenses/activate", func(writer http.ResponseWriter, request *http.Request) {
		server.handleOperation(protocol.OperationActivate, writer, request)
	})
	mux.HandleFunc("/v1/licenses/deactivate", func(writer http.ResponseWriter, request *http.Request) {
		server.handleOperation(protocol.OperationDeactivate, writer, request)
	})
	return mux
}

// handleHealth reports process readiness without requiring service credentials.
func (server *Server) handleHealth(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeJSON(writer, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	writeJSON(writer, http.StatusOK, server.healthResponse("liveness", "ok"))
}

// handleHealthLive reports that the License Service process is alive.
func (server *Server) handleHealthLive(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeJSON(writer, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	writeJSON(writer, http.StatusOK, server.healthResponse("liveness", "ok"))
}

// handleHealthReady reports whether the activation store is available for work.
func (server *Server) handleHealthReady(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeJSON(writer, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	status := "ok"
	dependencyStatus := "ok"
	detail := ""
	if server.activationStore == nil {
		status = "not_ready"
		dependencyStatus = "not_ready"
		detail = "activation store unavailable"
	}
	response := server.healthResponse("readiness", status)
	response.Dependencies = []protocol.HealthDependency{{Name: "activation-store", Status: dependencyStatus, Required: true, Detail: detail}}
	writeJSON(writer, healthHTTPStatus(status), response)
}

// healthResponse creates the stable health payload shared by service endpoints.
func (server *Server) healthResponse(checkType, status string) protocol.HealthResponse {
	return protocol.HealthResponse{Status: status, Service: "saasoptimizer-license-service", Environment: "development", ProtocolVersion: server.config.ResponseProtocolVersion, CheckType: checkType}
}

// healthHTTPStatus maps readiness failure to the orchestrator status code.
func healthHTTPStatus(status string) int {
	if status == "starting" || status == "not_ready" || status == "failed" {
		return http.StatusServiceUnavailable
	}
	return http.StatusOK
}

// handleOperation authenticates, decodes, and validates one License request.
func (server *Server) handleOperation(expected protocol.Operation, writer http.ResponseWriter, request *http.Request) {
	requestID := strings.TrimSpace(request.Header.Get("X-Request-ID"))
	operationID := strings.TrimSpace(request.Header.Get("X-Operation-ID"))
	respondError := func(version string, code protocol.ErrorCode, retryable bool, message string, status int) {
		server.logRequest(string(expected), requestID, operationID, status, code)
		writeProtocolError(writer, version, requestID, operationID, code, retryable, message, status)
	}
	if request.Method != http.MethodPost {
		respondError(server.responseVersion(), protocol.ErrorInvalidRequest, false, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !server.authorized(request) {
		respondError(server.responseVersion(), protocol.ErrorAuthenticationFailed, false, "service authentication failed", http.StatusUnauthorized)
		return
	}
	decoder := json.NewDecoder(io.LimitReader(request.Body, maxRequestBodyBytes+1))
	var envelope protocol.Request
	if err := decoder.Decode(&envelope); err != nil {
		respondError(server.responseVersion(), protocol.ErrorInvalidRequest, false, "invalid request envelope", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		requestID = envelope.RequestID
		operationID = envelope.OperationID
		respondError(server.responseVersion(), protocol.ErrorInvalidRequest, false, "request body is too large or contains trailing data", http.StatusBadRequest)
		return
	}
	requestID = envelope.RequestID
	operationID = envelope.OperationID
	if envelope.Operation != expected {
		respondError(server.responseVersion(), protocol.ErrorInvalidRequest, false, "operation does not match endpoint", http.StatusBadRequest)
		return
	}
	if err := envelope.Validate(); err != nil {
		code := protocol.ErrorInvalidRequest
		status := http.StatusBadRequest
		if errors.Is(err, protocol.ErrProtocolIncompatible) {
			code = protocol.ErrorProtocolIncompatible
			status = http.StatusConflict
		}
		respondError(server.responseVersion(), code, false, "request is not compatible", status)
		return
	}
	if server.config.Scenario == scenario.KindProtocolMismatch {
		respondError("v0", protocol.ErrorProtocolIncompatible, false, "protocol version is not supported", http.StatusConflict)
		return
	}
	if !server.waitForScenario(request.Context()) {
		return
	}
	response, status, code, retryable, message := server.execute(request.Context(), envelope)
	if code != "" {
		respondError(server.responseVersion(), code, retryable, message, status)
		return
	}
	server.logRequest(string(envelope.Operation), requestID, operationID, status, "")
	writeJSON(writer, status, response)
}

// authorized verifies the configured service bearer token in constant time.
func (server *Server) authorized(request *http.Request) bool {
	if !server.config.RequireAuth {
		return true
	}
	provided := strings.TrimSpace(request.Header.Get("Authorization"))
	expected := "Bearer " + server.config.AuthToken
	if len(provided) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

// responseVersion returns the configured response protocol version.
func (server *Server) responseVersion() string {
	if strings.TrimSpace(server.config.ResponseProtocolVersion) == "" {
		return protocol.Version
	}
	return server.config.ResponseProtocolVersion
}

// logRequest records bounded operational metadata without request secrets.
func (server *Server) logRequest(operation, requestID, operationID string, status int, code protocol.ErrorCode) {
	server.logger.Info("license request completed", "event", "license_request", "operation", operation, "request_id", requestID, "operation_id", operationID, "scenario", string(server.config.Scenario), "status", status, "error_code", string(code))
}

// waitForScenario applies deterministic delay and stops cleanly on request
// cancellation so timeout tests do not leak goroutines.
func (server *Server) waitForScenario(ctx context.Context) bool {
	if server.config.Scenario != scenario.KindTimeout || server.config.Delay <= 0 {
		return true
	}
	timer := time.NewTimer(server.config.Delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		server.logger.Debug("license scenario request canceled", "event", "request_canceled")
		return false
	}
}

// execute evaluates one validated request against the configured scenario.
func (server *Server) execute(ctx context.Context, request protocol.Request) (protocol.Response, int, protocol.ErrorCode, bool, string) {
	base := protocol.Response{ProtocolVersion: server.responseVersion(), RequestID: request.RequestID, OperationID: request.OperationID}
	if server.config.Scenario == scenario.KindUnavailable {
		status := server.config.FaultStatus
		if status == 0 {
			status = http.StatusServiceUnavailable
		}
		return base, status, protocol.ErrorUnavailable, true, "development license service is unavailable"
	}
	if server.config.Scenario == scenario.KindDeny && request.Operation != protocol.OperationDeactivate {
		return base, http.StatusForbidden, protocol.ErrorInvalidLicense, false, "development license is denied"
	}
	if server.config.Scenario == scenario.KindQuotaExceeded && request.Operation != protocol.OperationDeactivate {
		return base, http.StatusForbidden, protocol.ErrorQuotaExceeded, false, "development license quota is exceeded"
	}
	if server.config.Scenario == scenario.KindRevoked && request.Operation != protocol.OperationDeactivate {
		return base, http.StatusForbidden, protocol.ErrorLicenseRevoked, false, "development license is revoked"
	}
	if server.config.Scenario == scenario.KindExpired && request.Operation == protocol.OperationActivate {
		return base, http.StatusForbidden, protocol.ErrorLicenseExpired, false, "development license is expired"
	}
	switch request.Operation {
	case protocol.OperationValidate, protocol.OperationRefresh:
		grant := server.store.Grant()
		base.Grant = &grant
		return base, http.StatusOK, "", false, ""
	case protocol.OperationActivate:
		if request.Activation.ProductID != server.config.ProductID {
			return base, http.StatusForbidden, protocol.ErrorInvalidLicense, false, "development license product is not authorized"
		}
		keyReference, err := server.keyProtector.Protect(request.Activation.LicenseKey)
		if err != nil {
			return base, http.StatusBadRequest, protocol.ErrorInvalidRequest, false, "activation key cannot be protected"
		}
		fingerprint, err := activationRequestFingerprint(*request.Activation, keyReference)
		if err != nil {
			return base, http.StatusInternalServerError, protocol.ErrorInternal, false, "activation request cannot be fingerprinted"
		}
		activationResult, created, err := server.activationStore.Create(ctx, activation.CreateRequest{
			IdempotencyKey:     request.IdempotencyKey,
			RequestFingerprint: fingerprint,
			LicenseID:          server.config.LicenseID,
			ProductID:          request.Activation.ProductID,
			KeyReference:       keyReference,
			Binding: activation.IdentityBinding{
				AccountID: request.Activation.AccountID,
				TenantID:  request.Activation.TenantID,
				ProjectID: request.Activation.ProjectID,
				UserID:    request.Activation.UserID,
				ClientID:  request.Activation.ClientID,
				MachineID: request.Activation.MachineID,
			},
			At: server.config.Clock().UTC(),
		})
		if err != nil {
			return base, activationStoreErrorStatus(err), activationStoreErrorCode(err), false, "activation request cannot be applied"
		}
		grant := server.store.Grant()
		base.Activation = &protocol.Activation{ActivationID: activationResult.ID, Grant: &grant, IsNew: created}
		return base, http.StatusOK, "", false, ""
	case protocol.OperationDeactivate:
		if request.Deactivation.LicenseID != server.config.LicenseID {
			return base, http.StatusForbidden, protocol.ErrorInvalidLicense, false, "development license is not authorized"
		}
		activationID := request.Deactivation.ActivationID
		if activationID == "" {
			if request.Deactivation.UserID == "" {
				base.Accepted = true
				return base, http.StatusOK, "", false, ""
			}
			binding := activation.IdentityBinding{AccountID: request.Deactivation.AccountID, TenantID: request.Deactivation.TenantID, ProjectID: request.Deactivation.ProjectID, UserID: request.Deactivation.UserID, ClientID: request.Deactivation.ClientID, MachineID: request.Deactivation.MachineID}
			record, found, err := server.activationStore.FindActive(ctx, request.Deactivation.LicenseID, server.config.ProductID, binding)
			if err != nil {
				return base, activationStoreErrorStatus(err), activationStoreErrorCode(err), false, "deactivation request cannot be applied"
			}
			if !found {
				base.Accepted = true
				return base, http.StatusOK, "", false, ""
			}
			activationID = record.ID
		}
		fingerprint, err := deactivationRequestFingerprint(*request.Deactivation, activationID)
		if err != nil {
			return base, http.StatusInternalServerError, protocol.ErrorInternal, false, "deactivation request cannot be fingerprinted"
		}
		if _, _, err := server.activationStore.Deactivate(ctx, activation.DeactivateRequest{IdempotencyKey: request.IdempotencyKey, RequestFingerprint: fingerprint, ActivationID: activationID, At: server.config.Clock().UTC()}); err != nil {
			return base, activationStoreErrorStatus(err), activationStoreErrorCode(err), false, "deactivation request cannot be applied"
		}
		base.Accepted = true
		return base, http.StatusOK, "", false, ""
	default:
		return base, http.StatusNotImplemented, protocol.ErrorInternal, false, errOperationNotReady.Error()
	}
}

// storeErrorCode maps ephemeral store failures to protocol errors.
func storeErrorCode(err error) protocol.ErrorCode {
	if errors.Is(err, store.ErrIdempotencyConflict) {
		return protocol.ErrorConflict
	}
	if errors.Is(err, store.ErrCapacityExceeded) {
		return protocol.ErrorQuotaExceeded
	}
	return protocol.ErrorInternal
}

// storeErrorStatus maps ephemeral store failures to HTTP status classes.
func storeErrorStatus(err error) int {
	if errors.Is(err, store.ErrIdempotencyConflict) {
		return http.StatusConflict
	}
	if errors.Is(err, store.ErrCapacityExceeded) {
		return http.StatusForbidden
	}
	return http.StatusInternalServerError
}

// activationStoreErrorCode maps durable activation failures to protocol errors.
func activationStoreErrorCode(err error) protocol.ErrorCode {
	if errors.Is(err, activation.ErrIdempotencyConflict) || errors.Is(err, activation.ErrBindingConflict) || errors.Is(err, activation.ErrRevisionConflict) {
		return protocol.ErrorConflict
	}
	if errors.Is(err, activation.ErrNotFound) {
		return protocol.ErrorInvalidLicense
	}
	return protocol.ErrorInternal
}

// activationStoreErrorStatus maps durable activation failures to HTTP statuses.
func activationStoreErrorStatus(err error) int {
	if errors.Is(err, activation.ErrIdempotencyConflict) || errors.Is(err, activation.ErrBindingConflict) || errors.Is(err, activation.ErrRevisionConflict) {
		return http.StatusConflict
	}
	if errors.Is(err, activation.ErrNotFound) {
		return http.StatusForbidden
	}
	return http.StatusInternalServerError
}

// activationRequestFingerprint creates a non-secret digest for activation
// idempotency and durable conflict detection.
func activationRequestFingerprint(request protocol.ActivationRequest, keyReference string) (string, error) {
	payload, err := json.Marshal(struct {
		AccountID    string `json:"account_id"`
		MachineID    string `json:"machine_id"`
		UserID       string `json:"user_id"`
		TenantID     string `json:"tenant_id"`
		ProjectID    string `json:"project_id"`
		ClientID     string `json:"client_id"`
		ProductID    string `json:"product_id"`
		KeyReference string `json:"key_reference"`
	}{request.AccountID, request.MachineID, request.UserID, request.TenantID, request.ProjectID, request.ClientID, request.ProductID, keyReference})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

// deactivationRequestFingerprint creates a non-secret digest for deactivation
// idempotency and durable conflict detection.
func deactivationRequestFingerprint(request protocol.DeactivationRequest, activationID string) (string, error) {
	payload, err := json.Marshal(struct {
		AccountID    string `json:"account_id"`
		LicenseID    string `json:"license_id"`
		ActivationID string `json:"activation_id"`
		TenantID     string `json:"tenant_id"`
		ProjectID    string `json:"project_id"`
		ClientID     string `json:"client_id"`
		UserID       string `json:"user_id"`
		MachineID    string `json:"machine_id"`
	}{request.AccountID, request.LicenseID, activationID, request.TenantID, request.ProjectID, request.ClientID, request.UserID, request.MachineID})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

// writeJSON writes one bounded JSON response with a stable content type.
func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

// writeProtocolError writes a correlated typed error without exposing internal
// error text or service credentials.
func writeProtocolError(writer http.ResponseWriter, version, requestID, operationID string, code protocol.ErrorCode, retryable bool, message string, status int) {
	response := protocol.Response{ProtocolVersion: version, RequestID: requestID, OperationID: operationID, Error: &protocol.Error{Code: code, Retryable: retryable, Message: strings.TrimSpace(message)}}
	if response.Error.Message == "" {
		response.Error.Message = fmt.Sprintf("license service error: %s", code)
	}
	writeJSON(writer, status, response)
}
