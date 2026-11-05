package create

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	domaintrips "github.com/makkssik/template/internal/domain/trip"
)

func (s *Usecase) CreateTrip(ctx context.Context, opts Input) (*domaintrips.Trip, error) {
	now := time.Now().UTC()
	trip, err := domaintrips.New(uuid.New(), now, domaintrips.NewParams{
		UserID:     opts.UserID,
		DriverID:   opts.DriverID,
		StartPoint: opts.StartPoint,
		EndPoint:   opts.EndPoint,
		Price:      opts.Price,
	})
	if err != nil {
		return nil, err
	}
	err = s.txManager.Do(ctx, func(txCtx context.Context) error {
		if err := s.tripsRepository.CreateTrip(txCtx, trip); err != nil {
			return err
		}
		return s.statusHistoryRepository.RegisterStatusChange(txCtx, domaintrips.StatusChange{
			TripID:    trip.TripID,
			ToStatus:  domaintrips.StatusActive,
			ChangedAt: trip.StartedAt,
		})
	})
	if err != nil {
		return nil, fmt.Errorf("create trip: %w", err)
	}
	s.events.Created(ctx, trip.TripID)
	return &trip, nil
}

type Input struct {
	UserID     uuid.UUID
	DriverID   uuid.UUID
	StartPoint domaintrips.Coordinates
	EndPoint   domaintrips.Coordinates
	Price      int64
}

type Usecase struct {
	events                  events
	tripsRepository         tripRepository
	statusHistoryRepository historyRepository
	txManager               txManager
}

func New(repo tripRepository, history historyRepository, tx txManager, observer events) *Usecase {
	return &Usecase{tripsRepository: repo, statusHistoryRepository: history, txManager: tx, events: observer}
}
