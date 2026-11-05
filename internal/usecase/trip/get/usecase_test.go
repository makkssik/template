package get_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	trip "github.com/makkssik/template/internal/domain/trip"
	"github.com/makkssik/template/internal/usecase/trip/get"
)

func TestGetTrip(t *testing.T) {
	for _, expected := range []error{nil, trip.ErrTripNotFound, errors.New("database failed")} {
		ctrl := gomock.NewController(t)
		repo := NewMocktripRepository(ctrl)
		id := uuid.New()
		want := &trip.Trip{TripID: id}
		repo.EXPECT().GetTripByID(gomock.Any(), id).Return(want, expected)
		got, err := get.New(repo).GetTrip(context.Background(), id)
		if !errors.Is(err, expected) {
			t.Fatalf("error: %v", err)
		}
		if expected == nil && got != want {
			t.Fatal("wrong trip")
		}
		if expected != nil && got != nil {
			t.Fatal("trip returned on failure")
		}
	}
}
