package create_test

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
	"github.com/makkssik/template/internal/http/trip/create"
	createusecase "github.com/makkssik/template/internal/usecase/trip/create"
)

func TestHandler(t *testing.T) {
	const invalidRequestCode = "invalid_request"
	const tripFields = `{
  "user_id": "5cb72c04-7650-45c9-a79b-bcdba0631e0c",
  "driver_id": "8860b315-ec86-42eb-a17c-7c163d721ff5",
  "start_point": {"latitude": 59.9398, "longitude": 30.3146},
  "end_point": {"latitude": 59.929, "longitude": 30.3626}`
	const body = tripFields + `,"price":1450}`
	tests := []struct {
		name       string
		body       string
		err        error
		wantStatus int
		wantCode   string
		wantPrice  int64
	}{
		{
			name:       "success",
			body:       body,
			wantStatus: http.StatusCreated,
			wantPrice:  1450,
		},
		{
			name:       "zero price",
			body:       tripFields + `,"price":0}`,
			wantStatus: http.StatusCreated,
			wantPrice:  0,
		},
		{
			name:       "busy",
			body:       body,
			err:        trip.ErrDriverBusy,
			wantStatus: http.StatusConflict,
			wantCode:   "driver_busy",
			wantPrice:  1450,
		},
		{
			name:       "failure",
			body:       body,
			err:        errors.New("secret SQL"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "internal_error",
			wantPrice:  1450,
		},
		{
			name:       "invalid",
			body:       `{}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   invalidRequestCode,
		},
		{
			name:       "unknown",
			body:       tripFields + `,"price":1450,"extra":1}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   invalidRequestCode,
		},
		{
			name:       "missing price",
			body:       tripFields + "}",
			wantStatus: http.StatusBadRequest,
			wantCode:   invalidRequestCode,
		},
		{
			name:       "negative",
			body:       tripFields + `,"price":-1}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   invalidRequestCode,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			usecase := NewMocktripUsecase(ctrl)
			id := uuid.New()
			input := createusecase.Input{
				UserID:     uuid.MustParse("5cb72c04-7650-45c9-a79b-bcdba0631e0c"),
				DriverID:   uuid.MustParse("8860b315-ec86-42eb-a17c-7c163d721ff5"),
				StartPoint: trip.Coordinates{Latitude: 59.9398, Longitude: 30.3146},
				EndPoint:   trip.Coordinates{Latitude: 59.929, Longitude: 30.3626},
				Price:      tc.wantPrice,
			}
			started := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
			if tc.wantStatus != http.StatusBadRequest {
				usecase.EXPECT().CreateTrip(gomock.Any(), input).Return(&trip.Trip{
					TripID: id, UserID: input.UserID, DriverID: input.DriverID,
					StartPoint: input.StartPoint, EndPoint: input.EndPoint, Price: input.Price,
					Status: trip.StatusActive, StartedAt: started,
				}, tc.err)
			}
			h := create.New(usecase, slog.New(slog.NewTextHandler(io.Discard, nil)))
			r := httptest.NewRequest("POST", "/api/v1/trips", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.CreateTrip(w, r, api.CreateTripParams{})
			if w.Code != tc.wantStatus {
				t.Fatalf("status %d: %s", w.Code, w.Body)
			}
			if strings.Contains(w.Body.String(), "secret") {
				t.Fatal("internal error leaked")
			}
			if tc.wantStatus == http.StatusCreated && w.Header().Get("Location") != "/api/v1/trips/"+id.String() {
				t.Fatal("wrong location")
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
			want := api.Trip{
				Id: id, UserId: input.UserID, DriverId: input.DriverID, Status: api.Active, StartedAt: started,
				StartPoint: api.Coordinates{Latitude: 59.9398, Longitude: 30.3146},
				EndPoint:   api.Coordinates{Latitude: 59.929, Longitude: 30.3626},
				Price:      input.Price,
			}
			if w.Header().Get("Content-Type") != "application/json" || actual != want {
				t.Fatalf("unexpected trip: %+v", actual)
			}
		})
	}
}
