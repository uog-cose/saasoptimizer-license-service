// Package metrics provides bounded License Service operation metrics.
package metrics

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Registry stores operation outcomes without retaining request or identity IDs.
type Registry struct {
	mu       sync.RWMutex
	requests map[string]uint64
}

// NewRegistry creates an empty License Service metrics registry.
func NewRegistry() *Registry {
	return &Registry{requests: make(map[string]uint64)}
}

// Record increments one bounded operation and outcome pair.
func (registry *Registry) Record(operation, outcome string) {
	if registry == nil || !bounded(operation) || !bounded(outcome) {
		return
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	key := operation + "\x00" + outcome
	if len(registry.requests) >= 128 {
		if _, exists := registry.requests[key]; !exists {
			return
		}
	}
	registry.requests[key]++
}

// Handler renders License Service operation outcomes in Prometheus text format.
func (registry *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte(registry.Text()))
	})
}

// Text renders all bounded operation series in deterministic order.
func (registry *Registry) Text() string {
	if registry == nil {
		return ""
	}
	registry.mu.RLock()
	keys := make([]string, 0, len(registry.requests))
	values := make(map[string]uint64, len(registry.requests))
	for key, value := range registry.requests {
		keys = append(keys, key)
		values[key] = value
	}
	registry.mu.RUnlock()
	sort.Strings(keys)
	var builder strings.Builder
	if len(keys) > 0 {
		builder.WriteString("# HELP saasoptimizer_license_requests_total License Service operation outcomes\n")
		builder.WriteString("# TYPE saasoptimizer_license_requests_total counter\n")
	}
	for _, key := range keys {
		parts := strings.SplitN(key, "\x00", 2)
		builder.WriteString(`saasoptimizer_license_requests_total{operation="` + parts[0] + `",outcome="` + parts[1] + `"} ` + strconv.FormatUint(values[key], 10) + "\n")
	}
	return builder.String()
}

// bounded rejects empty, oversized, and identity-like metric values.
func bounded(value string) bool {
	if strings.TrimSpace(value) == "" || len(value) > 64 {
		return false
	}
	for _, forbidden := range []string{"token", "secret", "password", "user", "machine", "tenant", "project", "key"} {
		if strings.Contains(strings.ToLower(value), forbidden) {
			return false
		}
	}
	return true
}
