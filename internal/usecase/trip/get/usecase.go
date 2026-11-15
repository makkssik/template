package get

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	domaintrips "github.com/makkssik/template/internal/domain/trip"
)

func (s *Usecase) GetTrip(ctx context.Context, tripID uuid.UUID) (*domaintrips.Trip, error) {
	trip, err := s.tripsRepository.GetTripByID(ctx, tripID)
	if err != nil {
		return nil, fmt.Errorf("get trip: %w", err)
	}
	return trip, nil
}

type Usecase struct{ tripsRepository tripRepository }

func New(repo tripRepository) *Usecase { return &Usecase{tripsRepository: repo} }
