package position

import (
	domain "github.com/makkssik/template/internal/domain/position"
	api "github.com/makkssik/template/internal/generated"
)

func ToAPI(p domain.Position) api.TripPosition {
	return api.TripPosition{Id: p.ID, TripId: p.TripID, RecordedAt: p.RecordedAt, Coordinates: api.Coordinates{Latitude: p.Coordinates.Latitude, Longitude: p.Coordinates.Longitude}}
}
