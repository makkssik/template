package domainerror

import (
	"errors"
	"net/http"
	"strings"

	trip "github.com/makkssik/template/internal/domain/trip"
	api "github.com/makkssik/template/internal/generated"
)

func Problem(err error) (int, api.Problem) {
	status, code, detail := http.StatusInternalServerError, "internal_error", "Internal server error"
	var validation *trip.ValidationError
	switch {
	case errors.As(err, &validation):
		status, code, detail = http.StatusBadRequest, "invalid_request", validation.Error()
	case errors.Is(err, trip.ErrTripNotFound):
		status, code, detail = http.StatusNotFound, "trip_not_found", "Trip was not found"
	case errors.Is(err, trip.ErrTripCompleted):
		status, code, detail = http.StatusConflict, "trip_completed", "Trip is already completed"
	case errors.Is(err, trip.ErrDriverBusy):
		status, code, detail = http.StatusConflict, "driver_busy", "Driver already has an active trip"
	}
	return status, api.Problem{Type: "https://tripgo.example/problems/" + strings.ReplaceAll(code, "_", "-"), Title: http.StatusText(status), Status: int32(status), Detail: &detail, Code: code}
}
