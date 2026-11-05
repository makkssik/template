package pg

import (
	"time"

	"github.com/google/uuid"

	trips "github.com/makkssik/template/internal/domain/trip"
)

type Trip struct {
	TripID   uuid.UUID `db:"id"`
	UserID   uuid.UUID `db:"user_id"`
	DriverID uuid.UUID `db:"driver_id"`

	StartLatitude  float64 `db:"start_latitude"`
	StartLongitude float64 `db:"start_longitude"`
	EndLatitude    float64 `db:"end_latitude"`
	EndLongitude   float64 `db:"end_longitude"`
	Price          int64   `db:"price"`
	Status         string  `db:"status"`

	StartedAt  time.Time  `db:"started_at"`
	FinishedAt *time.Time `db:"finished_at"`
	CreatedAt  time.Time  `db:"created_at"`
	UpdatedAt  time.Time  `db:"updated_at"`
}

func dbToDomain(m Trip) trips.Trip {
	return trips.Trip{
		TripID:   m.TripID,
		UserID:   m.UserID,
		DriverID: m.DriverID,
		StartPoint: trips.Coordinates{
			Latitude:  m.StartLatitude,
			Longitude: m.StartLongitude,
		},
		EndPoint: trips.Coordinates{
			Latitude:  m.EndLatitude,
			Longitude: m.EndLongitude,
		},
		Price:      m.Price,
		Status:     trips.Status(m.Status),
		StartedAt:  m.StartedAt,
		FinishedAt: m.FinishedAt,
		CreatedAt:  m.CreatedAt,
		UpdatedAt:  m.UpdatedAt,
	}
}
