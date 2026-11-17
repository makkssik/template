//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package list_test
package list

import (
	"context"

	"github.com/google/uuid"

	"github.com/makkssik/template/internal/domain/position"
	"github.com/makkssik/template/internal/domain/trip"
)

type tripRepository interface {
	GetTripByID(context.Context, uuid.UUID) (*trip.Trip, error)
}

type positionRepository interface {
	GetPositionsByTripID(context.Context, uuid.UUID) ([]position.Position, error)
}
