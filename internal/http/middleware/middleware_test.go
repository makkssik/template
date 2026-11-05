package middleware_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/makkssik/template/internal/http/middleware"
)

const (
	tripRoute   = "/api/v1/trips/{tripId}"
	tripPath    = "/api/v1/trips/123"
	traceID     = "11111111111111111111111111111111"
	traceParent = "00-11111111111111111111111111111111-2222222222222222-01"
)

func TestPanicRecorded(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	}()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	defer func() {
		if err := mp.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	}()
	metrics, err := middleware.NewMetrics(mp.Meter("test"))
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	logHandler := &contextLogHandler{Handler: slog.NewJSONHandler(&logs, nil), spans: make(map[string]trace.SpanContext)}
	handler := chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }), slog.New(logHandler), tp, metrics, func(*http.Request) string { return tripRoute })
	r := httptest.NewRequest("GET", tripPath, nil)
	r.Header.Set("traceparent", traceParent)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 500 || !strings.Contains(w.Body.String(), "internal_error") {
		t.Fatalf("response: %d %s", w.Code, w.Body)
	}
	spans := exporter.GetSpans()
	if len(spans) != 1 || spans[0].Name != "GET "+tripRoute || spans[0].Status.Code != codes.Error || spans[0].SpanContext.TraceID().String() != traceID {
		t.Fatalf("spans: %+v", spans)
	}
	if strings.Count(logs.String(), "request handled") != 1 || !strings.Contains(logs.String(), `"status":500`) {
		t.Fatalf("logs: %s", logs.String())
	}
	for _, message := range []string{"request handled", "request panic"} {
		if got := logHandler.spans[message]; !got.Equal(spans[0].SpanContext) {
			t.Fatalf("%s log lost the server span context: got %v, want %v", message, got, spans[0].SpanContext)
		}
	}
	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name == "http.server.active_requests" {
				sum := m.Data.(metricdata.Sum[int64])
				if sum.DataPoints[0].Value != 0 {
					t.Fatal("active requests leaked")
				}
				found = true
			}
		}
	}
	if !found {
		t.Fatal("active metric missing")
	}
}

type contextLogHandler struct {
	slog.Handler
	spans map[string]trace.SpanContext
}

func (h *contextLogHandler) Handle(ctx context.Context, record slog.Record) error {
	h.spans[record.Message] = trace.SpanContextFromContext(ctx)
	return h.Handler.Handle(ctx, record)
}

type testTelemetry struct {
	exporter *tracetest.InMemoryExporter
	tracer   *sdktrace.TracerProvider
	reader   *sdkmetric.ManualReader
	meter    *sdkmetric.MeterProvider
}

func newTestTelemetry(t *testing.T) testTelemetry {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	tracer := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	reader := sdkmetric.NewManualReader()
	meter := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() {
		if err := tracer.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
		if err := meter.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return testTelemetry{exporter: exporter, tracer: tracer, reader: reader, meter: meter}
}

func collectMetrics(t *testing.T, reader *sdkmetric.ManualReader) map[string]metricdata.Metrics {
	t.Helper()
	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatal(err)
	}
	metrics := make(map[string]metricdata.Metrics)
	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if _, exists := metrics[metric.Name]; exists {
				t.Fatalf("duplicate metric %q", metric.Name)
			}
			metrics[metric.Name] = metric
		}
	}
	return metrics
}

func assertActiveRequests(t *testing.T, metrics map[string]metricdata.Metrics, method, route string, want int64) {
	t.Helper()
	sum, ok := metrics["http.server.active_requests"].Data.(metricdata.Sum[int64])
	if !ok || len(sum.DataPoints) != 1 || sum.IsMonotonic {
		t.Fatalf("wrong active requests metric: %+v", metrics["http.server.active_requests"])
	}
	attrs := attribute.NewSet(attribute.String("http.request.method", method), attribute.String("http.route", route))
	if sum.DataPoints[0].Value != want || !sum.DataPoints[0].Attributes.Equals(&attrs) {
		t.Fatalf("active requests: got %+v, want value %d with %v", sum.DataPoints[0], want, attrs)
	}
}

func assertRequestDuration(t *testing.T, metrics map[string]metricdata.Metrics, method, route string, status int) {
	t.Helper()
	metric := metrics["http.server.request.duration"]
	histogram, ok := metric.Data.(metricdata.Histogram[float64])
	if !ok || len(histogram.DataPoints) != 1 || metric.Unit != "s" {
		t.Fatalf("wrong duration metric: %+v", metric)
	}
	point := histogram.DataPoints[0]
	attrs := attribute.NewSet(attribute.String("http.request.method", method), attribute.String("http.route", route), attribute.Int("http.response.status_code", status))
	if point.Count != 1 || point.Sum < 0 || !point.Attributes.Equals(&attrs) {
		t.Fatalf("duration: got %+v, want one observation with %v", point, attrs)
	}
}

func assertAccessLog(t *testing.T, logs *bytes.Buffer, method, route string, status int) {
	t.Helper()
	var record struct {
		Level      string `json:"level"`
		Message    string `json:"msg"`
		Method     string `json:"method"`
		Route      string `json:"route"`
		Status     int    `json:"status"`
		DurationMS int64  `json:"duration_ms"`
	}
	decoder := json.NewDecoder(logs)
	if err := decoder.Decode(&record); err != nil {
		t.Fatal(err)
	}
	wantLevel := "INFO"
	if status >= http.StatusInternalServerError {
		wantLevel = "ERROR"
	}
	if record.Message != "request handled" || record.Level != wantLevel || record.Method != method || record.Route != route || record.Status != status || record.DurationMS < 0 {
		t.Fatalf("wrong access log: %+v", record)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("expected exactly one access log: extra=%v err=%v", extra, err)
	}
}

func assertHTTPSpan(t *testing.T, exporter *tracetest.InMemoryExporter, method, route string, status int) {
	t.Helper()
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected one HTTP span, got %+v", spans)
	}
	span := spans[0]
	if span.Name != method+" "+route || span.SpanKind != trace.SpanKindServer || span.SpanContext.TraceID().String() != traceID || span.Parent.SpanID().String() != "2222222222222222" || !span.Parent.IsRemote() {
		t.Fatalf("wrong HTTP span or traceparent: %+v", span)
	}
	attrs := attribute.NewSet(span.Attributes...)
	for _, want := range []attribute.KeyValue{
		attribute.String("http.request.method", method),
		attribute.String("http.route", route),
		attribute.Int("http.response.status_code", status),
	} {
		if got, ok := attrs.Value(want.Key); !ok || got != want.Value {
			t.Fatalf("span attribute %s: got %v, want %v", want.Key, got, want.Value)
		}
	}
	wantStatus := codes.Unset
	if status >= http.StatusInternalServerError {
		wantStatus = codes.Error
	}
	if span.Status.Code != wantStatus {
		t.Fatalf("span status: got %v, want %v", span.Status.Code, wantStatus)
	}
}

func TestResponsesPreservedAndRecorded(t *testing.T) {
	for _, tc := range []struct {
		status int
		method string
		route  string
		path   string
		body   string
	}{
		{http.StatusOK, http.MethodGet, tripRoute, tripPath, `{"status":"active"}`},
		{http.StatusCreated, http.MethodPost, "/api/v1/trips", "/api/v1/trips", `{"status":"active"}`},
		{http.StatusBadRequest, http.MethodPost, "/api/v1/trips", "/api/v1/trips", `{"code":"invalid_request"}`},
		{http.StatusNotFound, http.MethodGet, tripRoute, tripPath, `{"code":"trip_not_found"}`},
		{http.StatusConflict, http.MethodPost, tripRoute + "/finish", tripPath + "/finish", `{"code":"trip_completed"}`},
		{http.StatusInternalServerError, http.MethodGet, tripRoute, tripPath, `{"code":"internal_error"}`},
	} {
		t.Run(strconv.Itoa(tc.status), func(t *testing.T) {
			signals := newTestTelemetry(t)
			metrics, err := middleware.NewMetrics(signals.meter.Meter("test"))
			if err != nil {
				t.Fatal(err)
			}
			contentType := "application/json"
			if tc.status >= http.StatusBadRequest {
				contentType = "application/problem+json"
			}
			wantHeaders := http.Header{"Content-Type": {contentType}}
			if tc.status == http.StatusCreated {
				wantHeaders.Set("Location", tripPath)
			}
			endpoint := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				for name, values := range wantHeaders {
					w.Header()[name] = append([]string(nil), values...)
				}
				if tc.status != http.StatusOK {
					w.WriteHeader(tc.status)
				}
				if _, err := io.WriteString(w, tc.body); err != nil {
					t.Error(err)
				}
			})
			var logs bytes.Buffer
			handler := chain(endpoint, slog.New(slog.NewJSONHandler(&logs, nil)), signals.tracer, metrics, func(*http.Request) string { return tc.route })
			r := httptest.NewRequest(tc.method, tc.path, nil)
			r.Header.Set("traceparent", traceParent)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.status || w.Body.String() != tc.body || !reflect.DeepEqual(w.Result().Header, wantHeaders) {
				t.Fatalf("response changed: status=%d body=%q headers=%v", w.Code, w.Body.String(), w.Result().Header)
			}
			assertAccessLog(t, &logs, tc.method, tc.route, tc.status)
			assertHTTPSpan(t, signals.exporter, tc.method, tc.route, tc.status)
			collected := collectMetrics(t, signals.reader)
			assertRequestDuration(t, collected, tc.method, tc.route, tc.status)
			assertActiveRequests(t, collected, tc.method, tc.route, 0)
		})
	}
}

func TestActiveRequestsWhileHandlerRuns(t *testing.T) {
	signals := newTestTelemetry(t)
	metrics, err := middleware.NewMetrics(signals.meter.Meter("test"))
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	endpoint := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
		w.WriteHeader(http.StatusOK)
	})
	handler := chain(endpoint, slog.New(slog.NewJSONHandler(io.Discard, nil)), signals.tracer, metrics, func(*http.Request) string { return tripRoute })
	w := httptest.NewRecorder()
	t.Cleanup(func() {
		unblock()
		<-done
	})
	go func() {
		defer close(done)
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tripPath, nil))
	}()
	<-entered
	assertActiveRequests(t, collectMetrics(t, signals.reader), http.MethodGet, tripRoute, 1)
	unblock()
	<-done
	collected := collectMetrics(t, signals.reader)
	assertActiveRequests(t, collected, http.MethodGet, tripRoute, 0)
	assertRequestDuration(t, collected, http.MethodGet, tripRoute, http.StatusOK)
}

func chain(next http.Handler, logger *slog.Logger, tp trace.TracerProvider, metrics *middleware.HTTPMetrics, route func(*http.Request) string) http.Handler {
	next = middleware.Metrics(next, metrics, route)
	next = middleware.Logging(next, logger, route)
	next = middleware.Tracing(next, tp, route)
	return middleware.Recover(next, logger)
}

func TestMiddlewareWorksIndependently(t *testing.T) {
	for _, name := range []string{"logging", "metrics", "tracing", "recover"} {
		t.Run(name, func(t *testing.T) {
			signals := newTestTelemetry(t)
			metrics, err := middleware.NewMetrics(signals.meter.Meter("test"))
			if err != nil {
				t.Fatal(err)
			}
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, nil))
			route := func(*http.Request) string { return tripRoute }
			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusCreated)
			})
			switch name {
			case "logging":
				handler = middleware.Logging(handler, logger, route)
			case "metrics":
				handler = middleware.Metrics(handler, metrics, route)
			case "tracing":
				handler = middleware.Tracing(handler, signals.tracer, route)
			case "recover":
				handler = middleware.Recover(handler, logger)
			}
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, tripPath, nil)
			r.Header.Set("traceparent", traceParent)
			handler.ServeHTTP(w, r)
			if w.Code != http.StatusCreated {
				t.Fatalf("status: got %d, want 201", w.Code)
			}
			switch name {
			case "logging":
				assertAccessLog(t, &logs, http.MethodPost, tripRoute, http.StatusCreated)
			case "metrics":
				collected := collectMetrics(t, signals.reader)
				assertRequestDuration(t, collected, http.MethodPost, tripRoute, http.StatusCreated)
				assertActiveRequests(t, collected, http.MethodPost, tripRoute, 0)
			case "tracing":
				assertHTTPSpan(t, signals.exporter, http.MethodPost, tripRoute, http.StatusCreated)
			}
		})
	}
}
