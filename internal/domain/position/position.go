package position

import (
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/makkssik/template/internal/domain/trip"
)

type Position struct {
	ID          int64
	TripID      uuid.UUID
	Coordinates trip.Coordinates
	RecordedAt  time.Time
}

func Validate(point trip.Coordinates, at, now time.Time) error {
	if math.IsNaN(point.Latitude) || point.Latitude < -90 || point.Latitude > 90 {
		return &trip.ValidationError{Field: "coordinates.latitude", Detail: "must be between -90 and 90"}
	}
	if math.IsNaN(point.Longitude) || point.Longitude < -180 || point.Longitude > 180 {
		return &trip.ValidationError{Field: "coordinates.longitude", Detail: "must be between -180 and 180"}
	}
	if at.IsZero() || at.After(now.Add(time.Minute)) {
		return &trip.ValidationError{Field: "recorded_at", Detail: "required and must not be more than one minute in the future"}
	}
	return nil
}
