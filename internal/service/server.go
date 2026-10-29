// Package service implements the development License Service HTTP boundary.
package service

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/uog-cose/saasoptimizer-license-service/internal/protocol"
	"github.com/uog-cose/saasoptimizer-license-service/internal/scenario"
	"github.com/uog-cose/saasoptimizer-license-service/internal/store"
)

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
	mux.HandleFunc("/v1/licenses/validate", server.handleNotReady)
	mux.HandleFunc("/v1/licenses/refresh", server.handleNotReady)
	mux.HandleFunc("/v1/licenses/activate", server.handleNotReady)
	mux.HandleFunc("/v1/licenses/deactivate", server.handleNotReady)
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

// handleNotReady makes the unfinished operation boundary explicit during the
// incremental implementation instead of silently accepting requests.
func (server *Server) handleNotReady(writer http.ResponseWriter, request *http.Request) {
	writeJSON(writer, http.StatusNotImplemented, map[string]string{"error": "license operation is not implemented"})
}

// writeJSON writes one bounded JSON response with a stable content type.
func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
