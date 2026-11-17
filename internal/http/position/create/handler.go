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
	"time"

	"github.com/google/uuid"

	"github.com/makkssik/template/internal/domain/position"
	"github.com/makkssik/template/internal/domain/trip"
	api "github.com/makkssik/template/internal/generated"
	"github.com/makkssik/template/internal/http/domainerror"
	positionhttp "github.com/makkssik/template/internal/http/position"
	"github.com/makkssik/template/internal/usecase/position/create"
)

func (h *Handler) CreateTripPosition(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	var request api.CreateTripPositionJSONRequestBody
	if err := decodeCreateTripPositionRequest(w, r, &request); err != nil {
		domainerror.WriteProblem(h.logger, w, r, http.StatusBadRequest, "invalid_request", "Invalid request", err.Error())
		return
	}
	if err := validateCreateTripPositionRequest(tripID, request); err != nil {
		domainerror.WriteProblem(h.logger, w, r, http.StatusBadRequest, "invalid_request", "Invalid request", err.Error())
		return
	}

	result, err := h.usecase.CreatePosition(r.Context(), create.Input{
		TripID: tripID,
		Coordinates: trip.Coordinates{
			Latitude:  request.Coordinates.Latitude,
			Longitude: request.Coordinates.Longitude,
		},
		RecordedAt: request.RecordedAt,
	})
	if err != nil {
		domainerror.ServiceError(h.logger, w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(positionhttp.ToAPI(*result)); err != nil {
		h.logger.ErrorContext(r.Context(), "write position response", "error", err)
	}
}

func validateCreateTripPositionRequest(tripID api.TripId, request api.CreateTripPositionJSONRequestBody) error {
	if tripID == uuid.Nil {
		return errors.New("tripId must be a non-zero UUID")
	}
	return position.Validate(trip.Coordinates{
		Latitude:  request.Coordinates.Latitude,
		Longitude: request.Coordinates.Longitude,
	}, request.RecordedAt, time.Now())
}

func decodeCreateTripPositionRequest(w http.ResponseWriter, r *http.Request, request *api.CreateTripPositionJSONRequestBody) error {
	body, err := readBody(w, r)
	if err != nil {
		return err
	}

	if err := validatePositionFields(body); err != nil {
		return err
	}
	if err := json.Unmarshal(body, request); err != nil {
		return fmt.Errorf("decode position: %w", err)
	}
	return nil
}

func readBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return nil, errors.New("Content-Type must be application/json")
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read position: %w", err)
	}
	return body, nil
}

func validatePositionFields(body []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return err
	}
	if err := validateFields(fields, "coordinates", "recorded_at"); err != nil {
		return err
	}
	var coordinates map[string]json.RawMessage
	if err := json.Unmarshal(fields["coordinates"], &coordinates); err != nil {
		return err
	}
	return validateFields(coordinates, "latitude", "longitude")
}

func validateFields(fields map[string]json.RawMessage, names ...string) error {
	for _, name := range names {
		v := fields[name]
		if len(v) == 0 || bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return fmt.Errorf("%s is required", name)
		}
	}
	if len(fields) != len(names) {
		return errors.New("unexpected JSON field")
	}
	return nil
}

type Handler struct {
	usecase usecase
	logger  *slog.Logger
}

func New(usecase usecase, logger *slog.Logger) *Handler {
	return &Handler{usecase: usecase, logger: logger}
}
