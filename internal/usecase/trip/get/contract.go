//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package get_test
package get

import (
	"context"

	"github.com/google/uuid"

	trip "github.com/makkssik/template/internal/domain/trip"
)

type tripRepository interface {
	GetTripByID(context.Context, uuid.UUID) (*trip.Trip, error)
}
