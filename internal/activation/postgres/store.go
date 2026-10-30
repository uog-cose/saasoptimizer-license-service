// Package postgres implements the durable activation Store with PostgreSQL.
package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/uog-cose/saasoptimizer-license-service/internal/activation"
)

const (
	activationTable = "saasoptimizer_license_activations"
	operationTable  = "saasoptimizer_license_activation_operations"
)

// Store implements activation.Store against PostgreSQL.
type Store struct {
	pool *pgxpool.Pool
}

// New creates a PostgreSQL activation Store using an existing connection pool.
func New(pool *pgxpool.Pool) (*Store, error) {
	if pool == nil {
		return nil, errors.New("activation PostgreSQL pool is required")
	}
	return &Store{pool: pool}, nil
}

// Create creates or replays one activation in a serializable transaction.
func (store *Store) Create(ctx context.Context, request activation.CreateRequest) (activation.Record, bool, error) {
	ctx = normalizedContext(ctx)
	if store == nil || store.pool == nil {
		return activation.Record{}, false, errors.New("activation PostgreSQL store is required")
	}
	if err := request.Validate(); err != nil {
		return activation.Record{}, false, err
	}
	bindingFingerprint, err := request.Binding.Fingerprint()
	if err != nil {
		return activation.Record{}, false, err
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return activation.Record{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	operation, found, err := findOperation(ctx, tx, "activate", request.IdempotencyKey)
	if err != nil {
		return activation.Record{}, false, err
	}
	if found {
		if operation.Fingerprint != request.RequestFingerprint {
			return activation.Record{}, false, activation.ErrIdempotencyConflict
		}
		record, err := getByID(ctx, tx, operation.ActivationID)
		if err != nil {
			return activation.Record{}, false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return activation.Record{}, false, err
		}
		return record, false, nil
	}
	existing, found, err := findActive(ctx, tx, request.LicenseID, request.ProductID, bindingFingerprint)
	if err != nil {
		return activation.Record{}, false, err
	}
	if found {
		if existing.KeyReference != request.KeyReference || existing.RequestFingerprint != request.RequestFingerprint {
			return activation.Record{}, false, activation.ErrBindingConflict
		}
		if err := insertOperation(ctx, tx, "activate", request.IdempotencyKey, request.RequestFingerprint, existing.ID); err != nil {
			return activation.Record{}, false, mapDatabaseError(err)
		}
		if err := tx.Commit(ctx); err != nil {
			return activation.Record{}, false, err
		}
		return existing, false, nil
	}
	id, err := newActivationID()
	if err != nil {
		return activation.Record{}, false, err
	}
	record := activation.Record{
		ID:                 id,
		LicenseID:          request.LicenseID,
		ProductID:          request.ProductID,
		KeyReference:       request.KeyReference,
		Binding:            request.Binding,
		Status:             activation.StatusActive,
		Revision:           1,
		RequestFingerprint: request.RequestFingerprint,
		CreatedAt:          request.At.UTC(),
		UpdatedAt:          request.At.UTC(),
	}
	if err := record.Validate(); err != nil {
		return activation.Record{}, false, err
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO `+activationTable+` (
    id, license_id, product_id, key_reference,
    account_id, tenant_id, project_id, user_id, client_id, machine_id,
    binding_fingerprint, status, revision, request_fingerprint,
    created_at, updated_at, deactivated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
`, record.ID, record.LicenseID, record.ProductID, record.KeyReference,
		record.Binding.AccountID, record.Binding.TenantID, record.Binding.ProjectID, record.Binding.UserID, record.Binding.ClientID, record.Binding.MachineID,
		bindingFingerprint, record.Status, record.Revision, record.RequestFingerprint, record.CreatedAt, record.UpdatedAt, record.DeactivatedAt); err != nil {
		return activation.Record{}, false, mapDatabaseError(err)
	}
	if err := insertOperation(ctx, tx, "activate", request.IdempotencyKey, request.RequestFingerprint, record.ID); err != nil {
		return activation.Record{}, false, mapDatabaseError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return activation.Record{}, false, mapDatabaseError(err)
	}
	return record, true, nil
}

// Get returns one activation by stable identifier.
func (store *Store) Get(ctx context.Context, id string) (activation.Record, error) {
	ctx = normalizedContext(ctx)
	if store == nil || store.pool == nil {
		return activation.Record{}, errors.New("activation PostgreSQL store is required")
	}
	if id == "" {
		return activation.Record{}, activation.ErrNotFound
	}
	return getByID(ctx, store.pool, id)
}

// FindActive returns the active record for one license and identity binding.
func (store *Store) FindActive(ctx context.Context, licenseID, productID string, binding activation.IdentityBinding) (activation.Record, bool, error) {
	ctx = normalizedContext(ctx)
	if store == nil || store.pool == nil {
		return activation.Record{}, false, errors.New("activation PostgreSQL store is required")
	}
	fingerprint, err := binding.Fingerprint()
	if err != nil {
		return activation.Record{}, false, err
	}
	record, found, err := findActive(ctx, store.pool, licenseID, productID, fingerprint)
	return record, found, err
}

// Deactivate releases or replays one activation operation transactionally.
func (store *Store) Deactivate(ctx context.Context, request activation.DeactivateRequest) (activation.Record, bool, error) {
	ctx = normalizedContext(ctx)
	if store == nil || store.pool == nil {
		return activation.Record{}, false, errors.New("activation PostgreSQL store is required")
	}
	if err := request.Validate(); err != nil {
		return activation.Record{}, false, err
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return activation.Record{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	operation, found, err := findOperation(ctx, tx, "deactivate", request.IdempotencyKey)
	if err != nil {
		return activation.Record{}, false, err
	}
	if found {
		if operation.Fingerprint != request.RequestFingerprint {
			return activation.Record{}, false, activation.ErrIdempotencyConflict
		}
		record, err := getByID(ctx, tx, operation.ActivationID)
		if err != nil {
			return activation.Record{}, false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return activation.Record{}, false, err
		}
		return record, false, nil
	}
	record, err := getByIDForUpdate(ctx, tx, request.ActivationID)
	if err != nil {
		return activation.Record{}, false, err
	}
	if request.ExpectedRevision > 0 && request.ExpectedRevision != record.Revision {
		return activation.Record{}, false, activation.ErrRevisionConflict
	}
	if record.Status == activation.StatusDeactivated {
		if err := insertOperation(ctx, tx, "deactivate", request.IdempotencyKey, request.RequestFingerprint, record.ID); err != nil {
			return activation.Record{}, false, mapDatabaseError(err)
		}
		if err := tx.Commit(ctx); err != nil {
			return activation.Record{}, false, err
		}
		return record, false, nil
	}
	updated, err := record.Deactivate(request.At.UTC())
	if err != nil {
		return activation.Record{}, false, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE `+activationTable+`
SET status = $1, revision = $2, updated_at = $3, deactivated_at = $4
WHERE id = $5 AND revision = $6
`, updated.Status, updated.Revision, updated.UpdatedAt, updated.DeactivatedAt, updated.ID, record.Revision); err != nil {
		return activation.Record{}, false, mapDatabaseError(err)
	}
	if err := insertOperation(ctx, tx, "deactivate", request.IdempotencyKey, request.RequestFingerprint, updated.ID); err != nil {
		return activation.Record{}, false, mapDatabaseError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return activation.Record{}, false, mapDatabaseError(err)
	}
	return updated, true, nil
}

// findOperation loads one idempotency result from a transaction or pool.
func findOperation(ctx context.Context, query rowQueryer, operationType, key string) (operation, bool, error) {
	var result operation
	err := query.QueryRow(ctx, `
SELECT request_fingerprint, activation_id
FROM `+operationTable+`
WHERE operation_type = $1 AND idempotency_key = $2
`, operationType, key).Scan(&result.Fingerprint, &result.ActivationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return operation{}, false, nil
	}
	if err != nil {
		return operation{}, false, err
	}
	return result, true, nil
}

// insertOperation records one idempotent operation result.
func insertOperation(ctx context.Context, tx pgx.Tx, operationType, key, fingerprint, activationID string) error {
	_, err := tx.Exec(ctx, `
INSERT INTO `+operationTable+` (operation_type, idempotency_key, request_fingerprint, activation_id, created_at)
VALUES ($1, $2, $3, $4, $5)
`, operationType, key, fingerprint, activationID, time.Now().UTC())
	return err
}

// findActive loads one active binding using the partial unique index.
func findActive(ctx context.Context, query rowQueryer, licenseID, productID, bindingFingerprint string) (activation.Record, bool, error) {
	record, err := scanRecord(query.QueryRow(ctx, `
SELECT id, license_id, product_id, key_reference,
       account_id, tenant_id, project_id, user_id, client_id, machine_id,
       status, revision, request_fingerprint, created_at, updated_at, deactivated_at
FROM `+activationTable+`
WHERE license_id = $1 AND product_id = $2 AND binding_fingerprint = $3 AND status = 'active'
`, licenseID, productID, bindingFingerprint))
	if errors.Is(err, activation.ErrNotFound) {
		return activation.Record{}, false, nil
	}
	if err != nil {
		return activation.Record{}, false, err
	}
	return record, true, nil
}

// getByID loads one activation by ID from a query source.
func getByID(ctx context.Context, query rowQueryer, id string) (activation.Record, error) {
	return scanRecord(query.QueryRow(ctx, `
SELECT id, license_id, product_id, key_reference,
       account_id, tenant_id, project_id, user_id, client_id, machine_id,
       status, revision, request_fingerprint, created_at, updated_at, deactivated_at
FROM `+activationTable+`
WHERE id = $1
`, id))
}

// getByIDForUpdate loads an activation while holding a row lock.
func getByIDForUpdate(ctx context.Context, query rowQueryer, id string) (activation.Record, error) {
	return scanRecord(query.QueryRow(ctx, `
SELECT id, license_id, product_id, key_reference,
       account_id, tenant_id, project_id, user_id, client_id, machine_id,
       status, revision, request_fingerprint, created_at, updated_at, deactivated_at
FROM `+activationTable+`
WHERE id = $1
FOR UPDATE
`, id))
}

// scanRecord converts one PostgreSQL row into a validated activation record.
func scanRecord(row rowScanner) (activation.Record, error) {
	var record activation.Record
	var status string
	var deactivatedAt *time.Time
	err := row.Scan(
		&record.ID, &record.LicenseID, &record.ProductID, &record.KeyReference,
		&record.Binding.AccountID, &record.Binding.TenantID, &record.Binding.ProjectID, &record.Binding.UserID, &record.Binding.ClientID, &record.Binding.MachineID,
		&status, &record.Revision, &record.RequestFingerprint, &record.CreatedAt, &record.UpdatedAt, &deactivatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return activation.Record{}, activation.ErrNotFound
		}
		return activation.Record{}, err
	}
	record.Status = activation.Status(status)
	record.DeactivatedAt = deactivatedAt
	if err := record.Validate(); err != nil {
		return activation.Record{}, fmt.Errorf("validate persisted activation: %w", err)
	}
	return record, nil
}

// newActivationID returns an opaque ID that does not encode license secrets.
func newActivationID() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("generate activation ID: %w", err)
	}
	return "activation-" + hex.EncodeToString(random[:]), nil
}

// mapDatabaseError preserves stable domain conflict errors for callers.
func mapDatabaseError(err error) error {
	if err == nil {
		return nil
	}
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) {
		switch pgError.Code {
		case "23505":
			return activation.ErrBindingConflict
		case "40001":
			return activation.ErrRevisionConflict
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return activation.ErrNotFound
	}
	return err
}

// normalizedContext supplies a usable context to the pgx API.
func normalizedContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// rowScanner is the common Scan contract shared by pgx rows.
type rowScanner interface {
	Scan(...any) error
}

// rowQueryer is the common QueryRow contract shared by pools and transactions.
type rowQueryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// operation is the persisted idempotency result.
type operation struct {
	Fingerprint  string
	ActivationID string
}

var _ activation.Store = (*Store)(nil)
