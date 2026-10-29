// Package scenario defines deterministic development License Service scenarios.
package scenario

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/uog-cose/saasoptimizer-license-service/internal/protocol"
)

const (
	// EnvAddress configures the HTTP listen address.
	EnvAddress = "LICENSE_SERVICE_ADDR"
	// EnvToken configures the development service token.
	EnvToken = "LICENSE_SERVICE_TOKEN"
	// EnvRequireAuth controls whether the service requires a bearer token.
	EnvRequireAuth = "LICENSE_SERVICE_REQUIRE_AUTH"
	// EnvProtocolVersion configures the accepted request protocol version.
	EnvProtocolVersion = "LICENSE_SERVICE_PROTOCOL_VERSION"
	// EnvResponseProtocolVersion configures the response protocol version.
	EnvResponseProtocolVersion = "LICENSE_SERVICE_RESPONSE_PROTOCOL_VERSION"
	// EnvScenario selects the deterministic behavior.
	EnvScenario = "LICENSE_SERVICE_SCENARIO"
	// EnvDelay configures deterministic response delay.
	EnvDelay = "LICENSE_SERVICE_DELAY"
	// EnvExpiresAt configures the grant expiry timestamp.
	EnvExpiresAt = "LICENSE_SERVICE_EXPIRES_AT"
	// EnvFaultStatus configures an HTTP failure status for fault scenarios.
	EnvFaultStatus = "LICENSE_SERVICE_FAULT_STATUS"
	// EnvLicenseID configures the fixture license identifier.
	EnvLicenseID = "LICENSE_SERVICE_LICENSE_ID"
	// EnvProductID configures the fixture product identifier.
	EnvProductID = "LICENSE_SERVICE_PRODUCT_ID"
	// EnvFeatures configures comma-separated feature names.
	EnvFeatures = "LICENSE_SERVICE_FEATURES"
	// EnvMaxConcurrent configures the fixture concurrent capacity.
	EnvMaxConcurrent = "LICENSE_SERVICE_MAX_CONCURRENT"
	// EnvMaxActivations configures the fixture activation capacity.
	EnvMaxActivations = "LICENSE_SERVICE_MAX_ACTIVATIONS"
)

// Kind identifies one deterministic service scenario.
type Kind string

const (
	// KindAllow returns valid grants and successful side effects.
	KindAllow Kind = "allow"
	// KindDeny returns an invalid-license denial.
	KindDeny Kind = "deny"
	// KindExpired returns an expired grant.
	KindExpired Kind = "expired"
	// KindRevoked returns a revoked-license denial.
	KindRevoked Kind = "revoked"
	// KindQuotaExceeded returns a quota denial.
	KindQuotaExceeded Kind = "quota_exceeded"
	// KindUnavailable returns a retryable service outage.
	KindUnavailable Kind = "unavailable"
	// KindTimeout delays until the caller timeout or returns after the delay.
	KindTimeout Kind = "timeout"
	// KindProtocolMismatch returns an unsupported response protocol version.
	KindProtocolMismatch Kind = "protocol_mismatch"
)

// Config contains all deterministic service and fixture settings.
type Config struct {
	// Address is the HTTP listen address.
	Address string
	// AuthToken is the bearer token accepted by the service.
	AuthToken string
	// RequireAuth controls whether Authorization is mandatory.
	RequireAuth bool
	// ProtocolVersion is the accepted request protocol version.
	ProtocolVersion string
	// ResponseProtocolVersion is the normal response protocol version.
	ResponseProtocolVersion string
	// Scenario selects the behavior returned by all operations.
	Scenario Kind
	// Delay is applied before a response for delay and timeout scenarios.
	Delay time.Duration
	// FaultStatus is the HTTP status returned by unavailable scenarios.
	FaultStatus int
	// LicenseID identifies the deterministic fixture license.
	LicenseID string
	// ProductID identifies the deterministic fixture product.
	ProductID string
	// ExpiresAt is the configured valid-grant expiry. Zero uses the clock.
	ExpiresAt time.Time
	// Features contains the fixture feature names.
	Features []string
	// Capacity contains the fixture execution limits.
	Capacity protocol.Capacity
	// Clock supplies the current time for deterministic tests.
	Clock func() time.Time
}

// DefaultConfig returns a non-production configuration with authentication
// enabled and a deterministic development fixture.
func DefaultConfig() Config {
	return Config{
		Address:                 ":55154",
		RequireAuth:             true,
		ProtocolVersion:         protocol.Version,
		ResponseProtocolVersion: protocol.Version,
		Scenario:                KindAllow,
		Delay:                   2 * time.Second,
		FaultStatus:             503,
		LicenseID:               "dev-license-001",
		ProductID:               "saasoptimizer",
		Features:                []string{"optimization", "multi-objective"},
		Capacity:                protocol.Capacity{MaxConcurrent: 2, MaxActivations: 4},
		Clock:                   time.Now,
	}
}

// Validate checks configuration completeness and prevents accidental
// production-like or malformed fixture behavior.
func (config Config) Validate() error {
	if strings.TrimSpace(config.Address) == "" {
		return fmt.Errorf("license service address is required")
	}
	if config.RequireAuth && strings.TrimSpace(config.AuthToken) == "" {
		return fmt.Errorf("license service token is required when authentication is enabled")
	}
	if config.ProtocolVersion != protocol.Version {
		return fmt.Errorf("unsupported request protocol version %q", config.ProtocolVersion)
	}
	if strings.TrimSpace(config.ResponseProtocolVersion) == "" {
		return fmt.Errorf("license service response protocol version is required")
	}
	switch config.Scenario {
	case KindAllow, KindDeny, KindExpired, KindRevoked, KindQuotaExceeded, KindUnavailable, KindTimeout, KindProtocolMismatch:
	default:
		return fmt.Errorf("unsupported license service scenario %q", config.Scenario)
	}
	if config.Delay < 0 {
		return fmt.Errorf("license service delay must not be negative")
	}
	if config.FaultStatus != 0 && (config.FaultStatus < 500 || config.FaultStatus > 599) {
		return fmt.Errorf("license service fault status must be a 5xx status")
	}
	if strings.TrimSpace(config.LicenseID) == "" || strings.TrimSpace(config.ProductID) == "" {
		return fmt.Errorf("license fixture identity is required")
	}
	if config.Capacity.MaxConcurrent < 0 || config.Capacity.MaxActivations < 0 {
		return fmt.Errorf("license fixture capacity must not be negative")
	}
	if config.Clock == nil {
		return fmt.Errorf("license service clock is required")
	}
	return nil
}

// LoadFromEnv applies supported environment overrides to the default config.
func LoadFromEnv() (Config, error) {
	config := DefaultConfig()
	config.AuthToken = strings.TrimSpace(os.Getenv(EnvToken))
	if raw := os.Getenv(EnvAddress); raw != "" {
		config.Address = strings.TrimSpace(raw)
	}
	if raw := os.Getenv(EnvRequireAuth); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("%s: %w", EnvRequireAuth, err)
		}
		config.RequireAuth = value
	}
	if raw := os.Getenv(EnvProtocolVersion); raw != "" {
		config.ProtocolVersion = strings.TrimSpace(raw)
	}
	if raw := os.Getenv(EnvResponseProtocolVersion); raw != "" {
		config.ResponseProtocolVersion = strings.TrimSpace(raw)
	}
	if raw := os.Getenv(EnvScenario); raw != "" {
		config.Scenario = Kind(strings.TrimSpace(raw))
	}
	if raw := os.Getenv(EnvDelay); raw != "" {
		duration, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("%s: %w", EnvDelay, err)
		}
		config.Delay = duration
	}
	if raw := os.Getenv(EnvExpiresAt); raw != "" {
		expiresAt, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return Config{}, fmt.Errorf("%s: %w", EnvExpiresAt, err)
		}
		config.ExpiresAt = expiresAt
	}
	if raw := os.Getenv(EnvFaultStatus); raw != "" {
		status, err := strconv.Atoi(raw)
		if err != nil {
			return Config{}, fmt.Errorf("%s: %w", EnvFaultStatus, err)
		}
		config.FaultStatus = status
	}
	if raw := os.Getenv(EnvLicenseID); raw != "" {
		config.LicenseID = strings.TrimSpace(raw)
	}
	if raw := os.Getenv(EnvProductID); raw != "" {
		config.ProductID = strings.TrimSpace(raw)
	}
	if raw := os.Getenv(EnvFeatures); raw != "" {
		config.Features = splitList(raw)
	}
	if raw := os.Getenv(EnvMaxConcurrent); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return Config{}, fmt.Errorf("%s: %w", EnvMaxConcurrent, err)
		}
		config.Capacity.MaxConcurrent = value
	}
	if raw := os.Getenv(EnvMaxActivations); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return Config{}, fmt.Errorf("%s: %w", EnvMaxActivations, err)
		}
		config.Capacity.MaxActivations = value
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

// splitList parses a comma-separated list into trimmed non-empty values.
func splitList(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			values = append(values, value)
		}
	}
	return values
}

// Grant returns the deterministic grant represented by the selected scenario.
func (config Config) Grant(now time.Time) protocol.Grant {
	if now.IsZero() {
		now = config.Clock().UTC()
	}
	expiresAt := config.ExpiresAt
	status := "valid"
	if expiresAt.IsZero() {
		expiresAt = now.Add(24 * time.Hour)
	}
	switch config.Scenario {
	case KindExpired:
		status = "expired"
		expiresAt = now.Add(-time.Hour)
	case KindRevoked:
		status = "revoked"
	}
	return protocol.Grant{LicenseID: config.LicenseID, Status: status, Product: config.ProductID, Features: append([]string(nil), config.Features...), Capacity: config.Capacity, ExpiresAt: expiresAt, ValidatedAt: now}
}
