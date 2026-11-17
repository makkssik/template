package list_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/makkssik/template/internal/domain/position"
	"github.com/makkssik/template/internal/domain/trip"
	"github.com/makkssik/template/internal/usecase/position/list"
)

func TestListPositions(t *testing.T) {
	failure := errors.New("dependency failed")
	tests := []struct {
		name     string
		loadErr  error
		routeErr error
		wantErr  error
	}{
		{name: "route returned"},
		{name: "trip not found", loadErr: trip.ErrTripNotFound, wantErr: trip.ErrTripNotFound},
		{name: "route lookup fails", routeErr: failure, wantErr: failure},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			trips := NewMocktripRepository(ctrl)
			positions := NewMockpositionRepository(ctrl)
			id := uuid.New()
			trips.EXPECT().GetTripByID(gomock.Any(), id).Return(&trip.Trip{}, tc.loadErr)
			if tc.loadErr == nil {
				positions.EXPECT().GetPositionsByTripID(gomock.Any(), id).Return([]position.Position{{ID: 1, TripID: id}}, tc.routeErr)
			}
			got, err := list.New(trips, positions).List(t.Context(), id)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err=%v", err)
			}
			if tc.wantErr == nil && (len(got) != 1 || got[0].ID != 1) {
				t.Fatal("wrong route")
			}
		})
	}
}
