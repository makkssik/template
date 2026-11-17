package list

import (
	"context"

	"github.com/google/uuid"

	"github.com/makkssik/template/internal/domain/position"
)

func (s *Usecase) List(ctx context.Context, tripID uuid.UUID) ([]position.Position, error) {
	if _, err := s.tripsRepository.GetTripByID(ctx, tripID); err != nil {
		return nil, err
	}
	return s.positionsRepository.GetPositionsByTripID(ctx, tripID)
}

type Usecase struct {
	tripsRepository     tripRepository
	positionsRepository positionRepository
}

func New(trips tripRepository, positions positionRepository) *Usecase {
	return &Usecase{
		tripsRepository:     trips,
		positionsRepository: positions,
	}
}
