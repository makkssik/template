package trip

import (
	trips "github.com/makkssik/template/internal/domain/trip"
	api "github.com/makkssik/template/internal/generated"
)

func ToAPI(trip trips.Trip) api.Trip {
	return api.Trip{
		Id:       trip.TripID,
		UserId:   trip.UserID,
		DriverId: trip.DriverID,
		StartPoint: api.Coordinates{
			Latitude:  trip.StartPoint.Latitude,
			Longitude: trip.StartPoint.Longitude,
		},
		EndPoint: api.Coordinates{
			Latitude:  trip.EndPoint.Latitude,
			Longitude: trip.EndPoint.Longitude,
		},
		Price:      trip.Price,
		Status:     api.TripStatus(trip.Status),
		StartedAt:  trip.StartedAt,
		FinishedAt: trip.FinishedAt,
	}
}
