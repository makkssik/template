package finish

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/samber/lo"

	domaintrips "github.com/makkssik/template/internal/domain/trip"
)

func (s *Usecase) FinishTrip(ctx context.Context, tripID uuid.UUID) (*domaintrips.Trip, error) {
	var trip *domaintrips.Trip

	err := s.txManager.Do(ctx, func(txCtx context.Context) error {
		var err error
		trip, err = s.tripsRepository.GetTripForUpdate(txCtx, tripID)
		if err != nil {
			return err
		}
		fromStatus := trip.Status
		if err := trip.Finish(time.Now().UTC()); err != nil {
			return err
		}
		trip, err = s.tripsRepository.SaveTrip(txCtx, *trip)
		if err != nil {
			return err
		}

		return s.statusHistoryRepository.RegisterStatusChange(txCtx, domaintrips.StatusChange{
			TripID:     trip.TripID,
			FromStatus: lo.ToPtr(fromStatus),
			ToStatus:   trip.Status,
			ChangedAt:  *trip.FinishedAt,
		})
	})
	if err != nil {
		return nil, fmt.Errorf("finish trip: %w", err)
	}
	s.events.Completed(ctx, trip.TripID)
	return trip, nil
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
