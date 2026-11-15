package finish_test

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
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
	for _, tc := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"success", nil, 200, ""},
		{"completed", trip.ErrTripCompleted, 409, "trip_completed"},
		{"missing", trip.ErrTripNotFound, 404, "trip_not_found"},
		{"failure", errors.New("secret SQL"), 500, "internal_error"},
		{"nonempty body", nil, 400, "invalid_request"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			usecase := NewMocktripUsecase(ctrl)
			id := uuid.New()
			finished := time.Date(2026, 8, 24, 12, 18, 0, 0, time.UTC)
			if tc.status != 400 {
				usecase.EXPECT().FinishTrip(gomock.Any(), id).Return(&trip.Trip{TripID: id, Status: trip.StatusCompleted, FinishedAt: &finished}, tc.err)
			}
			h := finish.New(usecase, slog.New(slog.NewTextHandler(io.Discard, nil)))
			body := ""
			if tc.name == "nonempty body" {
				body = "unexpected"
			}
			r := httptest.NewRequest("POST", "/api/v1/trips/"+id.String()+"/finish", strings.NewReader(body))
			w := httptest.NewRecorder()
			h.FinishTrip(w, r, id)
			if w.Code != tc.status {
				t.Fatalf("status %d: %s", w.Code, w.Body)
			}
			if strings.Contains(w.Body.String(), "secret") {
				t.Fatal("internal error leaked")
			}
			if tc.code != "" {
				var problem api.Problem
				if err := json.Unmarshal(w.Body.Bytes(), &problem); err != nil {
					t.Fatal(err)
				}
				if w.Header().Get("Content-Type") != "application/problem+json" || problem.Code != tc.code || int(problem.Status) != tc.status || problem.Instance == nil || *problem.Instance != r.URL.Path {
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
