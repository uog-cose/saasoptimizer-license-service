package service

import "errors"

var (
	// ErrStoreRequired identifies a service created without a fixture store.
	ErrStoreRequired = errors.New("license service store is required")
	// ErrActivationStoreRequired identifies a service created without activation persistence.
	ErrActivationStoreRequired = errors.New("license service activation store is required")
)
