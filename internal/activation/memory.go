package activation

import (
	"context"
	"fmt"
	"sync"
)

// MemoryStore is a concurrency-safe activation Store for unit tests and
// explicitly non-durable development scenarios.
type MemoryStore struct {
	mu             sync.Mutex
	nextSequence   uint64
	records        map[string]Record
	activeBindings map[string]string
	operations     map[string]memoryOperation
}

// memoryOperation records the result of one idempotent side-effect operation.
type memoryOperation struct {
	fingerprint string
	activation  string
}

// NewMemoryActivationStore creates an empty activation Store.
func NewMemoryActivationStore() *MemoryStore {
	return &MemoryStore{
		records:        make(map[string]Record),
		activeBindings: make(map[string]string),
		operations:     make(map[string]memoryOperation),
	}
}

// Create creates or replays one activation atomically.
func (store *MemoryStore) Create(ctx context.Context, request CreateRequest) (Record, bool, error) {
	if err := contextError(ctx); err != nil {
		return Record{}, false, err
	}
	if store == nil {
		return Record{}, false, fmt.Errorf("activation store is required")
	}
	if err := request.Validate(); err != nil {
		return Record{}, false, err
	}
	bindingKey, err := activationBindingKey(request.LicenseID, request.ProductID, request.Binding)
	if err != nil {
		return Record{}, false, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	operationKey := "activate/" + request.IdempotencyKey
	if existing, ok := store.operations[operationKey]; ok {
		if existing.fingerprint != request.RequestFingerprint {
			return Record{}, false, ErrIdempotencyConflict
		}
		record, ok := store.records[existing.activation]
		if !ok {
			return Record{}, false, ErrNotFound
		}
		return cloneRecord(record), false, nil
	}
	if existingID, ok := store.activeBindings[bindingKey]; ok {
		record, found := store.records[existingID]
		if !found {
			return Record{}, false, ErrNotFound
		}
		if record.KeyReference != request.KeyReference || record.RequestFingerprint != request.RequestFingerprint {
			return Record{}, false, ErrBindingConflict
		}
		store.operations[operationKey] = memoryOperation{fingerprint: request.RequestFingerprint, activation: record.ID}
		return cloneRecord(record), false, nil
	}
	store.nextSequence++
	record := Record{
		ID:                 fmt.Sprintf("activation-%d", store.nextSequence),
		LicenseID:          request.LicenseID,
		ProductID:          request.ProductID,
		KeyReference:       request.KeyReference,
		Binding:            request.Binding,
		Status:             StatusActive,
		Revision:           1,
		RequestFingerprint: request.RequestFingerprint,
		CreatedAt:          request.At,
		UpdatedAt:          request.At,
	}
	if err := record.Validate(); err != nil {
		return Record{}, false, err
	}
	store.records[record.ID] = record
	store.activeBindings[bindingKey] = record.ID
	store.operations[operationKey] = memoryOperation{fingerprint: request.RequestFingerprint, activation: record.ID}
	return cloneRecord(record), true, nil
}

// Get returns one activation by stable identifier.
func (store *MemoryStore) Get(ctx context.Context, id string) (Record, error) {
	if err := contextError(ctx); err != nil {
		return Record{}, err
	}
	if store == nil {
		return Record{}, fmt.Errorf("activation store is required")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	record, ok := store.records[id]
	if !ok {
		return Record{}, ErrNotFound
	}
	return cloneRecord(record), nil
}

// FindActive returns the active record for one license and identity binding.
func (store *MemoryStore) FindActive(ctx context.Context, licenseID, productID string, binding IdentityBinding) (Record, bool, error) {
	if err := contextError(ctx); err != nil {
		return Record{}, false, err
	}
	if store == nil {
		return Record{}, false, fmt.Errorf("activation store is required")
	}
	bindingKey, err := activationBindingKey(licenseID, productID, binding)
	if err != nil {
		return Record{}, false, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	id, ok := store.activeBindings[bindingKey]
	if !ok {
		return Record{}, false, nil
	}
	record, ok := store.records[id]
	if !ok {
		return Record{}, false, ErrNotFound
	}
	return cloneRecord(record), true, nil
}

// Deactivate releases or replays one activation atomically.
func (store *MemoryStore) Deactivate(ctx context.Context, request DeactivateRequest) (Record, bool, error) {
	if err := contextError(ctx); err != nil {
		return Record{}, false, err
	}
	if store == nil {
		return Record{}, false, fmt.Errorf("activation store is required")
	}
	if err := request.Validate(); err != nil {
		return Record{}, false, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	operationKey := "deactivate/" + request.IdempotencyKey
	if existing, ok := store.operations[operationKey]; ok {
		if existing.fingerprint != request.RequestFingerprint {
			return Record{}, false, ErrIdempotencyConflict
		}
		record, ok := store.records[existing.activation]
		if !ok {
			return Record{}, false, ErrNotFound
		}
		return cloneRecord(record), false, nil
	}
	record, ok := store.records[request.ActivationID]
	if !ok {
		return Record{}, false, ErrNotFound
	}
	if request.ExpectedRevision > 0 && request.ExpectedRevision != record.Revision {
		return Record{}, false, ErrRevisionConflict
	}
	updated, err := record.Deactivate(request.At)
	if err != nil {
		return Record{}, false, err
	}
	store.records[updated.ID] = updated
	bindingKey, err := activationBindingKey(updated.LicenseID, updated.ProductID, updated.Binding)
	if err != nil {
		return Record{}, false, err
	}
	delete(store.activeBindings, bindingKey)
	store.operations[operationKey] = memoryOperation{fingerprint: request.RequestFingerprint, activation: updated.ID}
	return cloneRecord(updated), true, nil
}

// activationBindingKey creates a stable lookup key without storing raw key material.
func activationBindingKey(licenseID, productID string, binding IdentityBinding) (string, error) {
	if licenseID == "" || productID == "" {
		return "", fmt.Errorf("license and product IDs are required")
	}
	fingerprint, err := binding.Fingerprint()
	if err != nil {
		return "", err
	}
	return licenseID + "\x00" + productID + "\x00" + fingerprint, nil
}

// cloneRecord prevents callers from mutating the store's timestamp pointer.
func cloneRecord(record Record) Record {
	if record.DeactivatedAt != nil {
		value := *record.DeactivatedAt
		record.DeactivatedAt = &value
	}
	return record
}

// contextError returns a canceled context error without entering the store.
func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

var _ Store = (*MemoryStore)(nil)
