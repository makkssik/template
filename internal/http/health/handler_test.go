package health_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	metricnoop "go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"github.com/makkssik/template/internal/config"
	transport "github.com/makkssik/template/internal/http"
	"github.com/makkssik/template/internal/http/health"
	"github.com/makkssik/template/internal/http/middleware"
)

func TestHealthWithoutDatabase(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	metrics, err := middleware.NewMetrics(metricnoop.NewMeterProvider().Meter("test"))
	if err != nil {
		t.Fatal(err)
	}
	implementation := transport.New(nil, nil, nil, health.New(logger), nil)
	server := transport.NewServer(config.Config{}, implementation, logger, tracenoop.NewTracerProvider(), metrics)
	w := httptest.NewRecorder()
	server.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "application/json" || w.Body.String() != "{\"status\":\"ok\"}\n" {
		t.Fatalf("unexpected health response: status=%d headers=%v body=%s", w.Code, w.Header(), w.Body)
	}
}
