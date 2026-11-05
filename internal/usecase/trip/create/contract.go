//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package create_test
package create

import (
	"context"

	"github.com/google/uuid"

	trip "github.com/makkssik/template/internal/domain/trip"
)

type tripRepository interface {
	CreateTrip(context.Context, trip.Trip) error
}
type historyRepository interface {
	RegisterStatusChange(context.Context, trip.StatusChange) error
}
type txManager interface {
	Do(context.Context, func(context.Context) error) error
}

type events interface {
	Created(context.Context, uuid.UUID)
}
