package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type Telemetry struct {
	Tracer *sdktrace.TracerProvider
	Meter  *sdkmetric.MeterProvider
}

func New(ctx context.Context, protocol, service string, logger *slog.Logger) (*Telemetry, error) {
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		logger.WarnContext(context.WithoutCancel(ctx), "telemetry export failed", "error", err)
	}))
	res, err := resource.New(ctx,
		resource.WithAttributes(attribute.String("service.version", "dev")),
		resource.WithFromEnv(),
		resource.WithAttributes(attribute.String("service.name", service)),
	)
	if err != nil {
		return nil, fmt.Errorf("telemetry resource: %w", err)
	}
	var traces sdktrace.SpanExporter
	var metrics sdkmetric.Exporter
	switch protocol {
	case "grpc":
		traces, err = otlptracegrpc.New(ctx)
		if err == nil {
			metrics, err = otlpmetricgrpc.New(ctx)
		}
	case "http/protobuf":
		traces, err = otlptracehttp.New(ctx)
		if err == nil {
			metrics, err = otlpmetrichttp.New(ctx)
		}
	default:
		return nil, fmt.Errorf("unsupported OTLP protocol %q", protocol)
	}
	if err != nil {
		if traces != nil {
			err = errors.Join(err, traces.Shutdown(ctx))
		}
		return nil, fmt.Errorf("telemetry exporters: %w", err)
	}
	return &Telemetry{
		Tracer: sdktrace.NewTracerProvider(sdktrace.WithResource(res), sdktrace.WithBatcher(traces)),
		Meter:  sdkmetric.NewMeterProvider(sdkmetric.WithResource(res), sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metrics))),
	}, nil
}

func (t *Telemetry) Shutdown(ctx context.Context) error {
	return errors.Join(t.Tracer.Shutdown(ctx), t.Meter.Shutdown(ctx))
}

type contextHandler struct{ slog.Handler }

func (h contextHandler) Handle(ctx context.Context, r slog.Record) error {
	span := trace.SpanContextFromContext(ctx)
	r.AddAttrs(slog.String("trace_id", span.TraceID().String()), slog.String("span_id", span.SpanID().String()))
	return h.Handler.Handle(ctx, r)
}

func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextHandler{h.Handler.WithAttrs(attrs)}
}

func (h contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{h.Handler.WithGroup(name)}
}

func NewLogger(level slog.Level, service string) *slog.Logger {
	return slog.New(contextHandler{slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})}).With("service", service)
}

type TripEvents struct {
	logger    *slog.Logger
	created   metric.Int64Counter
	completed metric.Int64Counter
}

func RegisterActiveTrips(meter metric.Meter, countActive func(context.Context) (int64, error)) error {
	_, err := meter.Int64ObservableGauge("tripgo.trips.active", metric.WithInt64Callback(func(ctx context.Context, observer metric.Int64Observer) error {
		count, err := countActive(ctx)
		if err != nil {
			return err
		}
		observer.Observe(count)
		return nil
	}))
	return err
}

func NewTripEvents(logger *slog.Logger, meter metric.Meter) (*TripEvents, error) {
	created, err := meter.Int64Counter("tripgo.trips.created")
	if err != nil {
		return nil, err
	}
	completed, err := meter.Int64Counter("tripgo.trips.completed")
	if err != nil {
		return nil, err
	}
	return &TripEvents{logger: logger, created: created, completed: completed}, nil
}

func (e *TripEvents) Created(ctx context.Context, id uuid.UUID) {
	e.created.Add(ctx, 1)
	e.logger.InfoContext(ctx, "trip created", "trip_id", id)
}

func (e *TripEvents) Completed(ctx context.Context, id uuid.UUID) {
	e.completed.Add(ctx, 1)
	e.logger.InfoContext(ctx, "trip completed", "trip_id", id)
}
