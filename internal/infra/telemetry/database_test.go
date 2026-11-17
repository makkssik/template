package telemetry

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestDatabaseQueryAttributes(t *testing.T) {
	const (
		selectOperation = "SELECT"
		insertOperation = "INSERT"
		collection      = "bookings"
	)
	tests := []struct {
		name       string
		sql        string
		operation  string
		collection string
	}{
		{
			name:       "current table",
			sql:        "SELECT id FROM trips WHERE id=$1",
			operation:  selectOperation,
			collection: "trips",
		},
		{
			name:       "new table",
			sql:        "SELECT trips FROM bookings WHERE id=$1",
			operation:  selectOperation,
			collection: collection,
		},
		{
			name:       "mixed case and whitespace",
			sql:        "select id\nFrOm\tBookings b WHERE b.id=$1",
			operation:  selectOperation,
			collection: collection,
		},
		{
			name:       "insert",
			sql:        "INSERT INTO bookings (id) VALUES ($1)",
			operation:  insertOperation,
			collection: collection,
		},
		{
			name:       "insert without space",
			sql:        "INSERT INTO bookings(id) VALUES ($1)",
			operation:  insertOperation,
			collection: collection,
		},
		{
			name:       "update",
			sql:        "UPDATE bookings SET status=$1 WHERE id=$2",
			operation:  "UPDATE",
			collection: collection,
		},
		{
			name:       "delete",
			sql:        "DELETE FROM bookings WHERE id=$1",
			operation:  "DELETE",
			collection: collection,
		},
		{
			name:       "locking select",
			sql:        "SELECT id FROM bookings WHERE id=$1 FOR UPDATE",
			operation:  selectOperation,
			collection: collection,
		},
		{
			name:       "upsert",
			sql:        "INSERT INTO bookings (id) VALUES ($1) ON CONFLICT (id) DO UPDATE SET id=$1",
			operation:  insertOperation,
			collection: collection,
		},
		{
			name:       "qualified table",
			sql:        "SELECT id FROM public.bookings;",
			operation:  selectOperation,
			collection: "public.bookings",
		},
		{
			name:       "no table",
			sql:        "SELECT 1",
			operation:  selectOperation,
			collection: "",
		},
		{
			name:       "missing table name",
			sql:        "SELECT id FROM",
			operation:  selectOperation,
			collection: "",
		},
		{
			name:       "begin",
			sql:        "BEGIN ISOLATION LEVEL READ COMMITTED",
			operation:  "BEGIN",
			collection: "",
		},
		{
			name:       "commit",
			sql:        "COMMIT",
			operation:  "COMMIT",
			collection: "",
		},
		{
			name:       "rollback",
			sql:        "ROLLBACK",
			operation:  "ROLLBACK",
			collection: "",
		},
		{
			name:       "empty",
			sql:        "",
			operation:  "",
			collection: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recorder := tracetest.NewSpanRecorder()
			tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			reader := sdkmetric.NewManualReader()
			mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
			t.Cleanup(func() {
				if err := tp.Shutdown(context.Background()); err != nil {
					t.Error(err)
				}
				if err := mp.Shutdown(context.Background()); err != nil {
					t.Error(err)
				}
			})
			tracer, err := NewDatabaseTracer(tp, mp)
			if err != nil {
				t.Fatal(err)
			}
			ctx, parent := tp.Tracer("test").Start(t.Context(), "request")
			defer parent.End()
			ctx = tracer.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{
				SQL: tc.sql, Args: []any{"FROM another_table"},
			})
			tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{})
			spans := recorder.Ended()
			if len(spans) != 1 {
				t.Fatalf("expected one database span, got %d", len(spans))
			}
			assertDatabaseAttributes(t, attribute.NewSet(spans[0].Attributes()...), tc.operation, tc.collection)
			var data metricdata.ResourceMetrics
			if err := reader.Collect(t.Context(), &data); err != nil {
				t.Fatal(err)
			}
			if len(data.ScopeMetrics) != 1 || len(data.ScopeMetrics[0].Metrics) != 1 {
				t.Fatalf("expected one database metric: %+v", data)
			}
			metric := data.ScopeMetrics[0].Metrics[0]
			histogram, ok := metric.Data.(metricdata.Histogram[float64])
			if metric.Name != "db.client.operation.duration" || !ok || len(histogram.DataPoints) != 1 || histogram.DataPoints[0].Count != 1 {
				t.Fatalf("unexpected database duration metric: %+v", metric)
			}
			assertDatabaseAttributes(t, histogram.DataPoints[0].Attributes, tc.operation, tc.collection)
		})
	}
}

func assertDatabaseAttributes(t *testing.T, attrs attribute.Set, operation, collection string) {
	t.Helper()
	for key, want := range map[attribute.Key]string{
		"db.operation.name": operation, "db.collection.name": collection,
	} {
		if got, ok := attrs.Value(key); !ok || got.AsString() != want {
			t.Errorf("%s: got %v, want %q", key, got, want)
		}
	}
}
