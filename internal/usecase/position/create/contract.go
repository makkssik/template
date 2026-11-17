//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package create_test
package create

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/makkssik/template/internal/domain/position"
	"github.com/makkssik/template/internal/domain/trip"
)

type tripRepository interface {
	GetTripForUpdate(context.Context, uuid.UUID) (*trip.Trip, error)
	UpdateLastPosition(context.Context, uuid.UUID, time.Time) error
}

type positionRepository interface {
	SavePosition(context.Context, position.Position) (*position.Position, error)
}

type txManager interface {
	Do(context.Context, func(context.Context) error) error
}

type positionRequestCanceler interface {
	Cancel(context.Context, uuid.UUID)
}

type events interface {
	Saved(context.Context, uuid.UUID)
}
