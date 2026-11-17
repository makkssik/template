package http_test

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
	"github.com/makkssik/template/internal/http/middleware"
)

func TestMalformedTripID(t *testing.T) {
	metrics, err := middleware.NewMetrics(metricnoop.NewMeterProvider().Meter("test"))
	if err != nil {
		t.Fatal(err)
	}
	server := transport.NewServer(config.Config{}, &transport.Implementation{}, slog.New(slog.NewTextHandler(io.Discard, nil)), tracenoop.NewTracerProvider(), metrics)
	const invalidTripPath = "/api/v1/trips/not-a-uuid"
	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "get trip", method: http.MethodGet, path: invalidTripPath},
		{name: "finish trip", method: http.MethodPost, path: invalidTripPath + "/finish"},
		{name: "list positions", method: http.MethodGet, path: invalidTripPath + "/positions"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			server.Handler.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			if w.Code != http.StatusBadRequest {
				t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body)
			}
		})
	}
}
