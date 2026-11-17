package finish_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	trip "github.com/makkssik/template/internal/domain/trip"
	"github.com/makkssik/template/internal/usecase/trip/finish"
)

func TestOperation(t *testing.T) {
	failure := errors.New("dependency failed")
	tests := []struct {
		name       string
		completed  bool
		beginErr   error
		loadErr    error
		saveErr    error
		historyErr error
		commitErr  error
		wantErr    error
	}{
		{name: "trip finished"},
		{name: "load fails", loadErr: failure, wantErr: failure},
		{name: "save fails", saveErr: failure, wantErr: failure},
		{name: "history fails", historyErr: failure, wantErr: failure},
		{name: "begin fails", beginErr: failure, wantErr: failure},
		{name: "commit fails", commitErr: failure, wantErr: failure},
		{name: "trip already completed", completed: true, wantErr: trip.ErrTripCompleted},
		{name: "trip not found", loadErr: trip.ErrTripNotFound, wantErr: trip.ErrTripNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMocktripRepository(ctrl)
			history := NewMockhistoryRepository(ctrl)
			tx := NewMocktxManager(ctrl)
			observer := NewMockevents(ctrl)
			id := uuid.New()
			loaded := trip.Trip{
				TripID: id, UserID: uuid.New(), DriverID: uuid.New(),
				Price: 1450, Status: trip.StatusActive,
				StartPoint: trip.Coordinates{Latitude: 59, Longitude: 30},
				EndPoint:   trip.Coordinates{Latitude: 60, Longitude: 31},
				StartedAt:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			}
			if tc.completed {
				at := loaded.StartedAt.Add(time.Hour)
				loaded.Status, loaded.FinishedAt, loaded.UpdatedAt = trip.StatusCompleted, &at, at
			}
			before := loaded
			var saved trip.Trip
			committed := false
			type transactionKey struct{}
			txCtx := context.WithValue(t.Context(), transactionKey{}, true)
			tx.EXPECT().Do(t.Context(), gomock.Any()).DoAndReturn(func(_ context.Context, fn func(context.Context) error) error {
				if tc.beginErr != nil {
					return tc.beginErr
				}
				if err := fn(txCtx); err != nil {
					return err
				}
				if tc.commitErr != nil {
					return tc.commitErr
				}
				committed = true
				return nil
			})
			if tc.beginErr == nil {
				load := repo.EXPECT().GetTripForUpdate(txCtx, id).Return(&loaded, tc.loadErr)
				if tc.loadErr == nil && !tc.completed {
					save := repo.EXPECT().SaveTrip(txCtx, gomock.Any()).After(load).DoAndReturn(func(_ context.Context, value trip.Trip) (*trip.Trip, error) {
						assertFinishedTrip(t, before, value)
						if tc.saveErr != nil {
							return nil, tc.saveErr
						}
						at := value.FinishedAt.Truncate(time.Microsecond)
						saved = value
						saved.FinishedAt, saved.UpdatedAt = &at, at
						return &saved, nil
					})
					if tc.saveErr == nil {
						history.EXPECT().RegisterStatusChange(txCtx, gomock.Any()).After(save).DoAndReturn(func(_ context.Context, change trip.StatusChange) error {
							assertHistory(t, change, before, saved)
							return tc.historyErr
						})
					}
				}
			}
			if tc.wantErr == nil {
				observer.EXPECT().Completed(t.Context(), id).Do(func(context.Context, uuid.UUID) {
					if !committed {
						t.Fatal("completion event emitted before commit")
					}
				})
			}
			got, err := finish.New(repo, history, tx, observer).FinishTrip(t.Context(), id)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("got %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr == nil && (got == nil || *got != saved) {
				t.Fatalf("wrong result: got %+v, want %+v", got, saved)
			}
			if tc.wantErr != nil && got != nil {
				t.Fatal("result returned after failure")
			}
			if tc.completed && loaded != before {
				t.Fatal("repeated finish modified the completed trip")
			}
		})
	}
}

func assertFinishedTrip(t *testing.T, before, value trip.Trip) {
	t.Helper()
	if value.Status != trip.StatusCompleted || value.FinishedAt == nil || value.FinishedAt.IsZero() || !value.UpdatedAt.Equal(*value.FinishedAt) {
		t.Fatalf("domain transition was not applied before saving: %+v", value)
	}
	unchanged := value
	unchanged.Status, unchanged.FinishedAt, unchanged.UpdatedAt = before.Status, before.FinishedAt, before.UpdatedAt
	if unchanged != before {
		t.Fatalf("finish changed unrelated trip fields: %+v", value)
	}
}

func assertHistory(t *testing.T, change trip.StatusChange, before, saved trip.Trip) {
	t.Helper()
	if change.TripID != before.TripID || change.FromStatus == nil || *change.FromStatus != before.Status || change.ToStatus != saved.Status || !change.ChangedAt.Equal(*saved.FinishedAt) {
		t.Fatalf("history differs from persisted transition: %+v", change)
	}
}
