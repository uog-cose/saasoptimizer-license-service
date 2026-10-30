// Package store provides isolated ephemeral state for development scenarios.
package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/uog-cose/saasoptimizer-license-service/internal/protocol"
	"github.com/uog-cose/saasoptimizer-license-service/internal/scenario"
)

var (
	// ErrIdempotencyConflict identifies one key reused with another payload.
	ErrIdempotencyConflict = errors.New("idempotency key payload conflict")
	// ErrActivationNotFound identifies an unknown activation reference.
	ErrActivationNotFound = errors.New("activation not found")
	// ErrCapacityExceeded identifies a configured activation limit.
	ErrCapacityExceeded = errors.New("activation capacity exceeded")
)

// ActivationRecord stores one ephemeral activation and its request fingerprint.
type ActivationRecord struct {
	// Fingerprint identifies the original request without storing a raw key in logs.
	Fingerprint string
	// Result is the deterministic activation response.
	Result protocol.Activation
	// Active reports whether the activation has not been deactivated.
	Active bool
}

// MemoryStore stores one isolated scenario and its in-process side effects.
type MemoryStore struct {
	mu           sync.Mutex
	config       scenario.Config
	activations  map[string]ActivationRecord
	idempotency  map[string]string
	nextSequence uint64
}

// NewMemoryStore creates an empty store for one validated scenario.
func NewMemoryStore(config scenario.Config) (*MemoryStore, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &MemoryStore{config: config, activations: make(map[string]ActivationRecord), idempotency: make(map[string]string)}, nil
}

// Config returns the immutable scenario configuration used by the store.
func (store *MemoryStore) Config() scenario.Config {
	if store == nil {
		return scenario.Config{}
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	config := store.config
	config.Features = append([]string(nil), store.config.Features...)
	config.Entitlements = append([]protocol.Entitlement(nil), store.config.Entitlements...)
	return config
}

// Grant returns the current deterministic grant from the scenario clock.
func (store *MemoryStore) Grant() protocol.Grant {
	if store == nil {
		return protocol.Grant{}
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.config.Grant(store.config.Clock().UTC())
}

// Activate creates or replays one ephemeral activation using a stable key.
func (store *MemoryStore) Activate(key string, request protocol.ActivationRequest) (protocol.Activation, error) {
	if store == nil {
		return protocol.Activation{}, errors.New("license scenario store is required")
	}
	if err := request.Validate(); err != nil {
		return protocol.Activation{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if existing, ok := store.idempotency["activate/"+key]; ok {
		fingerprint := requestFingerprint(request)
		if existing != fingerprint {
			return protocol.Activation{}, ErrIdempotencyConflict
		}
		for _, record := range store.activations {
			if record.Fingerprint == fingerprint {
				result := record.Result
				result.IsNew = false
				return result, nil
			}
		}
		return protocol.Activation{}, ErrActivationNotFound
	}
	if store.config.Capacity.MaxActivations > 0 && store.activeActivationCount() >= store.config.Capacity.MaxActivations {
		return protocol.Activation{}, ErrCapacityExceeded
	}
	store.nextSequence++
	fingerprint := requestFingerprint(request)
	result := protocol.Activation{ActivationID: fmt.Sprintf("activation-%d", store.nextSequence), Grant: pointerToGrant(store.config.Grant(store.config.Clock().UTC())), IsNew: true}
	store.idempotency["activate/"+key] = fingerprint
	store.activations[result.ActivationID] = ActivationRecord{Fingerprint: fingerprint, Result: result, Active: true}
	return result, nil
}

// Deactivate releases or replays one ephemeral activation using a stable key.
func (store *MemoryStore) Deactivate(key string, request protocol.DeactivationRequest) error {
	if store == nil {
		return errors.New("license scenario store is required")
	}
	if err := request.Validate(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	fingerprint := requestFingerprint(request)
	idempotencyKey := "deactivate/" + key
	if existing, ok := store.idempotency[idempotencyKey]; ok {
		if existing != fingerprint {
			return ErrIdempotencyConflict
		}
		return nil
	}
	if request.ActivationID != "" {
		if record, ok := store.activations[request.ActivationID]; ok {
			record.Active = false
			store.activations[request.ActivationID] = record
		}
	}
	store.idempotency[idempotencyKey] = fingerprint
	return nil
}

// ActiveActivations returns the current number of active fixture activations.
func (store *MemoryStore) ActiveActivations() int {
	if store == nil {
		return 0
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.activeActivationCount()
}

// activeActivationCount counts active records while the caller holds the lock.
func (store *MemoryStore) activeActivationCount() int {
	count := 0
	for _, record := range store.activations {
		if record.Active {
			count++
		}
	}
	return count
}

// requestFingerprint hashes a request so idempotency never stores a raw key.
func requestFingerprint(value any) string {
	payload, err := json.Marshal(value)
	if err != nil {
		return "marshal-error"
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

// pointerToGrant copies a grant into a response-owned pointer.
func pointerToGrant(grant protocol.Grant) *protocol.Grant {
	grant.Features = append([]string(nil), grant.Features...)
	grant.Entitlements = append([]protocol.Entitlement(nil), grant.Entitlements...)
	return &grant
}
