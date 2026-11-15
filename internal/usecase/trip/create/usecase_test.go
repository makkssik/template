package create_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	trip "github.com/makkssik/template/internal/domain/trip"
	"github.com/makkssik/template/internal/usecase/trip/create"
)

func TestOperation(t *testing.T) {
	const (
		historyStage     = "history"
		invalidStage     = "invalid"
		repositoryStage  = "repository"
		transactionStage = "transaction"
	)
	for _, stage := range []string{"success", repositoryStage, historyStage, transactionStage, invalidStage} {
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
			if stage == invalidStage {
				input.Price = -1
			} else {
				tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
					if stage == transactionStage {
						return failure
					}
					return fn(ctx)
				})
				if stage == transactionStage {
					wantErr = failure
				} else {
					repo.EXPECT().CreateTrip(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, v trip.Trip) error {
						saved = v
						assertCreationTimestamps(t, v)
						if stage == repositoryStage {
							return trip.ErrDriverBusy
						}
						return nil
					})
					if stage == repositoryStage {
						wantErr = trip.ErrDriverBusy
					} else {
						history.EXPECT().RegisterStatusChange(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, change trip.StatusChange) error {
							if change.TripID != saved.TripID || change.FromStatus != nil || change.ToStatus != trip.StatusActive {
								t.Fatalf("expected initial transition to active, got %+v", change)
							}
							if saved.StartedAt.IsZero() || !change.ChangedAt.Equal(saved.StartedAt) {
								t.Fatalf("history time %v differs from start time %v", change.ChangedAt, saved.StartedAt)
							}
							if stage == historyStage {
								return failure
							}
							return nil
						})
						if stage == historyStage {
							wantErr = failure
						}
					}
				}
			}
			got, err := create.New(repo, history, tx, observer).CreateTrip(context.Background(), input)
			if stage == invalidStage {
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
			if wantErr == nil && got.StartedAt != saved.StartedAt {
				t.Fatalf("response start time %v differs from saved time %v", got.StartedAt, saved.StartedAt)
			}
		})
	}
}

func assertCreationTimestamps(t *testing.T, value trip.Trip) {
	t.Helper()
	if value.StartedAt.Location() != time.UTC || value.StartedAt.Nanosecond()%1000 != 0 || value.CreatedAt != value.StartedAt || value.UpdatedAt != value.StartedAt {
		t.Fatalf("creation timestamps must use UTC and microsecond precision: %+v", value)
	}
}
