package http_test

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	metricnoop "go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"github.com/makkssik/template/internal/config"
	transport "github.com/makkssik/template/internal/http"
	"github.com/makkssik/template/internal/http/middleware"
)

func TestMalformedTripID(t *testing.T) {
	metrics, err := middleware.NewMetrics(metricnoop.NewMeterProvider().Meter("test"))
	if err != nil {
		t.Fatal(err)
	}
	server := transport.NewServer(config.Config{}, &transport.Implementation{}, slog.New(slog.NewTextHandler(io.Discard, nil)), tracenoop.NewTracerProvider(), metrics)
	for _, path := range []string{"/api/v1/trips/not-a-uuid", "/api/v1/trips/not-a-uuid/finish"} {
		method := "GET"
		if path[len(path)-6:] == "finish" {
			method = "POST"
		}
		w := httptest.NewRecorder()
		server.Handler.ServeHTTP(w, httptest.NewRequest(method, path, nil))
		if w.Code != 400 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body)
		}
	}
}
