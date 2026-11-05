package create_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	trip "github.com/makkssik/template/internal/domain/trip"
	"github.com/makkssik/template/internal/usecase/trip/create"
)

func TestOperation(t *testing.T) {
	for _, stage := range []string{"success", "repository", "history", "transaction", "invalid"} {
		t.Run(stage, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMocktripRepository(ctrl)
			history := NewMockhistoryRepository(ctrl)
			tx := NewMocktxManager(ctrl)
			observer := NewMockevents(ctrl)
			if stage == "success" {
				observer.EXPECT().Created(gomock.Any(), gomock.Any())
			}
			failure := errors.New("dependency failed")
			var wantErr error
			var saved trip.Trip
			input := create.Input{UserID: uuid.New(), DriverID: uuid.New(), Price: 10}
			if stage == "invalid" {
				input.Price = -1
			} else {
				tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
					if stage == "transaction" {
						return failure
					}
					return fn(ctx)
				})
				if stage == "transaction" {
					wantErr = failure
				} else {
					repo.EXPECT().CreateTrip(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, v trip.Trip) error {
						saved = v
						if stage == "repository" {
							return trip.ErrDriverBusy
						}
						return nil
					})
					if stage == "repository" {
						wantErr = trip.ErrDriverBusy
					} else {
						history.EXPECT().RegisterStatusChange(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, change trip.StatusChange) error {
							if change.TripID != saved.TripID || change.FromStatus != nil || change.ToStatus != trip.StatusActive {
								t.Fatalf("expected initial transition to active, got %+v", change)
							}
							if saved.StartedAt.IsZero() || !change.ChangedAt.Equal(saved.StartedAt) {
								t.Fatalf("history time %v differs from start time %v", change.ChangedAt, saved.StartedAt)
							}
							if stage == "history" {
								return failure
							}
							return nil
						})
						if stage == "history" {
							wantErr = failure
						}
					}
				}
			}
			got, err := create.New(repo, history, tx, observer).CreateTrip(context.Background(), input)
			if stage == "invalid" {
				var ve *trip.ValidationError
				if !errors.As(err, &ve) {
					t.Fatalf("validation: %v", err)
				}
				return
			}
			if !errors.Is(err, wantErr) {
				t.Fatalf("got %v, want %v", err, wantErr)
			}
			if wantErr == nil && (got == nil || got.TripID != saved.TripID || got.UserID != input.UserID || got.DriverID != input.DriverID || got.Price != input.Price || got.Status != trip.StatusActive) {
				t.Fatal("missing result")
			}
			if wantErr != nil && got != nil {
				t.Fatal("result returned after failure")
			}
		})
	}
}
