package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/uog-cose/saasoptimizer-license-service/internal/activation"
)

// TestPostgresStorePersistsActivationAcrossPoolRestart verifies the durable
// contract against a dedicated PostgreSQL database when configured.
func TestPostgresStorePersistsActivationAcrossPoolRestart(t *testing.T) {
	dsn := os.Getenv("LICENSE_SERVICE_POSTGRES_URL")
	if dsn == "" {
		t.Skip("LICENSE_SERVICE_POSTGRES_URL is not configured")
	}
	ctx := context.Background()
	pool := openTestPool(t, ctx, dsn)
	applyTestSchema(t, ctx, pool)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS saasoptimizer_license_activation_operations")
		_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS saasoptimizer_license_activations")
		pool.Close()
	})

	store, err := New(pool)
	if err != nil {
		t.Fatal(err)
	}
	request := validCreateRequest()
	request.MaxActivations = 1
	first, created, err := store.Create(ctx, request)
	if err != nil || !created {
		t.Fatalf("record=%+v created=%t error=%v", first, created, err)
	}
	request.IdempotencyKey = "activation-operation-replay"
	replayed, created, err := store.Create(ctx, request)
	if err != nil || created || replayed.ID != first.ID {
		t.Fatalf("first=%+v replayed=%+v created=%t error=%v", first, replayed, created, err)
	}
	secondRequest := request
	secondRequest.IdempotencyKey = "activation-operation-capacity"
	secondRequest.RequestFingerprint = "request-fingerprint-capacity"
	secondRequest.Binding.MachineID = "machine-2"
	if _, _, err := store.Create(ctx, secondRequest); !errors.Is(err, activation.ErrCapacityExceeded) {
		t.Fatalf("error=%v, want ErrCapacityExceeded", err)
	}
	conflicting := request
	conflicting.IdempotencyKey = "activation-operation-conflict"
	conflicting.KeyReference = "hmac-sha256:other-key"
	conflicting.RequestFingerprint = "request-fingerprint-other"
	if _, _, err := store.Create(ctx, conflicting); !errors.Is(err, activation.ErrBindingConflict) {
		t.Fatalf("error=%v, want ErrBindingConflict", err)
	}
	deactivated, _, err := store.Deactivate(ctx, activation.DeactivateRequest{IdempotencyKey: "deactivate-1", RequestFingerprint: "deactivate-fingerprint", ActivationID: first.ID, ExpectedRevision: first.Revision, At: request.At.Add(time.Minute)})
	if err != nil || deactivated.Status != activation.StatusDeactivated {
		t.Fatalf("record=%+v error=%v", deactivated, err)
	}
	if record, created, err := store.Create(ctx, secondRequest); err != nil || !created || record.Binding.MachineID != "machine-2" {
		t.Fatalf("record=%+v created=%t error=%v", record, created, err)
	}

	pool.Close()
	restartedPool := openTestPool(t, ctx, dsn)
	t.Cleanup(restartedPool.Close)
	restartedStore, err := New(restartedPool)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := restartedStore.Get(ctx, first.ID)
	if err != nil || restored.Status != activation.StatusDeactivated || restored.Revision != deactivated.Revision {
		t.Fatalf("restored=%+v error=%v", restored, err)
	}
}

// TestPostgresStoreEnforcesActivationCapacityConcurrently verifies separate
// callers cannot reserve more than the configured active machine quota.
func TestPostgresStoreEnforcesActivationCapacityConcurrently(t *testing.T) {
	dsn := os.Getenv("LICENSE_SERVICE_POSTGRES_URL")
	if dsn == "" {
		t.Skip("LICENSE_SERVICE_POSTGRES_URL is not configured")
	}
	ctx := context.Background()
	pool := openTestPool(t, ctx, dsn)
	applyTestSchema(t, ctx, pool)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS saasoptimizer_license_activation_operations")
		_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS saasoptimizer_license_activations")
		pool.Close()
	})
	store, err := New(pool)
	if err != nil {
		t.Fatal(err)
	}
	const callers = 12
	var group sync.WaitGroup
	var mu sync.Mutex
	created := 0
	quotaDenied := 0
	errorsFound := make(chan error, callers)
	for index := 0; index < callers; index++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			request := validCreateRequest()
			request.LicenseID = "license-concurrent-capacity"
			request.MaxActivations = 1
			request.IdempotencyKey = fmt.Sprintf("activation-concurrent-%d", index)
			request.RequestFingerprint = fmt.Sprintf("request-concurrent-%d", index)
			request.Binding.MachineID = fmt.Sprintf("machine-%d", index)
			_, wasCreated, createErr := store.Create(ctx, request)
			mu.Lock()
			defer mu.Unlock()
			if errors.Is(createErr, activation.ErrCapacityExceeded) {
				quotaDenied++
				return
			}
			if createErr != nil {
				errorsFound <- createErr
				return
			}
			if wasCreated {
				created++
			}
		}(index)
	}
	group.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Fatal(err)
	}
	if created != 1 || quotaDenied != callers-1 {
		t.Fatalf("created=%d quota_denied=%d, want 1 and %d", created, quotaDenied, callers-1)
	}
}

// openTestPool opens a PostgreSQL pool for the opt-in integration contract.
func openTestPool(t *testing.T, ctx context.Context, dsn string) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatal(err)
	}
	return pool
}

// applyTestSchema installs the current activation migration for the contract test.
func applyTestSchema(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	path := filepath.Join("..", "..", "..", "migrations", "00001_activation_records.up.sql")
	schema, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(schema)); err != nil {
		t.Fatal(err)
	}
}

// validCreateRequest returns a deterministic request for the PostgreSQL contract.
func validCreateRequest() activation.CreateRequest {
	at := time.Date(2026, time.October, 30, 10, 0, 0, 0, time.UTC)
	return activation.CreateRequest{
		IdempotencyKey:     "activation-operation-1",
		RequestFingerprint: "request-fingerprint-1",
		LicenseID:          "license-1",
		ProductID:          "saasoptimizer",
		KeyReference:       "hmac-sha256:key-1",
		Binding:            activation.IdentityBinding{AccountID: "account-1", TenantID: "tenant-1", ProjectID: "project-1", UserID: "user-1", ClientID: "client-1", MachineID: "machine-1"},
		At:                 at,
	}
}
