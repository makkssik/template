package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/makkssik/template/internal/config"
	"github.com/makkssik/template/internal/container"
)

func main() {
	ctx := context.Background()
	if err := run(ctx); err != nil {
		container.LogError(ctx, err)
		os.Exit(1)
	}
}

func run(parent context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()

	application, err := container.New(ctx, cfg)
	if err != nil {
		return fmt.Errorf("build application: %w", err)
	}
	pool, server := application.Pool, application.Server
	serverErrors := make(chan error, 1)

	go func() {
		serverErrors <- server.ListenAndServe()
	}()
	application.Logger.InfoContext(ctx, "service started", "http_addr", cfg.HTTPAddr)

	var serverErr error
	select {
	case <-ctx.Done():
		application.Logger.InfoContext(ctx, "service stopping")
	case serverErr = <-serverErrors:
	}
	stop()

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(parent), cfg.ShutdownTimeout)
	defer cancel()

	shutdownErr := server.Shutdown(shutdownCtx)
	if shutdownErr != nil {
		shutdownErr = errors.Join(shutdownErr, server.Close())
	}

	if err := application.Telemetry.Shutdown(shutdownCtx); err != nil {
		application.Logger.WarnContext(shutdownCtx, "shutdown telemetry", "error", err)
	}
	poolClosed := make(chan struct{})
	go func() {
		pool.Close()
		close(poolClosed)
	}()

	select {
	case <-poolClosed:
	case <-shutdownCtx.Done():
		return fmt.Errorf("shutdown timeout, forcing exit: %w", errors.Join(shutdownCtx.Err(), shutdownErr))
	}
	if shutdownErr != nil {
		return fmt.Errorf("shutdown HTTP server: %w", shutdownErr)
	}

	if serverErr != nil && !errors.Is(serverErr, http.ErrServerClosed) {
		return fmt.Errorf("serve HTTP: %w", serverErr)
	}
	application.Logger.InfoContext(shutdownCtx, "service stopped")
	return nil
}
