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
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		spanStatus codes.Code
	}{
		{
			name:       "validation",
			err:        &trip.ValidationError{Field: "price", Detail: "negative"},
			wantStatus: http.StatusBadRequest,
			wantCode:   invalidRequestCode,
			spanStatus: codes.Unset,
		},
		{
			name:       "not found",
			err:        trip.ErrTripNotFound,
			wantStatus: http.StatusNotFound,
			wantCode:   "trip_not_found",
			spanStatus: codes.Unset,
		},
		{
			name:       "completed",
			err:        trip.ErrTripCompleted,
			wantStatus: http.StatusConflict,
			wantCode:   "trip_completed",
			spanStatus: codes.Unset,
		},
		{
			name:       "busy",
			err:        trip.ErrDriverBusy,
			wantStatus: http.StatusConflict,
			wantCode:   "driver_busy",
			spanStatus: codes.Unset,
		},
		{
			name:       "internal",
			err:        errors.New("database unavailable"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "internal_error",
			spanStatus: codes.Error,
		},
		{
			name:       "explicit client error",
			wantStatus: http.StatusBadRequest,
			wantCode:   invalidRequestCode,
			spanStatus: codes.Unset,
		},
		{
			name:       "explicit server error",
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   "service_unavailable",
			spanStatus: codes.Error,
		},
	}

	for _, tc := range tests {
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
				domainerror.WriteProblem(logger, response, request, tc.wantStatus, tc.wantCode, http.StatusText(tc.wantStatus), "test detail")
			}
			span.End()
			var problem api.Problem
			if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
				t.Fatal(err)
			}
			if response.Code != tc.wantStatus || int(problem.Status) != tc.wantStatus || problem.Code != tc.wantCode || response.Header().Get("Content-Type") != "application/problem+json" {
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
				if !exists || errorType.AsString() != tc.wantCode {
					t.Errorf("error.type: got %v, want %s", errorType, tc.wantCode)
				}
			} else if exists {
				t.Errorf("client error must not set error.type: %v", errorType)
			}
		})
	}
}
