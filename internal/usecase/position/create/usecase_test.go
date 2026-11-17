package create_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/makkssik/template/internal/domain/position"
	"github.com/makkssik/template/internal/domain/trip"
	"github.com/makkssik/template/internal/usecase/position/create"
)

func TestCreatePosition(t *testing.T) {
	failure := errors.New("dependency failed")
	tests := []struct {
		name        string
		invalidTime bool
		completed   bool
		loadErr     error
		saveErr     error
		updateErr   error
		commitErr   error
		wantErr     error
	}{
		{name: "position saved"},
		{name: "missing recorded time", invalidTime: true},
		{name: "trip not found", loadErr: trip.ErrTripNotFound, wantErr: trip.ErrTripNotFound},
		{name: "trip already completed", completed: true, wantErr: trip.ErrTripCompleted},
		{name: "save fails", saveErr: failure, wantErr: failure},
		{name: "update fails", updateErr: failure, wantErr: failure},
		{name: "commit fails", commitErr: failure, wantErr: failure},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			trips := NewMocktripRepository(ctrl)
			positions := NewMockpositionRepository(ctrl)
			tx := NewMocktxManager(ctrl)
			canceler := NewMockpositionRequestCanceler(ctrl)
			events := NewMockevents(ctrl)
			input := create.Input{TripID: uuid.New(), RecordedAt: time.Now().UTC().Add(-time.Second)}
			committed := false
			if tc.invalidTime {
				input.RecordedAt = time.Time{}
			} else {
				tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
					if err := fn(ctx); err != nil {
						return err
					}
					if tc.commitErr != nil {
						return tc.commitErr
					}
					committed = true
					return nil
				})
				value := &trip.Trip{Status: trip.StatusActive}
				if tc.completed {
					value.Status = trip.StatusCompleted
				}
				load := trips.EXPECT().GetTripForUpdate(gomock.Any(), input.TripID).Return(value, tc.loadErr)
				if tc.loadErr == nil && !tc.completed {
					saved := position.Position{
						ID:          42,
						TripID:      input.TripID,
						Coordinates: input.Coordinates,
						RecordedAt:  input.RecordedAt.Truncate(time.Microsecond),
					}
					save := positions.EXPECT().SavePosition(gomock.Any(), gomock.Any()).After(load).DoAndReturn(func(_ context.Context, p position.Position) (*position.Position, error) {
						if p.RecordedAt != saved.RecordedAt {
							t.Fatal("timestamp not normalized")
						}
						return &saved, tc.saveErr
					})
					if tc.saveErr == nil {
						trips.EXPECT().UpdateLastPosition(gomock.Any(), input.TripID, saved.RecordedAt).After(save).Return(tc.updateErr)
					}
				}
			}
			if !tc.invalidTime && tc.wantErr == nil {
				canceler.EXPECT().Cancel(gomock.Any(), input.TripID).Do(func(context.Context, uuid.UUID) {
					if !committed {
						t.Fatal("canceled before commit")
					}
				})
				events.EXPECT().Saved(gomock.Any(), input.TripID).Do(func(context.Context, uuid.UUID) {
					if !committed {
						t.Fatal("event before commit")
					}
				})
			}
			got, err := create.New(trips, positions, tx, canceler, events).CreatePosition(t.Context(), input)
			if tc.invalidTime {
				var validation *trip.ValidationError
				if !errors.As(err, &validation) {
					t.Fatalf("validation: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("got %v want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil && got != nil {
				t.Fatal("failed transaction returned a position")
			}
			if tc.wantErr == nil && (got.ID != 42 || got.TripID != input.TripID) {
				t.Fatalf("wrong saved position: %+v", got)
			}
		})
	}
}
