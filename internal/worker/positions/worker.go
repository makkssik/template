package positions

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/makkssik/template/internal/domain/trip"
)

type (
	Config struct {
		Interval, StaleAfter, Timeout time.Duration
		Workers, QueueSize, BatchSize int
	}
	Worker struct {
		cfg                    Config
		repo                   repository
		push                   gateway
		clock                  clock
		logger                 *slog.Logger
		tracer                 trace.Tracer
		queue                  *queue
		metrics                *Metrics
		active                 atomic.Int64
		scanCancel, workCancel context.CancelFunc
		scanDone, done         chan struct{}
		start                  sync.Once
	}
)

func New(cfg Config, repo repository, push gateway, clock clock, logger *slog.Logger, tracer trace.Tracer, metrics *Metrics) (*Worker, error) {
	if cfg.Interval <= 0 || cfg.StaleAfter <= 0 || cfg.Timeout <= 0 || cfg.Workers < 1 || cfg.QueueSize < 1 || cfg.BatchSize < 1 {
		return nil, fmt.Errorf("position worker configuration must be positive")
	}
	w := &Worker{cfg: cfg, repo: repo, push: push, clock: clock, logger: logger, tracer: tracer, metrics: metrics, queue: newQueue(cfg.QueueSize), scanDone: make(chan struct{}), done: make(chan struct{})}
	if err := metrics.observe(w.queue.size, w.active.Load); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *Worker) Start(ctx context.Context) {
	w.start.Do(func() {
		workCtx, workCancel := context.WithCancel(ctx)
		w.workCancel = workCancel
		scanCtx, scanCancel := context.WithCancel(ctx)
		w.scanCancel = scanCancel
		ticker := w.clock.NewTicker(w.cfg.Interval)
		var workers sync.WaitGroup
		for range w.cfg.Workers {
			workers.Go(func() {
				w.work(workCtx)
			})
		}
		go func() {
			defer close(w.scanDone)
			defer w.queue.close()
			defer ticker.Stop()
			w.logger.InfoContext(scanCtx, "position scanner started")
			defer w.logger.InfoContext(context.WithoutCancel(scanCtx), "position scanner stopped")
			for {
				select {
				case <-scanCtx.Done():
					return
				case <-ticker.Chan():
					w.scan(scanCtx)
				}
			}
		}()
		go func() {
			workers.Wait()
			close(w.done)
		}()
		w.logger.InfoContext(ctx, "position workers started", "workers", w.cfg.Workers)
	})
}

func (w *Worker) Cancel(ctx context.Context, id uuid.UUID) {
	if w.queue.cancel(id) {
		w.metrics.request(ctx, "cancelled")
	}
}

func (w *Worker) Shutdown(ctx context.Context) error {
	w.scanCancel()
	select {
	case <-w.scanDone:
	case <-ctx.Done():
		w.workCancel()
		<-w.scanDone
	}
	select {
	case <-w.done:
	case <-ctx.Done():
		w.workCancel()
		<-w.done
	}
	w.workCancel()
	if n := w.queue.discard(); n > 0 {
		w.logger.WarnContext(ctx, "position tasks dropped during shutdown", "count", n)
	}
	w.logger.InfoContext(context.WithoutCancel(ctx), "position workers stopped")
	return ctx.Err()
}

func (w *Worker) scan(ctx context.Context) {
	ctx, span := w.tracer.Start(ctx, "positions.scan")
	defer span.End()
	cutoff := w.clock.Now().Add(-w.cfg.StaleAfter)
	var after *trip.StaleTrip
	var read int64
	seen := make(map[uuid.UUID]struct{})
	defer func() {
		w.metrics.rows.Record(ctx, read)
	}()
	for ctx.Err() == nil {
		batch, err := w.repo.FindStale(ctx, cutoff, after, w.cfg.BatchSize)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			span.SetStatus(codes.Error, "database")
			span.SetAttributes(attribute.String("error.type", "database"))
			w.logger.ErrorContext(ctx, "scan stale trips", "error", err)
			return
		}
		read += int64(len(batch))
		for _, candidate := range batch {
			if ctx.Err() != nil {
				return
			}
			if _, ok := seen[candidate.TripID]; ok {
				continue
			}
			seen[candidate.TripID] = struct{}{}
			result, dropped := w.queue.offer(&task{candidate: candidate, link: span.SpanContext()})
			w.metrics.check(ctx, result)
			if dropped != nil {
				if result != droppedResult {
					w.metrics.check(ctx, droppedResult)
				}
				w.logger.WarnContext(ctx, "position queue full", "trip_id", dropped.candidate.TripID)
				return
			}
		}
		if len(batch) < w.cfg.BatchSize {
			return
		}
		last := batch[len(batch)-1]
		after = &last
	}
}

func (w *Worker) work(ctx context.Context) {
	for {
		task, taskCtx := w.queue.take(ctx, w.cfg.Timeout)
		if task == nil {
			return
		}
		w.process(taskCtx, task)
		w.queue.finish(task)
	}
}

func (w *Worker) process(ctx context.Context, task *task) {
	w.active.Add(1)
	defer w.active.Add(-1)
	ctx, span := w.tracer.Start(ctx, "positions.request", trace.WithNewRoot(), trace.WithLinks(trace.Link{SpanContext: task.link}))
	defer span.End()
	valid, err := w.repo.StillStale(ctx, task.candidate)
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		span.SetStatus(codes.Error, "database")
		span.SetAttributes(attribute.String("error.type", "database"))
		w.logger.ErrorContext(ctx, "check stale trip", "trip_id", task.candidate.TripID, "error", err)
		return
	}
	if !valid {
		return
	}

	start := w.clock.Now()
	err = w.push.RequestPosition(ctx, task.candidate.DriverID, task.candidate.TripID)
	duration := w.clock.Now().Sub(start)
	result := "success"
	if err != nil {
		result = "error"
		span.SetStatus(codes.Error, "position_request")
		span.SetAttributes(attribute.String("error.type", "position_request"))
		w.logger.WarnContext(ctx, "request position failed", "trip_id", task.candidate.TripID, "error", err)
	}
	w.metrics.request(ctx, result)
	w.metrics.duration.Record(ctx, duration.Seconds(), resultOption(result))
}
