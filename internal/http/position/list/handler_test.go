package list_test

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/makkssik/template/internal/domain/position"
	"github.com/makkssik/template/internal/domain/trip"
	api "github.com/makkssik/template/internal/generated"
	"github.com/makkssik/template/internal/http/position/list"
)

func TestListHandler(t *testing.T) {
	tests := []struct {
		name       string
		values     []position.Position
		err        error
		wantStatus int
	}{
		{name: "empty", wantStatus: http.StatusOK},
		{
			name:       "route",
			values:     []position.Position{{ID: 1}, {ID: 2}},
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing",
			err:        trip.ErrTripNotFound,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "failure",
			err:        errors.New("database"),
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uc := NewMockusecase(gomock.NewController(t))
			id := uuid.New()
			uc.EXPECT().List(gomock.Any(), id).Return(tc.values, tc.err)
			r := httptest.NewRequest("GET", "/", nil)
			w := httptest.NewRecorder()
			list.New(uc, slog.New(slog.NewTextHandler(io.Discard, nil))).ListTripPositions(w, r, id)
			if w.Code != tc.wantStatus {
				t.Fatal(w.Code)
			}
			if tc.wantStatus == http.StatusOK {
				var result api.TripPositionsResponse
				if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if result.Positions == nil || len(result.Positions) != len(tc.values) {
					t.Fatalf("wrong route %s", w.Body)
				}
			}
		})
	}
}

func TestListPositionsRejectsZeroTripID(t *testing.T) {
	uc := NewMockusecase(gomock.NewController(t))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/trips/"+uuid.Nil.String()+"/positions", nil)
	response := httptest.NewRecorder()
	list.New(uc, slog.New(slog.NewTextHandler(io.Discard, nil))).ListTripPositions(response, request, uuid.Nil)
	var problem api.Problem
	if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusBadRequest || problem.Code != "invalid_request" || response.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("unexpected response: status=%d body=%s", response.Code, response.Body)
	}
}
