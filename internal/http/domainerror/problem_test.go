package domainerror_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	trip "github.com/makkssik/template/internal/domain/trip"
	"github.com/makkssik/template/internal/http/domainerror"
)

func TestProblem(t *testing.T) {
	tests := []struct {
		err        error
		wantStatus int
		wantCode   string
	}{
		{
			err:        trip.ErrDriverBusy,
			wantStatus: http.StatusConflict,
			wantCode:   "driver_busy",
		},
		{
			err:        trip.ErrTripCompleted,
			wantStatus: http.StatusConflict,
			wantCode:   "trip_completed",
		},
		{
			err:        trip.ErrTripNotFound,
			wantStatus: http.StatusNotFound,
			wantCode:   "trip_not_found",
		},
		{
			err:        &trip.ValidationError{Field: "price", Detail: "negative"},
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_request",
		},
		{
			err:        errors.New("secret SQL password"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "internal_error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.wantCode, func(t *testing.T) {
			status, body := domainerror.Problem(fmt.Errorf("wrapped: %w", tc.err))
			if status != tc.wantStatus || body.Code != tc.wantCode || int(body.Status) != status {
				t.Fatalf("wrong problem: %+v", body)
			}
			data, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "secret") {
				t.Fatal("internal details leaked")
			}
		})
	}
}
