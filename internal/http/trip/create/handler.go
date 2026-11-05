package create

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"slices"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"
	"github.com/google/uuid"

	domaintrips "github.com/makkssik/template/internal/domain/trip"
	api "github.com/makkssik/template/internal/generated"
	"github.com/makkssik/template/internal/http/domainerror"
	triphttp "github.com/makkssik/template/internal/http/trip"
	"github.com/makkssik/template/internal/usecase/trip/create"
)

func (h *Handler) CreateTrip(w http.ResponseWriter, r *http.Request, _ api.CreateTripParams) {
	var request api.CreateTripJSONRequestBody
	if err := validateCreateTripRequest(r, &request); err != nil {
		domainerror.WriteProblem(h.logger, w, r, http.StatusBadRequest, "invalid_request", "Invalid request", err.Error())
		return
	}

	trip, err := h.usecase.CreateTrip(r.Context(), create.Input{
		UserID:   request.UserId,
		DriverID: request.DriverId,
		StartPoint: domaintrips.Coordinates{
			Latitude:  request.StartPoint.Latitude,
			Longitude: request.StartPoint.Longitude,
		},
		EndPoint: domaintrips.Coordinates{
			Latitude:  request.EndPoint.Latitude,
			Longitude: request.EndPoint.Longitude,
		},
		Price: request.Price,
	})
	if err != nil {
		domainerror.ServiceError(h.logger, w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Location", "/api/v1/trips/"+trip.TripID.String())
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(triphttp.ToAPI(*trip)); err != nil {
		h.logger.ErrorContext(r.Context(), "write create trip response", "error", err)
	}
}

func validateCreateTripRequest(r *http.Request, request *api.CreateTripJSONRequestBody) error {
	if err := decodeCreateTripRequest(r, request); err != nil {
		return err
	}
	if request.UserId == uuid.Nil || request.DriverId == uuid.Nil {
		return errors.New("user_id and driver_id must be non-zero UUIDs")
	}

	coordinates := validation.By(func(value any) error {
		point := value.(api.Coordinates)
		return validation.ValidateStruct(&point,
			validation.Field(&point.Latitude, validation.Min(-90.0), validation.Max(90.0)),
			validation.Field(&point.Longitude, validation.Min(-180.0), validation.Max(180.0)),
		)
	})

	return validation.ValidateStruct(request,
		validation.Field(&request.UserId, validation.Required, is.UUID),
		validation.Field(&request.DriverId, validation.Required, is.UUID),
		validation.Field(&request.StartPoint, coordinates),
		validation.Field(&request.EndPoint, coordinates),
		validation.Field(&request.Price, validation.Min(int64(0))),
	)
}

func decodeCreateTripRequest(r *http.Request, request *api.CreateTripJSONRequestBody) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return errors.New("Content-Type must be application/json")
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("read request body: %w", err)
	}
	if err := json.Unmarshal(body, request); err != nil {
		return fmt.Errorf("decode request body: %w", err)
	}

	var fields map[string]any
	fieldsDecoder := json.NewDecoder(bytes.NewReader(body))
	fieldsDecoder.UseNumber()
	if err := fieldsDecoder.Decode(&fields); err != nil {
		return fmt.Errorf("decode request fields: %w", err)
	}
	requiredFields := []string{"user_id", "driver_id", "start_point", "end_point", "price"}
	for _, field := range requiredFields {
		if fields[field] == nil {
			return fmt.Errorf("%s is required", field)
		}
	}
	for field := range fields {
		if !slices.Contains(requiredFields, field) {
			return fmt.Errorf("unknown field %q", field)
		}
	}
	for _, field := range []string{"start_point", "end_point"} {
		point, ok := fields[field].(map[string]any)
		if !ok || point["latitude"] == nil || point["longitude"] == nil {
			return fmt.Errorf("%s must contain latitude and longitude", field)
		}
		for name := range point {
			if name != "latitude" && name != "longitude" {
				return fmt.Errorf("unknown field %q in %s", name, field)
			}
		}
	}
	return nil
}

type Handler struct {
	usecase tripUsecase
	logger  *slog.Logger
}

func New(usecase tripUsecase, logger *slog.Logger) *Handler {
	return &Handler{usecase: usecase, logger: logger}
}
