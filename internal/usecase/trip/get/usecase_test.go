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
	tests := []struct {
		name    string
		repoErr error
	}{
		{name: "trip found"},
		{name: "trip not found", repoErr: trip.ErrTripNotFound},
		{name: "repository fails", repoErr: errors.New("database failed")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMocktripRepository(ctrl)
			id := uuid.New()
			want := &trip.Trip{TripID: id}
			repo.EXPECT().GetTripByID(gomock.Any(), id).Return(want, tc.repoErr)
			got, err := get.New(repo).GetTrip(context.Background(), id)
			if !errors.Is(err, tc.repoErr) {
				t.Fatalf("error: %v", err)
			}
			if tc.repoErr == nil && got != want {
				t.Fatal("wrong trip")
			}
			if tc.repoErr != nil && got != nil {
				t.Fatal("trip returned on failure")
			}
		})
	}
}
