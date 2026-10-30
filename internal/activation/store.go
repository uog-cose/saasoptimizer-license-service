package activation

import (
	"context"
	"errors"
)

var (
	// ErrNotFound identifies an activation that does not exist.
	ErrNotFound = errors.New("activation not found")
	// ErrIdempotencyConflict identifies an idempotency key reused for another request.
	ErrIdempotencyConflict = errors.New("activation idempotency conflict")
	// ErrBindingConflict identifies an active binding that cannot be reused.
	ErrBindingConflict = errors.New("active activation binding conflict")
	// ErrRevisionConflict identifies an update based on stale state.
	ErrRevisionConflict = errors.New("activation revision conflict")
)

// Store is the durable boundary for activation records and side-effect
// idempotency. Implementations must make each operation atomic.
type Store interface {
	// Create creates or replays an activation. The boolean reports whether a new
	// record was created during this call.
	Create(context.Context, CreateRequest) (Record, bool, error)
	// Get returns one activation by its stable identifier.
	Get(context.Context, string) (Record, error)
	// FindActive returns the active binding for a license, product, and identity.
	FindActive(context.Context, string, string, IdentityBinding) (Record, bool, error)
	// Deactivate releases or replays one activation operation.
	Deactivate(context.Context, DeactivateRequest) (Record, bool, error)
}
