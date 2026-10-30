// Package activation defines durable License Service activation records.
package activation

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Status identifies the lifecycle state of an activation record.
type Status string

const (
	// StatusActive identifies an activation that is currently bound.
	StatusActive Status = "active"
	// StatusDeactivated identifies an activation released by an explicit request.
	StatusDeactivated Status = "deactivated"
)

// IdentityBinding records the trusted ownership identities attached to an activation.
type IdentityBinding struct {
	// AccountID identifies the owning account when supplied by the Server.
	AccountID string
	// TenantID identifies the tenant scope when supplied by the Server.
	TenantID string
	// ProjectID identifies the project scope when supplied by the Server.
	ProjectID string
	// UserID identifies the authenticated user.
	UserID string
	// ClientID identifies the registered client instance.
	ClientID string
	// MachineID identifies the registered machine.
	MachineID string
}

// Validate checks that identity fields are trimmed and required trust anchors exist.
func (binding IdentityBinding) Validate() error {
	for name, value := range map[string]string{
		"account_id": binding.AccountID,
		"tenant_id":  binding.TenantID,
		"project_id": binding.ProjectID,
		"client_id":  binding.ClientID,
	} {
		if strings.TrimSpace(value) != value {
			return fmt.Errorf("activation binding %s must be trimmed", name)
		}
	}
	for name, value := range map[string]string{
		"user_id":    binding.UserID,
		"machine_id": binding.MachineID,
	} {
		if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value {
			return fmt.Errorf("activation binding %s must be non-empty and trimmed", name)
		}
	}
	return nil
}

// CanonicalKey returns a stable non-secret key for the identity binding.
func (binding IdentityBinding) CanonicalKey() (string, error) {
	if err := binding.Validate(); err != nil {
		return "", err
	}
	return strings.Join([]string{
		binding.AccountID,
		binding.TenantID,
		binding.ProjectID,
		binding.UserID,
		binding.ClientID,
		binding.MachineID,
	}, "\x00"), nil
}

// Fingerprint returns a stable digest of the identity binding without exposing
// the identity values in persistence keys or logs.
func (binding IdentityBinding) Fingerprint() (string, error) {
	canonical, err := binding.CanonicalKey()
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(digest[:]), nil
}

// CreateRequest contains the already-protected data needed to create an activation.
type CreateRequest struct {
	// IdempotencyKey identifies one logical activation operation.
	IdempotencyKey string
	// RequestFingerprint identifies the complete non-secret request payload.
	RequestFingerprint string
	// LicenseID identifies the provider license being activated.
	LicenseID string
	// ProductID identifies the product for which the license is activated.
	ProductID string
	// KeyReference identifies protected license-key material without containing it.
	KeyReference string
	// Binding contains the trusted ownership identity snapshot.
	Binding IdentityBinding
	// At is the operation timestamp supplied by the service clock.
	At time.Time
}

// Validate checks the activation creation contract and all lifecycle inputs.
func (request CreateRequest) Validate() error {
	if strings.TrimSpace(request.IdempotencyKey) == "" || strings.TrimSpace(request.IdempotencyKey) != request.IdempotencyKey {
		return errors.New("activation idempotency key must be non-empty and trimmed")
	}
	if strings.TrimSpace(request.RequestFingerprint) == "" || strings.TrimSpace(request.RequestFingerprint) != request.RequestFingerprint {
		return errors.New("activation request fingerprint must be non-empty and trimmed")
	}
	for name, value := range map[string]string{
		"license_id":    request.LicenseID,
		"product_id":    request.ProductID,
		"key_reference": request.KeyReference,
	} {
		if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value {
			return fmt.Errorf("activation %s must be non-empty and trimmed", name)
		}
	}
	if err := request.Binding.Validate(); err != nil {
		return err
	}
	if request.At.IsZero() {
		return errors.New("activation timestamp is required")
	}
	return nil
}

// DeactivateRequest contains the data needed to release one activation.
type DeactivateRequest struct {
	// IdempotencyKey identifies one logical deactivation operation.
	IdempotencyKey string
	// RequestFingerprint identifies the complete non-secret request payload.
	RequestFingerprint string
	// ActivationID identifies the record being released.
	ActivationID string
	// ExpectedRevision prevents an update based on stale state when non-zero.
	ExpectedRevision int64
	// At is the operation timestamp supplied by the service clock.
	At time.Time
}

// Validate checks the deactivation operation contract.
func (request DeactivateRequest) Validate() error {
	for name, value := range map[string]string{
		"idempotency_key":     request.IdempotencyKey,
		"request_fingerprint": request.RequestFingerprint,
		"activation_id":       request.ActivationID,
	} {
		if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value {
			return fmt.Errorf("deactivation %s must be non-empty and trimmed", name)
		}
	}
	if request.ExpectedRevision < 0 {
		return errors.New("deactivation expected revision must not be negative")
	}
	if request.At.IsZero() {
		return errors.New("deactivation timestamp is required")
	}
	return nil
}

// Record is the durable representation of one license activation binding.
type Record struct {
	// ID identifies the activation and remains stable across service restarts.
	ID string
	// LicenseID identifies the provider license.
	LicenseID string
	// ProductID identifies the product bound by this activation.
	ProductID string
	// KeyReference identifies protected key material, never the raw key.
	KeyReference string
	// Binding is the trusted identity snapshot attached to the activation.
	Binding IdentityBinding
	// Status is the current activation lifecycle state.
	Status Status
	// Revision increments on every durable state transition.
	Revision int64
	// RequestFingerprint identifies the original activation request.
	RequestFingerprint string
	// CreatedAt records when the activation was created.
	CreatedAt time.Time
	// UpdatedAt records the most recent state change.
	UpdatedAt time.Time
	// DeactivatedAt records when the activation was released.
	DeactivatedAt *time.Time
}

// Validate checks identity, lifecycle, revision, and timestamp invariants.
func (record Record) Validate() error {
	if strings.TrimSpace(record.ID) == "" || strings.TrimSpace(record.ID) != record.ID {
		return errors.New("activation ID must be non-empty and trimmed")
	}
	if strings.TrimSpace(record.LicenseID) == "" || strings.TrimSpace(record.LicenseID) != record.LicenseID {
		return errors.New("activation license ID must be non-empty and trimmed")
	}
	if strings.TrimSpace(record.ProductID) == "" || strings.TrimSpace(record.ProductID) != record.ProductID {
		return errors.New("activation product ID must be non-empty and trimmed")
	}
	if strings.TrimSpace(record.KeyReference) == "" || strings.TrimSpace(record.KeyReference) != record.KeyReference {
		return errors.New("activation key reference must be non-empty and trimmed")
	}
	if err := record.Binding.Validate(); err != nil {
		return err
	}
	if record.Status != StatusActive && record.Status != StatusDeactivated {
		return fmt.Errorf("unsupported activation status %q", record.Status)
	}
	if record.Revision <= 0 {
		return errors.New("activation revision must be positive")
	}
	if strings.TrimSpace(record.RequestFingerprint) == "" || strings.TrimSpace(record.RequestFingerprint) != record.RequestFingerprint {
		return errors.New("activation request fingerprint must be non-empty and trimmed")
	}
	if record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() || record.UpdatedAt.Before(record.CreatedAt) {
		return errors.New("activation timestamps are invalid")
	}
	if record.DeactivatedAt != nil {
		if record.DeactivatedAt.Before(record.CreatedAt) || record.DeactivatedAt.After(record.UpdatedAt) {
			return errors.New("activation deactivated_at is invalid")
		}
	}
	if record.Status == StatusActive && record.DeactivatedAt != nil {
		return errors.New("active activation must not have deactivated_at")
	}
	if record.Status == StatusDeactivated && record.DeactivatedAt == nil {
		return errors.New("deactivated activation requires deactivated_at")
	}
	return nil
}

// Deactivate returns a validated copy with one atomic lifecycle transition.
func (record Record) Deactivate(at time.Time) (Record, error) {
	if err := record.Validate(); err != nil {
		return Record{}, err
	}
	if record.Status == StatusDeactivated {
		return record, nil
	}
	if at.IsZero() {
		return Record{}, errors.New("deactivation timestamp is required")
	}
	if at.Before(record.CreatedAt) {
		return Record{}, errors.New("deactivation timestamp precedes activation")
	}
	record.Status = StatusDeactivated
	record.Revision++
	record.UpdatedAt = at
	record.DeactivatedAt = &at
	return record, record.Validate()
}
