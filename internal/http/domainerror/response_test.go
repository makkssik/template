package domainerror_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	trip "github.com/makkssik/template/internal/domain/trip"
	api "github.com/makkssik/template/internal/generated"
	"github.com/makkssik/template/internal/http/domainerror"
)

func TestProblemResponseSpanStatus(t *testing.T) {
	const invalidRequestCode = "invalid_request"
	for _, tc := range []struct {
		name       string
		err        error
		status     int
		code       string
		spanStatus codes.Code
	}{
		{"validation", &trip.ValidationError{Field: "price", Detail: "negative"}, http.StatusBadRequest, invalidRequestCode, codes.Unset},
		{"not found", trip.ErrTripNotFound, http.StatusNotFound, "trip_not_found", codes.Unset},
		{"completed", trip.ErrTripCompleted, http.StatusConflict, "trip_completed", codes.Unset},
		{"busy", trip.ErrDriverBusy, http.StatusConflict, "driver_busy", codes.Unset},
		{"internal", errors.New("database unavailable"), http.StatusInternalServerError, "internal_error", codes.Error},
		{"explicit client error", nil, http.StatusBadRequest, invalidRequestCode, codes.Unset},
		{"explicit server error", nil, http.StatusServiceUnavailable, "service_unavailable", codes.Error},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := tracetest.NewSpanRecorder()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			t.Cleanup(func() {
				if err := provider.Shutdown(context.Background()); err != nil {
					t.Error(err)
				}
			})
			ctx, span := provider.Tracer("test").Start(t.Context(), "request")
			request := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/trips/test", nil)
			response := httptest.NewRecorder()
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			if tc.err != nil {
				domainerror.ServiceError(logger, response, request, tc.err)
			} else {
				domainerror.WriteProblem(logger, response, request, tc.status, tc.code, http.StatusText(tc.status), "test detail")
			}
			span.End()
			var problem api.Problem
			if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
				t.Fatal(err)
			}
			if response.Code != tc.status || int(problem.Status) != tc.status || problem.Code != tc.code || response.Header().Get("Content-Type") != "application/problem+json" {
				t.Fatalf("unexpected HTTP response: status=%d problem=%+v headers=%v", response.Code, problem, response.Header())
			}
			spans := recorder.Ended()
			if len(spans) != 1 {
				t.Fatalf("expected one span, got %d", len(spans))
			}
			if got := spans[0].Status().Code; got != tc.spanStatus {
				t.Errorf("span status: got %v, want %v", got, tc.spanStatus)
			}
			attrs := attribute.NewSet(spans[0].Attributes()...)
			errorType, exists := attrs.Value("error.type")
			if tc.spanStatus == codes.Error {
				if !exists || errorType.AsString() != tc.code {
					t.Errorf("error.type: got %v, want %s", errorType, tc.code)
				}
			} else if exists {
				t.Errorf("client error must not set error.type: %v", errorType)
			}
		})
	}
}
