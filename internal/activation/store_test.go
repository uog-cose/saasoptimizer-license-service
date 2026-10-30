package activation

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// TestMemoryStoreCreatesAndFindsActivation verifies the basic store contract.
func TestMemoryStoreCreatesAndFindsActivation(t *testing.T) {
	store := NewMemoryActivationStore()
	request := validCreateRequest()
	first, created, err := store.Create(context.Background(), request)
	if err != nil || !created {
		t.Fatalf("record=%+v created=%t error=%v", first, created, err)
	}
	found, ok, err := store.FindActive(context.Background(), request.LicenseID, request.ProductID, request.Binding)
	if err != nil || !ok || found.ID != first.ID {
		t.Fatalf("found=%+v ok=%t error=%v", found, ok, err)
	}
}

// TestMemoryStoreReplaysDuplicateBinding verifies a new idempotency key does
// not create a second active binding for the same request.
func TestMemoryStoreReplaysDuplicateBinding(t *testing.T) {
	store := NewMemoryActivationStore()
	request := validCreateRequest()
	first, created, err := store.Create(context.Background(), request)
	if err != nil || !created {
		t.Fatalf("record=%+v created=%t error=%v", first, created, err)
	}
	request.IdempotencyKey = "activation-operation-2"
	second, created, err := store.Create(context.Background(), request)
	if err != nil || created || second.ID != first.ID {
		t.Fatalf("first=%+v second=%+v created=%t error=%v", first, second, created, err)
	}
}

// TestMemoryStoreRejectsBindingConflict verifies different protected key data
// cannot silently reuse an existing identity binding.
func TestMemoryStoreRejectsBindingConflict(t *testing.T) {
	store := NewMemoryActivationStore()
	request := validCreateRequest()
	if _, _, err := store.Create(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	request.IdempotencyKey = "activation-operation-2"
	request.KeyReference = "hmac-sha256:key-2"
	request.RequestFingerprint = "request-fingerprint-2"
	if _, _, err := store.Create(context.Background(), request); !errors.Is(err, ErrBindingConflict) {
		t.Fatalf("error=%v, want ErrBindingConflict", err)
	}
}

// TestMemoryStoreRejectsStaleDeactivate verifies revision checks are atomic.
func TestMemoryStoreRejectsStaleDeactivate(t *testing.T) {
	store := NewMemoryActivationStore()
	request := validCreateRequest()
	record, _, err := store.Create(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	deactivate := DeactivateRequest{IdempotencyKey: "deactivation-operation-1", RequestFingerprint: "close-fingerprint", ActivationID: record.ID, ExpectedRevision: record.Revision + 1, At: request.At.Add(time.Minute)}
	if _, _, err := store.Deactivate(context.Background(), deactivate); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("error=%v, want ErrRevisionConflict", err)
	}
}

// TestMemoryStoreConcurrentCreateIsSingleFlight verifies concurrent duplicate
// requests cannot create more than one active record.
func TestMemoryStoreConcurrentCreateIsSingleFlight(t *testing.T) {
	store := NewMemoryActivationStore()
	request := validCreateRequest()
	const callers = 16
	results := make(chan Record, callers)
	errorsFound := make(chan error, callers)
	var group sync.WaitGroup
	for index := 0; index < callers; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			record, _, err := store.Create(context.Background(), request)
			if err != nil {
				errorsFound <- err
				return
			}
			results <- record
		}()
	}
	group.Wait()
	close(results)
	close(errorsFound)
	for err := range errorsFound {
		t.Fatal(err)
	}
	var id string
	for record := range results {
		if id == "" {
			id = record.ID
		}
		if record.ID != id {
			t.Fatalf("record=%+v, want ID %s", record, id)
		}
	}
}
