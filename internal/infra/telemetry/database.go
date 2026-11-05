package telemetry

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
)

type (
	queryKey  struct{}
	queryInfo struct {
		started               time.Time
		operation, collection string
	}
)

type DatabaseTracer struct {
	tracer   *otelpgx.Tracer
	duration metric.Float64Histogram
}

func NewDatabaseTracer(tp trace.TracerProvider, mp metric.MeterProvider) (*DatabaseTracer, error) {
	duration, err := mp.Meter("trip-service/database").Float64Histogram("db.client.operation.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5))
	if err != nil {
		return nil, err
	}
	return &DatabaseTracer{tracer: otelpgx.NewTracer(otelpgx.WithTracerProvider(tp), otelpgx.WithMeterProvider(noop.NewMeterProvider()), otelpgx.WithDisableSQLStatementInAttributes(), otelpgx.WithDisableConnectionDetailsInAttributes()), duration: duration}, nil
}

func (t *DatabaseTracer) TraceQueryStart(ctx context.Context, c *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	info := queryInfo{started: time.Now()}
	fields := strings.Fields(strings.ToUpper(data.SQL))
	if len(fields) > 0 {
		info.operation = fields[0]
	}
	for _, field := range fields {
		switch field {
		case "TRIPS", "TRIP_STATUS_HISTORY":
			info.collection = strings.ToLower(field)
		}
	}
	ctx = t.tracer.TraceQueryStart(ctx, c, data)
	trace.SpanFromContext(ctx).SetAttributes(attribute.String("db.operation.name", info.operation), attribute.String("db.collection.name", info.collection))
	return context.WithValue(ctx, queryKey{}, info)
}

func (t *DatabaseTracer) TraceQueryEnd(ctx context.Context, c *pgx.Conn, data pgx.TraceQueryEndData) {
	info, _ := ctx.Value(queryKey{}).(queryInfo)
	kind := ""
	if data.Err != nil {
		kind = "database_error"
		var pgErr *pgconn.PgError
		if errors.As(data.Err, &pgErr) {
			kind = pgErr.Code
		}
		trace.SpanFromContext(ctx).SetStatus(codes.Error, kind)
		trace.SpanFromContext(ctx).SetAttributes(attribute.String("error.type", kind))
	}
	t.duration.Record(ctx, time.Since(info.started).Seconds(), metric.WithAttributes(attribute.String("db.operation.name", info.operation), attribute.String("db.collection.name", info.collection), attribute.String("error.type", kind)))
	t.tracer.TraceQueryEnd(ctx, c, data)
}
