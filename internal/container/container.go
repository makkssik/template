package container

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/makkssik/template/internal/config"
	transport "github.com/makkssik/template/internal/http"
	healthhandler "github.com/makkssik/template/internal/http/health"
	"github.com/makkssik/template/internal/http/middleware"
	readyhandler "github.com/makkssik/template/internal/http/ready"
	createhandler "github.com/makkssik/template/internal/http/trip/create"
	finishhandler "github.com/makkssik/template/internal/http/trip/finish"
	gethandler "github.com/makkssik/template/internal/http/trip/get"
	"github.com/makkssik/template/internal/infra/postgres"
	"github.com/makkssik/template/internal/infra/telemetry"
	historypg "github.com/makkssik/template/internal/repository/history"
	trippg "github.com/makkssik/template/internal/repository/trip"
	"github.com/makkssik/template/internal/usecase/checkup"
	"github.com/makkssik/template/internal/usecase/trip/create"
	"github.com/makkssik/template/internal/usecase/trip/finish"
	"github.com/makkssik/template/internal/usecase/trip/get"
)

type Application struct {
	Telemetry *telemetry.Telemetry
	Logger    *slog.Logger
	Pool      *pgxpool.Pool
	Server    *http.Server
}

func New(ctx context.Context, cfg config.Config) (app *Application, err error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		return nil, err
	}
	logger := telemetry.NewLogger(level, cfg.OTELServiceName)
	signals, err := telemetry.New(ctx, cfg.OTELProtocol, cfg.OTELServiceName, logger)
	if err != nil {
		return nil, err
	}
	var pool *pgxpool.Pool
	defer func() {
		if err == nil {
			return
		}
		err = errors.Join(err, shutdownTelemetry(ctx, cfg, signals))
		if pool != nil {
			pool.Close()
		}
	}()

	tracer, err := telemetry.NewDatabaseTracer(signals.Tracer, signals.Meter)
	if err != nil {
		return nil, err
	}
	poolConfig, err := postgres.NewPoolConfig(cfg, tracer)
	if err != nil {
		return nil, err
	}
	pool, err = pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create database pool: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, cfg.DatabaseQueryTimeout)
	err = pool.Ping(pingCtx)
	cancel()
	if err != nil {
		pool.Close()
		pool = nil
		return nil, fmt.Errorf("ping database: %w", err)
	}

	trips := trippg.NewPostgresqlRepository(pool, cfg.DatabaseQueryTimeout)
	history := historypg.NewPostgresqlRepository(pool, cfg.DatabaseQueryTimeout)
	tx := postgres.NewTxManager(pool, cfg.DatabaseQueryTimeout)

	tripMeter := signals.Meter.Meter("trip-service/trips")
	events, err := telemetry.NewTripEvents(logger, tripMeter)
	if err != nil {
		return nil, err
	}
	if err := telemetry.RegisterActiveTrips(tripMeter, trips.CountActive); err != nil {
		return nil, err
	}
	metrics, err := middleware.NewMetrics(signals.Meter.Meter("trip-service/http"))
	if err != nil {
		return nil, err
	}

	createTrip := create.New(trips, history, tx, events)
	getTrip := get.New(trips)
	finishTrip := finish.New(trips, history, tx, events)
	checkupService := checkup.NewService(checkup.ServiceCfg{
		Database:     pool,
		QueryTimeout: cfg.DatabaseQueryTimeout,
	})

	createHandler := createhandler.New(createTrip, logger)
	getHandler := gethandler.New(getTrip, logger)
	finishHandler := finishhandler.New(finishTrip, logger)
	healthHandler := healthhandler.New(logger)
	readyHandler := readyhandler.New(checkupService, logger)
	implementation := transport.New(createHandler, getHandler, finishHandler, healthHandler, readyHandler)
	return &Application{
		Pool:      pool,
		Server:    transport.NewServer(cfg, implementation, logger, signals.Tracer, metrics),
		Telemetry: signals,
		Logger:    logger,
	}, nil
}

func shutdownTelemetry(ctx context.Context, cfg config.Config, signals *telemetry.Telemetry) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.ShutdownTimeout)
	defer cancel()
	return signals.Shutdown(ctx)
}
