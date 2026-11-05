//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package finish_test
package finish

import (
	"context"

	"github.com/google/uuid"

	trip "github.com/makkssik/template/internal/domain/trip"
)

type tripUsecase interface {
	FinishTrip(context.Context, uuid.UUID) (*trip.Trip, error)
}
