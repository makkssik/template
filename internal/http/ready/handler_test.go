package ready_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	metricnoop "go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/mock/gomock"

	"github.com/makkssik/template/internal/config"
	transport "github.com/makkssik/template/internal/http"
	"github.com/makkssik/template/internal/http/middleware"
	"github.com/makkssik/template/internal/http/ready"
)

func TestReady(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		body       string
	}{
		{
			name:       "available",
			wantStatus: http.StatusOK,
			body:       "{\"status\":\"ok\"}\n",
		},
		{
			name:       "unavailable",
			err:        errors.New("private database details"),
			wantStatus: http.StatusServiceUnavailable,
			body:       "{\"status\":\"unavailable\"}\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
			metrics, err := middleware.NewMetrics(metricnoop.NewMeterProvider().Meter("test"))
			if err != nil {
				t.Fatal(err)
			}
			type contextKey struct{}
			ctx := context.WithValue(t.Context(), contextKey{}, tc.name)
			checkup := NewMockcheckupService(gomock.NewController(t))
			checkup.EXPECT().Check(gomock.Any()).DoAndReturn(func(ctx context.Context) error {
				if ctx.Value(contextKey{}) != tc.name {
					t.Fatal("request context was not passed to readiness check")
				}
				return tc.err
			})
			implementation := transport.New(transport.ImplementationCfg{
				ReadyHandler: ready.New(checkup, logger),
			})
			server := transport.NewServer(config.Config{}, implementation, logger, tracenoop.NewTracerProvider(), metrics)
			w := httptest.NewRecorder()
			r := httptest.NewRequestWithContext(ctx, http.MethodGet, "/ready", nil)
			server.Handler.ServeHTTP(w, r)
			if w.Code != tc.wantStatus || w.Header().Get("Content-Type") != "application/json" || w.Body.String() != tc.body {
				t.Fatalf("unexpected readiness response: status=%d headers=%v body=%s", w.Code, w.Header(), w.Body)
			}
		})
	}
}
