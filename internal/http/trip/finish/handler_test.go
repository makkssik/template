package finish_test

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	trip "github.com/makkssik/template/internal/domain/trip"
	api "github.com/makkssik/template/internal/generated"
	"github.com/makkssik/template/internal/http/trip/finish"
)

func TestHandler(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		body       string
	}{
		{
			name:       "success",
			wantStatus: http.StatusOK,
			body:       "",
		},
		{
			name:       "completed",
			err:        trip.ErrTripCompleted,
			wantStatus: http.StatusConflict,
			wantCode:   "trip_completed",
			body:       "",
		},
		{
			name:       "missing",
			err:        trip.ErrTripNotFound,
			wantStatus: http.StatusNotFound,
			wantCode:   "trip_not_found",
			body:       "",
		},
		{
			name:       "failure",
			err:        errors.New("secret SQL"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "internal_error",
			body:       "",
		},
		{
			name:       "nonempty body",
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_request",
			body:       "unexpected",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			usecase := NewMocktripUsecase(ctrl)
			id := uuid.New()
			finished := time.Date(2026, 8, 24, 12, 18, 0, 0, time.UTC)
			if tc.wantStatus != http.StatusBadRequest {
				usecase.EXPECT().FinishTrip(gomock.Any(), id).Return(&trip.Trip{TripID: id, Status: trip.StatusCompleted, FinishedAt: &finished}, tc.err)
			}
			h := finish.New(usecase, slog.New(slog.NewTextHandler(io.Discard, nil)))
			r := httptest.NewRequest("POST", "/api/v1/trips/"+id.String()+"/finish", strings.NewReader(tc.body))
			w := httptest.NewRecorder()
			h.FinishTrip(w, r, id)
			if w.Code != tc.wantStatus {
				t.Fatalf("status %d: %s", w.Code, w.Body)
			}
			if strings.Contains(w.Body.String(), "secret") {
				t.Fatal("internal error leaked")
			}
			if tc.wantCode != "" {
				var problem api.Problem
				if err := json.Unmarshal(w.Body.Bytes(), &problem); err != nil {
					t.Fatal(err)
				}
				if w.Header().Get("Content-Type") != "application/problem+json" || problem.Code != tc.wantCode || int(problem.Status) != tc.wantStatus || problem.Instance == nil || *problem.Instance != r.URL.Path {
					t.Fatalf("unexpected problem: %+v, headers: %v", problem, w.Header())
				}
				return
			}
			var actual api.Trip
			if err := json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
				t.Fatal(err)
			}
			if w.Header().Get("Content-Type") != "application/json" || actual.Id != id || actual.Status != api.Completed || actual.FinishedAt == nil || !actual.FinishedAt.Equal(finished) {
				t.Fatalf("unexpected completed trip: %+v", actual)
			}
		})
	}
}
