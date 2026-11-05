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
	const completedStage = "completed"
	for _, stage := range []string{"success", "load", "save", "history", "transaction", "commit", completedStage, "missing"} {
		t.Run(stage, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMocktripRepository(ctrl)
			history := NewMockhistoryRepository(ctrl)
			tx := NewMocktxManager(ctrl)
			observer := NewMockevents(ctrl)
			id := uuid.New()
			failure := errors.New("dependency failed")
			var wantErr error
			switch stage {
			case completedStage:
				wantErr = trip.ErrTripCompleted
			case "missing":
				wantErr = trip.ErrTripNotFound
			case "success":
			default:
				wantErr = failure
			}
			loaded := trip.Trip{
				TripID: id, UserID: uuid.New(), DriverID: uuid.New(),
				Price: 1450, Status: trip.StatusActive,
				StartPoint: trip.Coordinates{Latitude: 59, Longitude: 30},
				EndPoint:   trip.Coordinates{Latitude: 60, Longitude: 31},
				StartedAt:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			}
			if stage == completedStage {
				at := loaded.StartedAt.Add(time.Hour)
				loaded.Status, loaded.FinishedAt, loaded.UpdatedAt = trip.StatusCompleted, &at, at
			}
			before := loaded
			var saved trip.Trip
			committed := false
			type transactionKey struct{}
			txCtx := context.WithValue(t.Context(), transactionKey{}, true)
			tx.EXPECT().Do(t.Context(), gomock.Any()).DoAndReturn(func(_ context.Context, fn func(context.Context) error) error {
				if stage == "transaction" {
					return failure
				}
				if err := fn(txCtx); err != nil {
					return err
				}
				if stage == "commit" {
					return failure
				}
				committed = true
				return nil
			})
			if stage != "transaction" {
				var loadErr error
				if stage == "load" || stage == "missing" {
					loadErr = wantErr
				}
				load := repo.EXPECT().GetTripForUpdate(txCtx, id).Return(&loaded, loadErr)
				if loadErr == nil && stage != completedStage {
					save := repo.EXPECT().SaveTrip(txCtx, gomock.Any()).After(load).DoAndReturn(func(_ context.Context, value trip.Trip) (*trip.Trip, error) {
						assertFinishedTrip(t, before, value)
						if stage == "save" {
							return nil, failure
						}
						// Model database timestamp precision and ensure the persisted value is returned.
						at := value.FinishedAt.Truncate(time.Microsecond)
						saved = value
						saved.FinishedAt, saved.UpdatedAt = &at, at
						return &saved, nil
					})
					if stage != "save" {
						history.EXPECT().RegisterStatusChange(txCtx, gomock.Any()).After(save).DoAndReturn(func(_ context.Context, change trip.StatusChange) error {
							assertHistory(t, change, before, saved)
							if stage == "history" {
								return failure
							}
							return nil
						})
					}
				}
			}
			if stage == "success" {
				observer.EXPECT().Completed(t.Context(), id).Do(func(context.Context, uuid.UUID) {
					if !committed {
						t.Fatal("completion event emitted before commit")
					}
				})
			}
			got, err := finish.New(repo, history, tx, observer).FinishTrip(t.Context(), id)
			if !errors.Is(err, wantErr) {
				t.Fatalf("got %v, want %v", err, wantErr)
			}
			if wantErr == nil && (got == nil || *got != saved) {
				t.Fatalf("wrong result: got %+v, want %+v", got, saved)
			}
			if wantErr != nil && got != nil {
				t.Fatal("result returned after failure")
			}
			if stage == completedStage && loaded != before {
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
