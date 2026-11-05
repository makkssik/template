package trip_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	trip "github.com/makkssik/template/internal/domain/trip"
)

func TestFinish(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	value := trip.Trip{Status: trip.StatusActive}
	if err := value.Finish(at); err != nil {
		t.Fatal(err)
	}
	if value.Status != trip.StatusCompleted || value.FinishedAt == nil || !value.FinishedAt.Equal(at) || !value.UpdatedAt.Equal(at) {
		t.Fatalf("wrong transition: %+v", value)
	}
	if err := value.Finish(at.Add(time.Hour)); !errors.Is(err, trip.ErrTripCompleted) {
		t.Fatalf("wrong error: %v", err)
	}
	if !value.FinishedAt.Equal(at) {
		t.Fatal("repeated finish changed timestamp")
	}
}

func TestNew(t *testing.T) {
	id := uuid.New()
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		change func(*trip.NewParams)
		field  string
	}{
		{"valid", func(*trip.NewParams) {}, ""},
		{"zero price", func(v *trip.NewParams) { v.Price = 0 }, ""},
		{"user", func(v *trip.NewParams) { v.UserID = uuid.Nil }, "user_id"},
		{"driver", func(v *trip.NewParams) { v.DriverID = uuid.Nil }, "driver_id"},
		{"price", func(v *trip.NewParams) { v.Price = -1 }, "price"},
		{"latitude", func(v *trip.NewParams) { v.StartPoint.Latitude = 91 }, "start_point.latitude"},
		{"longitude", func(v *trip.NewParams) { v.EndPoint.Longitude = -181 }, "end_point.longitude"},
		{"nan", func(v *trip.NewParams) { v.EndPoint.Latitude = math.NaN() }, "end_point.latitude"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params := trip.NewParams{
				UserID:     uuid.New(),
				DriverID:   uuid.New(),
				StartPoint: trip.Coordinates{Latitude: 59.9, Longitude: 30.3},
				EndPoint:   trip.Coordinates{Latitude: 60, Longitude: 30.4},
				Price:      1450,
			}
			tc.change(&params)
			got, err := trip.New(id, at, params)
			if tc.field == "" {
				if err != nil {
					t.Fatal(err)
				}
				if got.TripID != id || got.UserID != params.UserID || got.DriverID != params.DriverID ||
					got.StartPoint != params.StartPoint || got.EndPoint != params.EndPoint || got.Price != params.Price {
					t.Fatalf("wrong trip data: %+v", got)
				}
				if got.Status != trip.StatusActive || got.FinishedAt != nil ||
					!got.StartedAt.Equal(at) || !got.CreatedAt.Equal(at) || !got.UpdatedAt.Equal(at) {
					t.Fatalf("wrong initial state: %+v", got)
				}
				return
			}
			var ve *trip.ValidationError
			if !errors.As(err, &ve) || ve.Field != tc.field {
				t.Fatalf("unexpected validation: %v", err)
			}
			if got != (trip.Trip{}) {
				t.Fatalf("trip returned after validation failure: %+v", got)
			}
		})
	}
}
