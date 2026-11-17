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
	failure := errors.New("dependency failed")
	tests := []struct {
		name         string
		invalidPrice bool
		beginErr     error
		createErr    error
		historyErr   error
		wantErr      error
	}{
		{name: "trip created"},
		{name: "driver busy", createErr: trip.ErrDriverBusy, wantErr: trip.ErrDriverBusy},
		{name: "history fails", historyErr: failure, wantErr: failure},
		{name: "begin fails", beginErr: failure, wantErr: failure},
		{name: "negative price", invalidPrice: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMocktripRepository(ctrl)
			history := NewMockhistoryRepository(ctrl)
			tx := NewMocktxManager(ctrl)
			observer := NewMockevents(ctrl)
			if !tc.invalidPrice && tc.wantErr == nil {
				observer.EXPECT().Created(gomock.Any(), gomock.Any())
			}
			var saved trip.Trip
			input := create.Input{UserID: uuid.New(), DriverID: uuid.New(), Price: 10}
			if tc.invalidPrice {
				input.Price = -1
			} else {
				tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
					if tc.beginErr != nil {
						return tc.beginErr
					}
					return fn(ctx)
				})
				if tc.beginErr == nil {
					repo.EXPECT().CreateTrip(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, v trip.Trip) error {
						saved = v
						assertCreationTimestamps(t, v)
						return tc.createErr
					})
					if tc.createErr == nil {
						history.EXPECT().RegisterStatusChange(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, change trip.StatusChange) error {
							if change.TripID != saved.TripID || change.FromStatus != nil || change.ToStatus != trip.StatusActive {
								t.Fatalf("expected initial transition to active, got %+v", change)
							}
							if saved.StartedAt.IsZero() || !change.ChangedAt.Equal(saved.StartedAt) {
								t.Fatalf("history time %v differs from start time %v", change.ChangedAt, saved.StartedAt)
							}
							return tc.historyErr
						})
					}
				}
			}
			got, err := create.New(repo, history, tx, observer).CreateTrip(context.Background(), input)
			if tc.invalidPrice {
				var ve *trip.ValidationError
				if !errors.As(err, &ve) {
					t.Fatalf("validation: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("got %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr == nil && (got == nil || got.TripID != saved.TripID || got.UserID != input.UserID || got.DriverID != input.DriverID || got.Price != input.Price || got.Status != trip.StatusActive) {
				t.Fatal("missing result")
			}
			if tc.wantErr != nil && got != nil {
				t.Fatal("result returned after failure")
			}
			if tc.wantErr == nil && got.StartedAt != saved.StartedAt {
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
