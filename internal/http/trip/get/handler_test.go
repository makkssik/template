package get_test

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
	"github.com/makkssik/template/internal/http/trip/get"
)

func TestHandler(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"success", nil, 200, ""},
		{"missing", trip.ErrTripNotFound, 404, "trip_not_found"},
		{"failure", errors.New("secret SQL"), 500, "internal_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			usecase := NewMocktripUsecase(ctrl)
			id := uuid.New()
			result := trip.Trip{
				TripID: id, UserID: uuid.New(), DriverID: uuid.New(), Price: 1450,
				StartPoint: trip.Coordinates{Latitude: 59.9398, Longitude: 30.3146},
				EndPoint:   trip.Coordinates{Latitude: 59.929, Longitude: 30.3626},
				Status:     trip.StatusActive, StartedAt: time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC),
			}
			usecase.EXPECT().GetTrip(gomock.Any(), id).Return(&result, tc.err)
			h := get.New(usecase, slog.New(slog.NewTextHandler(io.Discard, nil)))
			r := httptest.NewRequest("GET", "/api/v1/trips/"+id.String(), nil)
			w := httptest.NewRecorder()
			h.GetTrip(w, r, id)
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
			want := api.Trip{
				Id: id, UserId: result.UserID, DriverId: result.DriverID, Price: 1450,
				StartPoint: api.Coordinates{Latitude: 59.9398, Longitude: 30.3146},
				EndPoint:   api.Coordinates{Latitude: 59.929, Longitude: 30.3626},
				Status:     api.Active, StartedAt: result.StartedAt,
			}
			if w.Header().Get("Content-Type") != "application/json" || actual != want {
				t.Fatalf("unexpected trip: %+v", actual)
			}
		})
	}
}
