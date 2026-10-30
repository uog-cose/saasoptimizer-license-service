// Command license-service runs the non-production development License Service.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/uog-cose/saasoptimizer-license-service/internal/activation"
	activationpostgres "github.com/uog-cose/saasoptimizer-license-service/internal/activation/postgres"
	"github.com/uog-cose/saasoptimizer-license-service/internal/scenario"
	"github.com/uog-cose/saasoptimizer-license-service/internal/service"
	"github.com/uog-cose/saasoptimizer-license-service/internal/store"
	"github.com/uog-cose/saasoptimizer-license-service/migrations"
)

// main starts the configured non-production License Service and exits with a
// diagnostic when configuration or serving fails.
func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "license-service: %v\n", err)
		os.Exit(1)
	}
}

// run loads configuration, serves HTTP requests, and performs graceful
// shutdown on SIGINT or SIGTERM.
func run() error {
	config, err := scenario.LoadFromEnv()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	fixtureStore, err := store.NewMemoryStore(config)
	if err != nil {
		return fmt.Errorf("create scenario store: %w", err)
	}
	activationStore, closeStore, err := openActivationStore(config)
	if err != nil {
		return fmt.Errorf("open activation store: %w", err)
	}
	defer closeStore()
	application, err := service.NewWithActivationStore(config, fixtureStore, activationStore, nil, logger)
	if err != nil {
		return fmt.Errorf("create HTTP service: %w", err)
	}
	httpServer := &http.Server{Addr: config.Address, Handler: application.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second}
	contextValue, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- httpServer.ListenAndServe()
	}()
	logger.Info("development license service listening", "event", "service_start", "service", "saasoptimizer-license-service", "environment", "development", "address", config.Address, "scenario", string(config.Scenario), "storage_mode", string(config.StorageMode), "protocol_version", config.ResponseProtocolVersion)
	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	case <-contextValue.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownContext); err != nil {
			return fmt.Errorf("shutdown HTTP service: %w", err)
		}
		return nil
	}
}

// openActivationStore creates the configured activation backend and returns a
// cleanup function for resources owned by the command.
func openActivationStore(config scenario.Config) (activation.Store, func(), error) {
	if config.StorageMode == scenario.StorageModeMemory {
		return activation.NewMemoryActivationStore(), func() {}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, config.PostgresURL)
	if err != nil {
		return nil, func() {}, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, func() {}, fmt.Errorf("ping PostgreSQL: %w", err)
	}
	if err := migrations.Apply(ctx, pool); err != nil {
		pool.Close()
		return nil, func() {}, err
	}
	activationStore, err := activationpostgres.New(pool)
	if err != nil {
		pool.Close()
		return nil, func() {}, err
	}
	return activationStore, pool.Close, nil
}
