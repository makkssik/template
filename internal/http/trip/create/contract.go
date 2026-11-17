//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package create_test
package create

import (
	"context"

	trip "github.com/makkssik/template/internal/domain/trip"
	"github.com/makkssik/template/internal/usecase/trip/create"
)

type tripUsecase interface {
	CreateTrip(context.Context, create.Input) (*trip.Trip, error)
}
