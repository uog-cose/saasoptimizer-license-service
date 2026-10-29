// Package service implements the development License Service HTTP boundary.
package service

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/uog-cose/saasoptimizer-license-service/internal/protocol"
	"github.com/uog-cose/saasoptimizer-license-service/internal/scenario"
	"github.com/uog-cose/saasoptimizer-license-service/internal/store"
)

const maxRequestBodyBytes = 1 << 20

var errOperationNotReady = errors.New("license operation is not implemented")

// Server exposes the deterministic License Service over HTTP.
type Server struct {
	config scenario.Config
	store  *store.MemoryStore
	logger *slog.Logger
}

// New creates an HTTP service with one isolated validated scenario store.
func New(config scenario.Config, fixtureStore *store.MemoryStore, logger *slog.Logger) (*Server, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if fixtureStore == nil {
		return nil, ErrStoreRequired
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{config: config, store: fixtureStore, logger: logger}, nil
}

// Handler returns the HTTP handler for health and License Service endpoints.
func (server *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/health", server.handleHealth)
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
	writeJSON(writer, http.StatusOK, protocol.HealthResponse{Status: "ok", Service: "saasoptimizer-license-service", Environment: "development", ProtocolVersion: server.config.ResponseProtocolVersion})
}

// handleOperation authenticates, decodes, and validates one License request.
func (server *Server) handleOperation(expected protocol.Operation, writer http.ResponseWriter, request *http.Request) {
	requestID := strings.TrimSpace(request.Header.Get("X-Request-ID"))
	operationID := strings.TrimSpace(request.Header.Get("X-Operation-ID"))
	if request.Method != http.MethodPost {
		writeProtocolError(writer, server.responseVersion(), requestID, operationID, protocol.ErrorInvalidRequest, false, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !server.authorized(request) {
		writeProtocolError(writer, server.responseVersion(), requestID, operationID, protocol.ErrorAuthenticationFailed, false, "service authentication failed", http.StatusUnauthorized)
		return
	}
	decoder := json.NewDecoder(io.LimitReader(request.Body, maxRequestBodyBytes+1))
	var envelope protocol.Request
	if err := decoder.Decode(&envelope); err != nil {
		writeProtocolError(writer, server.responseVersion(), requestID, operationID, protocol.ErrorInvalidRequest, false, "invalid request envelope", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeProtocolError(writer, server.responseVersion(), envelope.RequestID, envelope.OperationID, protocol.ErrorInvalidRequest, false, "request body is too large or contains trailing data", http.StatusBadRequest)
		return
	}
	requestID = envelope.RequestID
	operationID = envelope.OperationID
	if envelope.Operation != expected {
		writeProtocolError(writer, server.responseVersion(), requestID, operationID, protocol.ErrorInvalidRequest, false, "operation does not match endpoint", http.StatusBadRequest)
		return
	}
	if err := envelope.Validate(); err != nil {
		code := protocol.ErrorInvalidRequest
		status := http.StatusBadRequest
		if errors.Is(err, protocol.ErrProtocolIncompatible) {
			code = protocol.ErrorProtocolIncompatible
			status = http.StatusConflict
		}
		writeProtocolError(writer, server.responseVersion(), requestID, operationID, code, false, "request is not compatible", status)
		return
	}
	if server.config.Scenario == scenario.KindProtocolMismatch {
		writeProtocolError(writer, "v0", requestID, operationID, protocol.ErrorProtocolIncompatible, false, "protocol version is not supported", http.StatusConflict)
		return
	}
	writeProtocolError(writer, server.responseVersion(), requestID, operationID, protocol.ErrorInternal, false, errOperationNotReady.Error(), http.StatusNotImplemented)
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
