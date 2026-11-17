package get

import (
	"encoding/json"
	"log/slog"
	"net/http"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"

	api "github.com/makkssik/template/internal/generated"
	"github.com/makkssik/template/internal/http/domainerror"
	triphttp "github.com/makkssik/template/internal/http/trip"
)

func (h *Handler) GetTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	if err := validateGetTripRequest(tripID); err != nil {
		domainerror.WriteProblem(h.logger, w, r, http.StatusBadRequest, "invalid_request", "Invalid request", err.Error())
		return
	}

	trip, err := h.usecase.GetTrip(r.Context(), tripID)
	if err != nil {
		domainerror.ServiceError(h.logger, w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(triphttp.ToAPI(*trip)); err != nil {
		h.logger.ErrorContext(r.Context(), "write get trip response", "error", err)
	}
}

func validateGetTripRequest(tripID api.TripId) error {
	return validation.Validate(
		tripID,
		validation.Required,
		is.UUID,
	)
}

type Handler struct {
	usecase tripUsecase
	logger  *slog.Logger
}

func New(usecase tripUsecase, logger *slog.Logger) *Handler {
	return &Handler{usecase: usecase, logger: logger}
}
