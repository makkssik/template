//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package finish_test
package finish

import (
	"context"

	"github.com/google/uuid"

	trip "github.com/makkssik/template/internal/domain/trip"
)

type tripRepository interface {
	GetTripForUpdate(context.Context, uuid.UUID) (*trip.Trip, error)
	SaveTrip(context.Context, trip.Trip) (*trip.Trip, error)
}
type historyRepository interface {
	RegisterStatusChange(context.Context, trip.StatusChange) error
}
type txManager interface {
	Do(context.Context, func(context.Context) error) error
}

type events interface {
	Completed(context.Context, uuid.UUID)
}
