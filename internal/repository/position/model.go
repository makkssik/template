package position

import (
	"time"

	"github.com/google/uuid"

	positions "github.com/makkssik/template/internal/domain/position"
	"github.com/makkssik/template/internal/domain/trip"
)

type Position struct {
	ID         int64     `db:"id"`
	TripID     uuid.UUID `db:"trip_id"`
	Latitude   float64   `db:"latitude"`
	Longitude  float64   `db:"longitude"`
	RecordedAt time.Time `db:"recorded_at"`
}

func domainToDB(point positions.Position) Position {
	return Position{
		ID:         point.ID,
		TripID:     point.TripID,
		Latitude:   point.Coordinates.Latitude,
		Longitude:  point.Coordinates.Longitude,
		RecordedAt: point.RecordedAt,
	}
}

func dbToDomain(model Position) positions.Position {
	return positions.Position{
		ID:     model.ID,
		TripID: model.TripID,
		Coordinates: trip.Coordinates{
			Latitude:  model.Latitude,
			Longitude: model.Longitude,
		},
		RecordedAt: model.RecordedAt.UTC(),
	}
}
