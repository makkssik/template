package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	metricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/protobuf/proto"
)

func TestLogTraceContext(t *testing.T) {
	span := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2},
	})
	ctx := trace.ContextWithSpanContext(t.Context(), span)
	var output bytes.Buffer
	logger := slog.New(contextHandler{slog.NewJSONHandler(&output, nil)}).With("service", "trip-service")
	logger.InfoContext(ctx, "trip created", "trip_id", "example")
	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		"service": "trip-service", "msg": "trip created", "level": "INFO",
		"trace_id": span.TraceID().String(), "span_id": span.SpanID().String(), "trip_id": "example",
	} {
		if record[key] != want {
			t.Errorf("%s = %v, want %s", key, record[key], want)
		}
	}
	if _, err := time.Parse(time.RFC3339Nano, record["time"].(string)); err != nil {
		t.Fatalf("invalid log timestamp: %v", err)
	}
}

func TestOTLPExportOnShutdown(t *testing.T) {
	var mu sync.Mutex
	var traces tracepb.ExportTraceServiceRequest
	var metrics metricspb.ExportMetricsServiceRequest
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/v1/traces":
			err = proto.Unmarshal(body, &traces)
		case "/v1/metrics":
			err = proto.Unmarshal(body, &metrics)
		default:
			t.Errorf("unexpected OTLP path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/x-protobuf")
	}))
	defer collector.Close()
	for _, key := range []string{
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "OTEL_EXPORTER_OTLP_METRICS_ENDPOINT",
		"OTEL_EXPORTER_OTLP_HEADERS", "OTEL_EXPORTER_OTLP_TRACES_HEADERS", "OTEL_EXPORTER_OTLP_METRICS_HEADERS",
		"OTEL_EXPORTER_OTLP_COMPRESSION", "OTEL_EXPORTER_OTLP_TRACES_COMPRESSION", "OTEL_EXPORTER_OTLP_METRICS_COMPRESSION",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", collector.URL)
	t.Setenv("OTEL_TRACES_SAMPLER", "always_on")
	t.Setenv("OTEL_METRIC_EXPORT_INTERVAL", "60000")
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "service.namespace=tripgo,deployment.environment=local,service.version=test")
	previousHandler := otel.GetErrorHandler()
	t.Cleanup(func() { otel.SetErrorHandler(previousHandler) })
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	signals, err := New(t.Context(), "http/protobuf", "trip-service", logger)
	if err != nil {
		t.Fatal(err)
	}
	stopped := false
	t.Cleanup(func() {
		if stopped {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := signals.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	ctx, span := signals.Tracer.Tracer("test").Start(t.Context(), "POST /api/v1/trips")
	spanContext := span.SpanContext()
	events, err := NewTripEvents(logger, signals.Meter.Meter("test"))
	if err != nil {
		t.Fatal(err)
	}
	events.Created(ctx, uuid.New())
	span.End()
	shutdownCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	stopped = true
	if err := signals.Shutdown(shutdownCtx); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(traces.ResourceSpans) != 1 || len(traces.ResourceSpans[0].ScopeSpans) != 1 || len(traces.ResourceSpans[0].ScopeSpans[0].Spans) != 1 {
		t.Fatalf("expected one exported span: %v", &traces)
	}
	exported := traces.ResourceSpans[0].ScopeSpans[0].Spans[0]
	traceID, spanID := spanContext.TraceID(), spanContext.SpanID()
	if exported.Name != "POST /api/v1/trips" || !bytes.Equal(exported.TraceId, traceID[:]) || !bytes.Equal(exported.SpanId, spanID[:]) {
		t.Fatalf("unexpected span: %v", exported)
	}
	if len(metrics.ResourceMetrics) != 1 || len(metrics.ResourceMetrics[0].ScopeMetrics) != 1 {
		t.Fatalf("expected exported metrics: %v", &metrics)
	}
	for _, resource := range []*resourcepb.Resource{
		traces.ResourceSpans[0].Resource,
		metrics.ResourceMetrics[0].Resource,
	} {
		attrs := make(map[string]string)
		for _, attr := range resource.GetAttributes() {
			attrs[attr.Key] = attr.Value.GetStringValue()
		}
		for key, want := range map[string]string{
			"service.name": "trip-service", "service.namespace": "tripgo",
			"service.version": "test", "deployment.environment": "local",
		} {
			if attrs[key] != want {
				t.Errorf("resource %s = %q, want %q", key, attrs[key], want)
			}
		}
	}
	for _, metric := range metrics.ResourceMetrics[0].ScopeMetrics[0].Metrics {
		if metric.Name == "tripgo.trips.created" {
			sum := metric.GetSum()
			if sum == nil || !sum.IsMonotonic || len(sum.DataPoints) != 1 || sum.DataPoints[0].GetAsInt() != 1 || len(sum.DataPoints[0].Attributes) != 0 {
				t.Fatalf("unexpected created counter: %v", metric)
			}
			return
		}
	}
	t.Fatal("tripgo.trips.created was not exported")
}
