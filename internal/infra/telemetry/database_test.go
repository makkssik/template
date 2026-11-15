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
	for _, tc := range []struct {
		name, sql, operation, collection string
	}{
		{"current table", "SELECT id FROM trips WHERE id=$1", selectOperation, "trips"},
		{"new table", "SELECT trips FROM bookings WHERE id=$1", selectOperation, collection},
		{"mixed case and whitespace", "select id\nFrOm\tBookings b WHERE b.id=$1", selectOperation, collection},
		{"insert", "INSERT INTO bookings (id) VALUES ($1)", insertOperation, collection},
		{"insert without space", "INSERT INTO bookings(id) VALUES ($1)", insertOperation, collection},
		{"update", "UPDATE bookings SET status=$1 WHERE id=$2", "UPDATE", collection},
		{"delete", "DELETE FROM bookings WHERE id=$1", "DELETE", collection},
		{"locking select", "SELECT id FROM bookings WHERE id=$1 FOR UPDATE", selectOperation, collection},
		{"upsert", "INSERT INTO bookings (id) VALUES ($1) ON CONFLICT (id) DO UPDATE SET id=$1", insertOperation, collection},
		{"qualified table", "SELECT id FROM public.bookings;", selectOperation, "public.bookings"},
		{"no table", "SELECT 1", selectOperation, ""},
		{"missing table name", "SELECT id FROM", selectOperation, ""},
		{"begin", "BEGIN ISOLATION LEVEL READ COMMITTED", "BEGIN", ""},
		{"commit", "COMMIT", "COMMIT", ""},
		{"rollback", "ROLLBACK", "ROLLBACK", ""},
		{"empty", "", "", ""},
	} {
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
