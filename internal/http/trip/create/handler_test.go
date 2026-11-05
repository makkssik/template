package create_test

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
	"github.com/makkssik/template/internal/http/trip/create"
	createusecase "github.com/makkssik/template/internal/usecase/trip/create"
)

func TestHandler(t *testing.T) {
	body := `{"user_id":"5cb72c04-7650-45c9-a79b-bcdba0631e0c","driver_id":"8860b315-ec86-42eb-a17c-7c163d721ff5","start_point":{"latitude":59.9398,"longitude":30.3146},"end_point":{"latitude":59.929,"longitude":30.3626},"price":1450}`
	for _, tc := range []struct {
		name, body string
		err        error
		status     int
		code       string
	}{
		{"success", body, nil, 201, ""},
		{"zero price", strings.Replace(body, `"price":1450`, `"price":0`, 1), nil, 201, ""},
		{"busy", body, trip.ErrDriverBusy, 409, "driver_busy"},
		{"failure", body, errors.New("secret SQL"), 500, "internal_error"},
		{"invalid", `{}`, nil, 400, "invalid_request"},
		{"unknown", strings.Replace(body, `"price":1450`, `"price":1450,"extra":1`, 1), nil, 400, "invalid_request"},
		{"missing price", strings.Replace(body, `,"price":1450`, "", 1), nil, 400, "invalid_request"},
		{"negative", strings.Replace(body, `"price":1450`, `"price":-1`, 1), nil, 400, "invalid_request"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			usecase := NewMocktripUsecase(ctrl)
			id := uuid.New()
			input := createusecase.Input{
				UserID:     uuid.MustParse("5cb72c04-7650-45c9-a79b-bcdba0631e0c"),
				DriverID:   uuid.MustParse("8860b315-ec86-42eb-a17c-7c163d721ff5"),
				StartPoint: trip.Coordinates{Latitude: 59.9398, Longitude: 30.3146},
				EndPoint:   trip.Coordinates{Latitude: 59.929, Longitude: 30.3626},
				Price:      1450,
			}
			if tc.name == "zero price" {
				input.Price = 0
			}
			started := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
			if tc.status != 400 {
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
			if w.Code != tc.status {
				t.Fatalf("status %d: %s", w.Code, w.Body)
			}
			if strings.Contains(w.Body.String(), "secret") {
				t.Fatal("internal error leaked")
			}
			if tc.status == 201 && w.Header().Get("Location") != "/api/v1/trips/"+id.String() {
				t.Fatal("wrong location")
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
