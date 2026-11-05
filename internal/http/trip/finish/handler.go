package finish

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"

	api "github.com/makkssik/template/internal/generated"
	"github.com/makkssik/template/internal/http/domainerror"
	triphttp "github.com/makkssik/template/internal/http/trip"
)

func (h *Handler) FinishTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	if err := validateFinishTripRequest(r, tripID); err != nil {
		domainerror.WriteProblem(h.logger, w, r, http.StatusBadRequest, "invalid_request", "Invalid request", err.Error())
		return
	}

	trip, err := h.usecase.FinishTrip(r.Context(), tripID)
	if err != nil {
		domainerror.ServiceError(h.logger, w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(triphttp.ToAPI(*trip)); err != nil {
		h.logger.ErrorContext(r.Context(), "write finish trip response", "error", err)
	}
}

func validateFinishTripRequest(r *http.Request, tripID api.TripId) error {
	if err := validation.Validate(tripID, validation.Required, is.UUID); err != nil {
		return err
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1))
	if err != nil {
		return fmt.Errorf("read finish trip request: %w", err)
	}
	if len(body) != 0 {
		return errors.New("request body must be empty")
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
