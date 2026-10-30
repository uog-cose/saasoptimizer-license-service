package scenario

import (
	"testing"
	"time"
)

// TestDefaultConfigRequiresExplicitDevelopmentToken verifies the service does
// not silently start an unauthenticated fixture.
func TestDefaultConfigRequiresExplicitDevelopmentToken(t *testing.T) {
	config := DefaultConfig()
	if err := config.Validate(); err == nil {
		t.Fatal("Validate() accepted an authenticated config without a token")
	}
	config.AuthToken = "development-token"
	if err := config.Validate(); err != nil {
		t.Fatalf("Validate() error=%v", err)
	}
}

// TestScenarioGrantIsDeterministic verifies the fixture outcome does not
// depend on wall-clock time when a clock is supplied.
func TestScenarioGrantIsDeterministic(t *testing.T) {
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	config := DefaultConfig()
	config.AuthToken = "development-token"
	config.Clock = func() time.Time { return now }
	config.Scenario = KindExpired
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	first := config.Grant(now)
	second := config.Grant(now)
	if first.Status != "expired" || len(first.Entitlements) == 0 || !first.ExpiresAt.Equal(second.ExpiresAt) || !first.ValidatedAt.Equal(second.ValidatedAt) {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
}

// TestLoadFromEnvParsesEntitlements verifies capability decisions are
// configurable for allow and deny integration scenarios.
func TestLoadFromEnvParsesEntitlements(t *testing.T) {
	t.Setenv(EnvToken, "development-token")
	t.Setenv(EnvEntitlements, "optimizer.ga,optimizer.random-search=false")
	config, err := LoadFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Entitlements) != 2 || !config.Entitlements[0].Enabled || config.Entitlements[1].Enabled {
		t.Fatalf("entitlements=%#v", config.Entitlements)
	}
}

// TestLoadFromEnvParsesScenarioSettings verifies configuration can be driven
// by the deployment environment without exposing token values in errors.
func TestLoadFromEnvParsesScenarioSettings(t *testing.T) {
	t.Setenv(EnvToken, "development-token")
	t.Setenv(EnvScenario, string(KindQuotaExceeded))
	t.Setenv(EnvDelay, "25ms")
	t.Setenv(EnvMaxConcurrent, "3")
	config, err := LoadFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if config.Scenario != KindQuotaExceeded || config.Delay != 25*time.Millisecond || config.Capacity.MaxConcurrent != 3 {
		t.Fatalf("config=%+v", config)
	}
}
