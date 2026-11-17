package container

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jonboulle/clockwork"

	"github.com/makkssik/template/internal/config"
	"github.com/makkssik/template/internal/gateway/push"
	createhandler "github.com/makkssik/template/internal/http/position/create"
	listhandler "github.com/makkssik/template/internal/http/position/list"
	"github.com/makkssik/template/internal/infra/postgres"
	"github.com/makkssik/template/internal/infra/telemetry"
	"github.com/makkssik/template/internal/repository/position"
	"github.com/makkssik/template/internal/repository/trip"
	"github.com/makkssik/template/internal/usecase/position/create"
	"github.com/makkssik/template/internal/usecase/position/list"
	"github.com/makkssik/template/internal/worker/positions"
)

type positionComponents struct {
	worker *positions.Worker
	push   *push.Gateway
	create *createhandler.Handler
	list   *listhandler.Handler
}

func buildPositions(cfg config.Config, pool *pgxpool.Pool, signals *telemetry.Telemetry, logger *slog.Logger) (positionComponents, error) {
	meter := signals.Meter.Meter("trip-service/positions")
	metrics, err := positions.NewMetrics(meter)
	if err != nil {
		return positionComponents{}, err
	}
	events, err := telemetry.NewPositionEvents(meter, logger)
	if err != nil {
		return positionComponents{}, err
	}
	gateway, err := push.New(push.Config{
		URL:             cfg.PushHTTPURL,
		RequestTimeout:  cfg.PushRequestTimeout,
		KeepAlive:       cfg.PushKeepAlive,
		IdleConnTimeout: cfg.PushIdleConnTimeout,
	}, signals.Tracer)
	if err != nil {
		return positionComponents{}, err
	}
	trips := trip.NewPostgresqlRepository(pool, cfg.DatabaseQueryTimeout)
	repo := position.NewPostgresqlRepository(pool, cfg.DatabaseQueryTimeout)
	worker, err := positions.New(positions.Config{Interval: cfg.PositionCheckInterval, StaleAfter: cfg.PositionStaleAfter, Timeout: cfg.PushRequestTimeout, Workers: cfg.PositionWorkers, QueueSize: cfg.PositionQueueSize, BatchSize: cfg.PositionBatchSize}, trips, gateway, clockwork.NewRealClock(), logger, signals.Tracer.Tracer("trip-service/positions"), metrics)
	if err != nil {
		gateway.Close()
		return positionComponents{}, err
	}
	createPosition := create.New(trips, repo, postgres.NewTxManager(pool, cfg.DatabaseQueryTimeout), worker, events)
	return positionComponents{worker: worker, push: gateway, create: createhandler.New(createPosition, logger), list: listhandler.New(list.New(trips, repo), logger)}, nil
}
