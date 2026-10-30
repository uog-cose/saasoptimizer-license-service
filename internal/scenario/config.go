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
	// EnvEntitlements configures comma-separated capability decisions. A bare
	// identifier enables it; identifier=false explicitly disables it.
	EnvEntitlements = "LICENSE_SERVICE_ENTITLEMENTS"
	// EnvMaxConcurrent configures the fixture concurrent capacity.
	EnvMaxConcurrent = "LICENSE_SERVICE_MAX_CONCURRENT"
	// EnvMaxActivations configures the fixture activation capacity.
	EnvMaxActivations = "LICENSE_SERVICE_MAX_ACTIVATIONS"
	// EnvStorageMode selects memory or PostgreSQL activation persistence.
	EnvStorageMode = "LICENSE_SERVICE_STORAGE_MODE"
	// EnvPostgresURL configures the PostgreSQL activation store connection.
	EnvPostgresURL = "LICENSE_SERVICE_POSTGRES_URL"
	// EnvKeyProtectionSecret configures the HMAC key used for license references.
	EnvKeyProtectionSecret = "LICENSE_SERVICE_KEY_PROTECTION_SECRET"
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

// StorageMode identifies the activation persistence backend.
type StorageMode string

const (
	// StorageModeMemory keeps activation state in the process for fixtures.
	StorageModeMemory StorageMode = "memory"
	// StorageModePostgres persists activation state in PostgreSQL.
	StorageModePostgres StorageMode = "postgres"
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
	// Entitlements contains the fixture capability decisions.
	Entitlements []protocol.Entitlement
	// Capacity contains the fixture execution limits.
	Capacity protocol.Capacity
	// StorageMode selects the activation persistence backend.
	StorageMode StorageMode
	// PostgresURL configures the durable activation database.
	PostgresURL string
	// KeyProtectionSecret protects license-key references in durable storage.
	KeyProtectionSecret string
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
		Entitlements:            defaultEntitlements(),
		Capacity:                protocol.Capacity{MaxConcurrent: 2, MaxActivations: 4},
		StorageMode:             StorageModeMemory,
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
	if _, err := normalizeEntitlements(config.Entitlements); err != nil {
		return fmt.Errorf("license fixture entitlements: %w", err)
	}
	switch config.StorageMode {
	case StorageModeMemory:
	case StorageModePostgres:
		if strings.TrimSpace(config.PostgresURL) == "" {
			return fmt.Errorf("license service PostgreSQL URL is required")
		}
		if strings.TrimSpace(config.KeyProtectionSecret) != config.KeyProtectionSecret || len(config.KeyProtectionSecret) < 16 {
			return fmt.Errorf("license key protection secret must contain at least 16 characters")
		}
	default:
		return fmt.Errorf("unsupported license service storage mode %q", config.StorageMode)
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
	if raw := os.Getenv(EnvEntitlements); raw != "" {
		entitlements, err := parseEntitlements(raw)
		if err != nil {
			return Config{}, fmt.Errorf("%s: %w", EnvEntitlements, err)
		}
		config.Entitlements = entitlements
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
	if raw := os.Getenv(EnvStorageMode); raw != "" {
		config.StorageMode = StorageMode(strings.TrimSpace(raw))
	}
	config.PostgresURL = strings.TrimSpace(os.Getenv(EnvPostgresURL))
	config.KeyProtectionSecret = os.Getenv(EnvKeyProtectionSecret)
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
	entitlements, _ := normalizeEntitlements(config.Entitlements)
	return protocol.Grant{LicenseID: config.LicenseID, Status: status, Product: config.ProductID, Features: append([]string(nil), config.Features...), Entitlements: entitlements, Capacity: config.Capacity, ExpiresAt: expiresAt, ValidatedAt: now}
}

// defaultEntitlements returns the capabilities granted by the development
// fixture so local discovery and task creation exercise the real policy path.
func defaultEntitlements() []protocol.Entitlement {
	return []protocol.Entitlement{
		{ID: "multi-objective", Enabled: true},
		{ID: "optimization", Enabled: true},
		{ID: "optimizer.essabpro", Enabled: true},
		{ID: "optimizer.ga", Enabled: true},
		{ID: "optimizer.random-search", Enabled: true},
	}
}

// parseEntitlements parses comma-separated capability decisions from an
// environment variable without accepting ambiguous duplicate values.
func parseEntitlements(raw string) ([]protocol.Entitlement, error) {
	values := make([]protocol.Entitlement, 0)
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		id := item
		enabled := true
		if strings.Contains(item, "=") {
			parts := strings.SplitN(item, "=", 2)
			id = strings.TrimSpace(parts[0])
			parsed, err := strconv.ParseBool(strings.TrimSpace(parts[1]))
			if err != nil {
				return nil, fmt.Errorf("entitlement %q has invalid enabled value", id)
			}
			enabled = parsed
		}
		if id == "" || id != strings.ToLower(id) || strings.TrimSpace(id) != id {
			return nil, fmt.Errorf("entitlement ID %q must be lowercase and trimmed", id)
		}
		values = append(values, protocol.Entitlement{ID: id, Enabled: enabled})
	}
	return normalizeEntitlements(values)
}

// normalizeEntitlements returns sorted unique capability decisions and rejects
// conflicting duplicates before a grant is exposed on the wire.
func normalizeEntitlements(entitlements []protocol.Entitlement) ([]protocol.Entitlement, error) {
	if len(entitlements) == 0 {
		return nil, nil
	}
	values := make(map[string]bool, len(entitlements))
	for _, entitlement := range entitlements {
		id := strings.TrimSpace(entitlement.ID)
		if id == "" || id != strings.ToLower(id) || id != entitlement.ID {
			return nil, fmt.Errorf("entitlement ID %q must be lowercase and trimmed", entitlement.ID)
		}
		if previous, exists := values[id]; exists {
			if previous != entitlement.Enabled {
				return nil, fmt.Errorf("entitlement %q has conflicting duplicate decisions", id)
			}
			continue
		}
		values[id] = entitlement.Enabled
	}
	result := make([]protocol.Entitlement, 0, len(values))
	for id, enabled := range values {
		result = append(result, protocol.Entitlement{ID: id, Enabled: enabled})
	}
	// The small fixture list is sorted with a simple insertion pass to keep
	// output deterministic without another dependency.
	for index := 1; index < len(result); index++ {
		for cursor := index; cursor > 0 && result[cursor].ID < result[cursor-1].ID; cursor-- {
			result[cursor], result[cursor-1] = result[cursor-1], result[cursor]
		}
	}
	return result, nil
}
