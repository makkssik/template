package trip

import (
	"math"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusActive    Status = "active"
	StatusCompleted Status = "completed"
)

type Coordinates struct {
	Latitude  float64
	Longitude float64
}

type Trip struct {
	TripID   uuid.UUID
	UserID   uuid.UUID
	DriverID uuid.UUID

	StartPoint Coordinates
	EndPoint   Coordinates
	Price      int64
	Status     Status

	StartedAt  time.Time
	FinishedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type NewParams struct {
	UserID     uuid.UUID
	DriverID   uuid.UUID
	StartPoint Coordinates
	EndPoint   Coordinates
	Price      int64
}

func New(id uuid.UUID, at time.Time, params NewParams) (Trip, error) {
	trip := Trip{
		TripID:     id,
		UserID:     params.UserID,
		DriverID:   params.DriverID,
		StartPoint: params.StartPoint,
		EndPoint:   params.EndPoint,
		Price:      params.Price,
		Status:     StatusActive,
		StartedAt:  at,
		CreatedAt:  at,
		UpdatedAt:  at,
	}
	if err := trip.validate(); err != nil {
		return Trip{}, err
	}
	return trip, nil
}

func (t *Trip) Finish(at time.Time) error {
	if t.Status != StatusActive {
		return ErrTripCompleted
	}
	t.Status = StatusCompleted
	t.FinishedAt = &at
	t.UpdatedAt = at
	return nil
}

func (t Trip) validate() error {
	if t.UserID == uuid.Nil {
		return &ValidationError{Field: "user_id", Detail: "must be a non-zero UUID"}
	}
	if t.DriverID == uuid.Nil {
		return &ValidationError{Field: "driver_id", Detail: "must be a non-zero UUID"}
	}
	if t.Price < 0 {
		return &ValidationError{Field: "price", Detail: "must be non-negative"}
	}
	if math.IsNaN(t.StartPoint.Latitude) || t.StartPoint.Latitude < -90 || t.StartPoint.Latitude > 90 {
		return &ValidationError{Field: "start_point.latitude", Detail: "must be between -90 and 90"}
	}
	if math.IsNaN(t.StartPoint.Longitude) || t.StartPoint.Longitude < -180 || t.StartPoint.Longitude > 180 {
		return &ValidationError{Field: "start_point.longitude", Detail: "must be between -180 and 180"}
	}
	if math.IsNaN(t.EndPoint.Latitude) || t.EndPoint.Latitude < -90 || t.EndPoint.Latitude > 90 {
		return &ValidationError{Field: "end_point.latitude", Detail: "must be between -90 and 90"}
	}
	if math.IsNaN(t.EndPoint.Longitude) || t.EndPoint.Longitude < -180 || t.EndPoint.Longitude > 180 {
		return &ValidationError{Field: "end_point.longitude", Detail: "must be between -180 and 180"}
	}
	return nil
}
