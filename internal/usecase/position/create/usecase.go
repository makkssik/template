package create

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/makkssik/template/internal/domain/position"
	"github.com/makkssik/template/internal/domain/trip"
)

func (s *Usecase) CreatePosition(ctx context.Context, opts Input) (*position.Position, error) {
	if err := position.Validate(opts.Coordinates, opts.RecordedAt, time.Now()); err != nil {
		return nil, err
	}
	point := position.Position{
		TripID:      opts.TripID,
		Coordinates: opts.Coordinates,
		RecordedAt:  opts.RecordedAt.UTC().Truncate(time.Microsecond),
	}
	var savedPosition *position.Position

	err := s.txManager.Do(ctx, func(txCtx context.Context) error {
		currentTrip, err := s.tripsRepository.GetTripForUpdate(txCtx, point.TripID)
		if err != nil {
			return err
		}
		if currentTrip.Status != trip.StatusActive {
			return trip.ErrTripCompleted
		}
		savedPosition, err = s.positionsRepository.SavePosition(txCtx, point)
		if err != nil {
			return err
		}
		return s.tripsRepository.UpdateLastPosition(txCtx, point.TripID, savedPosition.RecordedAt)
	})
	if err != nil {
		return nil, fmt.Errorf("create position: %w", err)
	}
	s.positionRequestCanceler.Cancel(ctx, point.TripID)
	s.events.Saved(ctx, point.TripID)
	return savedPosition, nil
}

type Input struct {
	TripID      uuid.UUID
	Coordinates trip.Coordinates
	RecordedAt  time.Time
}

type Usecase struct {
	events                  events
	tripsRepository         tripRepository
	positionsRepository     positionRepository
	txManager               txManager
	positionRequestCanceler positionRequestCanceler
}

func New(trips tripRepository, positions positionRepository, tx txManager, canceler positionRequestCanceler, observer events) *Usecase {
	return &Usecase{
		tripsRepository:         trips,
		positionsRepository:     positions,
		txManager:               tx,
		positionRequestCanceler: canceler,
		events:                  observer,
	}
}
