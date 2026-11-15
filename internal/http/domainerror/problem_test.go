package domainerror_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	trip "github.com/makkssik/template/internal/domain/trip"
	"github.com/makkssik/template/internal/http/domainerror"
)

func TestProblem(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{trip.ErrDriverBusy, 409, "driver_busy"}, {trip.ErrTripCompleted, 409, "trip_completed"}, {trip.ErrTripNotFound, 404, "trip_not_found"}, {&trip.ValidationError{Field: "price", Detail: "negative"}, 400, "invalid_request"}, {errors.New("secret SQL password"), 500, "internal_error"},
	} {
		status, body := domainerror.Problem(fmt.Errorf("wrapped: %w", tc.err))
		if status != tc.status || body.Code != tc.code || int(body.Status) != status {
			t.Fatalf("wrong problem: %+v", body)
		}
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "secret") {
			t.Fatal("internal details leaked")
		}
	}
}
