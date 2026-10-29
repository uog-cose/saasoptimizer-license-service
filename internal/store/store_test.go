package store

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/uog-cose/saasoptimizer-license-service/internal/protocol"
	"github.com/uog-cose/saasoptimizer-license-service/internal/scenario"
)

// testStore creates a valid isolated fixture store for store tests.
func testStore(t *testing.T) *MemoryStore {
	t.Helper()
	config := scenario.DefaultConfig()
	config.AuthToken = "development-token"
	config.Clock = func() time.Time { return time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC) }
	store, err := NewMemoryStore(config)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

// testActivationRequest returns a valid fixture activation request.
func testActivationRequest() protocol.ActivationRequest {
	return protocol.ActivationRequest{LicenseKey: "fixture-key", UserID: "user-1", MachineID: "machine-1", ProductID: "saasoptimizer"}
}

// TestMemoryStoreReplaysActivation verifies repeated requests reuse one result.
func TestMemoryStoreReplaysActivation(t *testing.T) {
	store := testStore(t)
	first, err := store.Activate("key-1", testActivationRequest())
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Activate("key-1", testActivationRequest())
	if err != nil || second.ActivationID != first.ActivationID || second.IsNew {
		t.Fatalf("first=%+v second=%+v error=%v", first, second, err)
	}
}

// TestMemoryStoreRejectsIdempotencyPayloadChange verifies a key cannot be
// reused for a different side effect payload.
func TestMemoryStoreRejectsIdempotencyPayloadChange(t *testing.T) {
	store := testStore(t)
	if _, err := store.Activate("key-1", testActivationRequest()); err != nil {
		t.Fatal(err)
	}
	request := testActivationRequest()
	request.MachineID = "machine-2"
	if _, err := store.Activate("key-1", request); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("error=%v, want ErrIdempotencyConflict", err)
	}
}

// TestMemoryStoreConcurrentActivationIsSingleFlight verifies concurrent
// duplicate requests do not create more than one active activation.
func TestMemoryStoreConcurrentActivationIsSingleFlight(t *testing.T) {
	store := testStore(t)
	const callers = 16
	results := make(chan protocol.Activation, callers)
	errorsFound := make(chan error, callers)
	var group sync.WaitGroup
	for index := 0; index < callers; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			result, err := store.Activate("key-1", testActivationRequest())
			if err != nil {
				errorsFound <- err
				return
			}
			results <- result
		}()
	}
	group.Wait()
	close(results)
	close(errorsFound)
	for err := range errorsFound {
		t.Fatal(err)
	}
	var activationID string
	for result := range results {
		if activationID == "" {
			activationID = result.ActivationID
		}
		if result.ActivationID != activationID {
			t.Fatalf("result=%+v, want activation %s", result, activationID)
		}
	}
	if store.ActiveActivations() != 1 {
		t.Fatalf("active activations=%d, want 1", store.ActiveActivations())
	}
}

// TestMemoryStoreDeactivateIsIdempotent verifies repeated cleanup succeeds.
func TestMemoryStoreDeactivateIsIdempotent(t *testing.T) {
	store := testStore(t)
	activation, err := store.Activate("key-1", testActivationRequest())
	if err != nil {
		t.Fatal(err)
	}
	request := protocol.DeactivationRequest{LicenseID: "dev-license-001", ActivationID: activation.ActivationID, MachineID: "machine-1"}
	if err := store.Deactivate("close-1", request); err != nil {
		t.Fatal(err)
	}
	if err := store.Deactivate("close-1", request); err != nil {
		t.Fatal(err)
	}
	if store.ActiveActivations() != 0 {
		t.Fatalf("active activations=%d, want 0", store.ActiveActivations())
	}
}
