// Package protocol defines the wire contract served by the development
// License Service.
package protocol

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	// Version is the first supported License Service protocol version.
	Version = "v1"
	// ProtocolVersionHeader carries the negotiated protocol version.
	ProtocolVersionHeader = "X-License-Protocol-Version"
)

var (
	// ErrInvalidRequest identifies a malformed request envelope.
	ErrInvalidRequest = errors.New("invalid license protocol request")
	// ErrInvalidResponse identifies a malformed response envelope.
	ErrInvalidResponse = errors.New("invalid license protocol response")
	// ErrProtocolIncompatible identifies an unsupported protocol version.
	ErrProtocolIncompatible = errors.New("license protocol incompatible")
)

// Operation identifies one License Service operation.
type Operation string

const (
	// OperationValidate checks the current grant for a subject.
	OperationValidate Operation = "validate"
	// OperationRefresh refreshes a grant by license identifier.
	OperationRefresh Operation = "refresh"
	// OperationActivate creates or reuses an activation.
	OperationActivate Operation = "activate"
	// OperationDeactivate releases an activation.
	OperationDeactivate Operation = "deactivate"
)

// ErrorCode identifies one stable service failure class.
type ErrorCode string

const (
	// ErrorInvalidRequest identifies malformed input.
	ErrorInvalidRequest ErrorCode = "invalid_request"
	// ErrorAuthenticationFailed identifies invalid service credentials.
	ErrorAuthenticationFailed ErrorCode = "authentication_failed"
	// ErrorProtocolIncompatible identifies unsupported protocol versions.
	ErrorProtocolIncompatible ErrorCode = "protocol_incompatible"
	// ErrorInvalidLicense identifies a denied or invalid license.
	ErrorInvalidLicense ErrorCode = "invalid_license"
	// ErrorLicenseExpired identifies an expired license.
	ErrorLicenseExpired ErrorCode = "license_expired"
	// ErrorLicenseRevoked identifies a revoked license.
	ErrorLicenseRevoked ErrorCode = "license_revoked"
	// ErrorQuotaExceeded identifies a capacity denial.
	ErrorQuotaExceeded ErrorCode = "quota_exceeded"
	// ErrorConflict identifies an idempotency conflict.
	ErrorConflict ErrorCode = "conflict"
	// ErrorUnavailable identifies a temporary service outage.
	ErrorUnavailable ErrorCode = "unavailable"
	// ErrorInternal identifies an unexpected service failure.
	ErrorInternal ErrorCode = "internal"
)

// Error is the safe typed error returned by the License Service.
type Error struct {
	// Code is the stable machine-readable failure class.
	Code ErrorCode `json:"code"`
	// Message is a bounded non-secret diagnostic message.
	Message string `json:"message,omitempty"`
	// Retryable reports whether the caller may retry.
	Retryable bool `json:"retryable"`
}

// Error returns the stable error code without exposing the remote message.
func (serviceError Error) Error() string {
	if serviceError.Code == "" {
		return "license service error"
	}
	return fmt.Sprintf("license service error: %s", serviceError.Code)
}

// Validate checks that a typed error has a bounded code and message.
func (serviceError Error) Validate() error {
	if strings.TrimSpace(string(serviceError.Code)) == "" || strings.TrimSpace(string(serviceError.Code)) != string(serviceError.Code) {
		return fmt.Errorf("%w: error code is required and trimmed", ErrInvalidResponse)
	}
	if strings.TrimSpace(serviceError.Message) != serviceError.Message || len(serviceError.Message) > 1024 {
		return fmt.Errorf("%w: error message is invalid", ErrInvalidResponse)
	}
	return nil
}

// Subject identifies the trusted scope for a license decision.
type Subject struct {
	// AccountID identifies the owning account.
	AccountID string `json:"account_id,omitempty"`
	// UserID identifies the acting user.
	UserID string `json:"user_id"`
	// TenantID identifies the tenant scope.
	TenantID string `json:"tenant_id,omitempty"`
	// ProjectID identifies the project scope.
	ProjectID string `json:"project_id,omitempty"`
	// ClientID identifies the client instance.
	ClientID string `json:"client_id,omitempty"`
	// MachineID identifies the host machine.
	MachineID string `json:"machine_id"`
	// ProductID identifies the licensed product.
	ProductID string `json:"product_id"`
}

// Validate checks the required and optional identity fields.
func (subject Subject) Validate() error {
	for name, value := range map[string]string{"user_id": subject.UserID, "machine_id": subject.MachineID, "product_id": subject.ProductID} {
		if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value {
			return fmt.Errorf("%w: subject %s is required and trimmed", ErrInvalidRequest, name)
		}
	}
	for name, value := range map[string]string{"account_id": subject.AccountID, "tenant_id": subject.TenantID, "project_id": subject.ProjectID, "client_id": subject.ClientID} {
		if strings.TrimSpace(value) != value {
			return fmt.Errorf("%w: subject %s must be trimmed", ErrInvalidRequest, name)
		}
	}
	return nil
}

// Entitlement identifies one licensed capability.
type Entitlement struct {
	// ID is the stable capability identifier.
	ID string `json:"id"`
	// Enabled reports whether the capability is granted.
	Enabled bool `json:"enabled"`
}

// Capacity contains optional provider-reported limits.
type Capacity struct {
	// MaxConcurrent limits simultaneous execution when non-zero.
	MaxConcurrent int `json:"max_concurrent,omitempty"`
	// MaxActivations limits active machine bindings when non-zero.
	MaxActivations int `json:"max_activations,omitempty"`
}

// Grant is the provider-neutral license result.
type Grant struct {
	// LicenseID identifies the license.
	LicenseID string `json:"license_id"`
	// Status is the current status string.
	Status string `json:"status"`
	// Product identifies the licensed product.
	Product string `json:"product"`
	// Features lists legacy feature names.
	Features []string `json:"features,omitempty"`
	// Entitlements lists capability decisions.
	Entitlements []Entitlement `json:"entitlements,omitempty"`
	// Capacity contains provider-reported limits.
	Capacity Capacity `json:"capacity,omitempty"`
	// ExpiresAt identifies the grant expiry time.
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	// ValidatedAt identifies when the grant was evaluated.
	ValidatedAt time.Time `json:"validated_at,omitempty"`
}

// Validate checks the stable identity and capacity fields of a grant.
func (grant Grant) Validate() error {
	if strings.TrimSpace(grant.LicenseID) == "" || strings.TrimSpace(grant.Product) == "" || strings.TrimSpace(grant.Status) == "" {
		return fmt.Errorf("%w: grant identity is incomplete", ErrInvalidResponse)
	}
	if grant.Capacity.MaxConcurrent < 0 || grant.Capacity.MaxActivations < 0 {
		return fmt.Errorf("%w: grant capacity must not be negative", ErrInvalidResponse)
	}
	return nil
}

// ActivationRequest carries a license activation request.
type ActivationRequest struct {
	// AccountID identifies the activating account.
	AccountID string `json:"account_id,omitempty"`
	// LicenseKey is the opaque development fixture key.
	LicenseKey string `json:"license_key"`
	// MachineID identifies the target machine.
	MachineID string `json:"machine_id"`
	// UserID identifies the activating user.
	UserID string `json:"user_id"`
	// TenantID identifies the tenant binding.
	TenantID string `json:"tenant_id,omitempty"`
	// ProjectID identifies the project binding.
	ProjectID string `json:"project_id,omitempty"`
	// ClientID identifies the client binding.
	ClientID string `json:"client_id,omitempty"`
	// ProductID identifies the product.
	ProductID string `json:"product_id"`
}

// Validate checks an activation request without including its key in errors.
func (request ActivationRequest) Validate() error {
	if err := (Subject{AccountID: request.AccountID, UserID: request.UserID, TenantID: request.TenantID, ProjectID: request.ProjectID, ClientID: request.ClientID, MachineID: request.MachineID, ProductID: request.ProductID}).Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(request.LicenseKey) == "" || strings.TrimSpace(request.LicenseKey) != request.LicenseKey {
		return fmt.Errorf("%w: license key is required and trimmed", ErrInvalidRequest)
	}
	return nil
}

// Activation is the result of an activation request.
type Activation struct {
	// ActivationID identifies the ephemeral activation.
	ActivationID string `json:"activation_id,omitempty"`
	// Grant is the resulting license grant.
	Grant *Grant `json:"grant,omitempty"`
	// IsNew reports whether a new activation was created.
	IsNew bool `json:"is_new"`
}

// DeactivationRequest carries a request to release an activation.
type DeactivationRequest struct {
	// AccountID identifies the owning account.
	AccountID string `json:"account_id,omitempty"`
	// LicenseID identifies the license.
	LicenseID string `json:"license_id"`
	// ActivationID identifies the activation when available.
	ActivationID string `json:"activation_id,omitempty"`
	// TenantID identifies the tenant binding.
	TenantID string `json:"tenant_id,omitempty"`
	// ProjectID identifies the project binding.
	ProjectID string `json:"project_id,omitempty"`
	// ClientID identifies the client binding.
	ClientID string `json:"client_id,omitempty"`
	// UserID identifies the releasing user.
	UserID string `json:"user_id,omitempty"`
	// MachineID identifies the releasing machine.
	MachineID string `json:"machine_id"`
}

// Validate checks deactivation references without exposing secret material.
func (request DeactivationRequest) Validate() error {
	for name, value := range map[string]string{"license_id": request.LicenseID, "machine_id": request.MachineID} {
		if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value {
			return fmt.Errorf("%w: deactivation %s is required and trimmed", ErrInvalidRequest, name)
		}
	}
	for name, value := range map[string]string{"account_id": request.AccountID, "activation_id": request.ActivationID, "tenant_id": request.TenantID, "project_id": request.ProjectID, "client_id": request.ClientID, "user_id": request.UserID} {
		if strings.TrimSpace(value) != value {
			return fmt.Errorf("%w: deactivation %s must be trimmed", ErrInvalidRequest, name)
		}
	}
	return nil
}

// Request is the versioned request envelope.
type Request struct {
	// ProtocolVersion identifies the requested contract version.
	ProtocolVersion string `json:"protocol_version"`
	// RequestID correlates one transport request.
	RequestID string `json:"request_id"`
	// OperationID correlates one logical operation.
	OperationID string `json:"operation_id"`
	// IdempotencyKey protects side-effecting operations from duplication.
	IdempotencyKey string `json:"idempotency_key,omitempty"`
	// Operation selects the requested operation.
	Operation Operation `json:"operation"`
	// Subject carries validation identity.
	Subject *Subject `json:"subject,omitempty"`
	// LicenseID identifies a refresh target.
	LicenseID string `json:"license_id,omitempty"`
	// Activation carries activation input.
	Activation *ActivationRequest `json:"activation,omitempty"`
	// Deactivation carries deactivation input.
	Deactivation *DeactivationRequest `json:"deactivation,omitempty"`
}

// Validate checks envelope correlation and operation-specific fields.
func (request Request) Validate() error {
	if request.ProtocolVersion != Version {
		return fmt.Errorf("%w: want %s", ErrProtocolIncompatible, Version)
	}
	for name, value := range map[string]string{"request_id": request.RequestID, "operation_id": request.OperationID} {
		if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value {
			return fmt.Errorf("%w: %s is required and trimmed", ErrInvalidRequest, name)
		}
	}
	if request.Operation != OperationValidate && request.Operation != OperationRefresh && request.Operation != OperationActivate && request.Operation != OperationDeactivate {
		return fmt.Errorf("%w: unsupported operation", ErrInvalidRequest)
	}
	if request.Operation == OperationActivate || request.Operation == OperationDeactivate {
		if strings.TrimSpace(request.IdempotencyKey) == "" || strings.TrimSpace(request.IdempotencyKey) != request.IdempotencyKey {
			return fmt.Errorf("%w: idempotency key is required and trimmed", ErrInvalidRequest)
		}
	}
	switch request.Operation {
	case OperationValidate:
		if request.Subject == nil {
			return fmt.Errorf("%w: subject is required", ErrInvalidRequest)
		}
		return request.Subject.Validate()
	case OperationRefresh:
		if strings.TrimSpace(request.LicenseID) == "" || strings.TrimSpace(request.LicenseID) != request.LicenseID {
			return fmt.Errorf("%w: license ID is required and trimmed", ErrInvalidRequest)
		}
	case OperationActivate:
		if request.Activation == nil {
			return fmt.Errorf("%w: activation is required", ErrInvalidRequest)
		}
		return request.Activation.Validate()
	case OperationDeactivate:
		if request.Deactivation == nil {
			return fmt.Errorf("%w: deactivation is required", ErrInvalidRequest)
		}
		return request.Deactivation.Validate()
	}
	return nil
}

// Response is the versioned response envelope.
type Response struct {
	// ProtocolVersion identifies the response contract version.
	ProtocolVersion string `json:"protocol_version"`
	// RequestID echoes the request correlation identifier.
	RequestID string `json:"request_id"`
	// OperationID echoes the operation correlation identifier.
	OperationID string `json:"operation_id"`
	// Grant carries a successful validation or refresh result.
	Grant *Grant `json:"grant,omitempty"`
	// Activation carries a successful activation result.
	Activation *Activation `json:"activation,omitempty"`
	// Accepted reports successful deactivation.
	Accepted bool `json:"accepted,omitempty"`
	// Error carries a typed failure.
	Error *Error `json:"error,omitempty"`
}

// Validate checks response correlation and success/error exclusivity.
func (response Response) Validate() error {
	if response.ProtocolVersion != Version {
		return fmt.Errorf("%w: want %s", ErrProtocolIncompatible, Version)
	}
	if strings.TrimSpace(response.RequestID) == "" || strings.TrimSpace(response.OperationID) == "" {
		return fmt.Errorf("%w: response correlation is incomplete", ErrInvalidResponse)
	}
	if response.Error != nil {
		if response.Grant != nil || response.Activation != nil || response.Accepted {
			return fmt.Errorf("%w: error response contains success payload", ErrInvalidResponse)
		}
		return response.Error.Validate()
	}
	if response.Grant == nil && response.Activation == nil && !response.Accepted {
		return fmt.Errorf("%w: success payload is missing", ErrInvalidResponse)
	}
	if response.Grant != nil {
		if err := response.Grant.Validate(); err != nil {
			return err
		}
	}
	if response.Activation != nil && response.Activation.Grant != nil {
		if err := response.Activation.Grant.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// HealthResponse reports service readiness without exposing credentials.
type HealthResponse struct {
	// Status reports the service state.
	Status string `json:"status"`
	// Service identifies the service name.
	Service string `json:"service"`
	// Environment identifies the non-production runtime boundary.
	Environment string `json:"environment"`
	// ProtocolVersion identifies the supported contract.
	ProtocolVersion string `json:"protocol_version"`
	// CheckType identifies whether the response is liveness or readiness.
	CheckType string `json:"check_type,omitempty"`
	// Dependencies reports bounded dependency state without secret details.
	Dependencies []HealthDependency `json:"dependencies,omitempty"`
}

// HealthDependency describes one required or optional service dependency.
type HealthDependency struct {
	// Name identifies the dependency using a stable low-cardinality name.
	Name string `json:"name"`
	// Status reports the dependency state.
	Status string `json:"status"`
	// Required reports whether failure blocks readiness.
	Required bool `json:"required"`
	// Detail is a bounded, non-secret diagnostic.
	Detail string `json:"detail,omitempty"`
}
