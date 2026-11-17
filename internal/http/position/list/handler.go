package list

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	api "github.com/makkssik/template/internal/generated"
	"github.com/makkssik/template/internal/http/domainerror"
	positionhttp "github.com/makkssik/template/internal/http/position"
)

func (h *Handler) ListTripPositions(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	if err := validateListTripPositionsRequest(tripID); err != nil {
		domainerror.WriteProblem(h.logger, w, r, http.StatusBadRequest, "invalid_request", "Invalid request", err.Error())
		return
	}

	positions, err := h.usecase.List(r.Context(), tripID)
	if err != nil {
		domainerror.ServiceError(h.logger, w, r, err)
		return
	}

	result := api.TripPositionsResponse{Positions: make([]api.TripPosition, 0, len(positions))}
	for _, point := range positions {
		result.Positions = append(result.Positions, positionhttp.ToAPI(point))
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		h.logger.ErrorContext(r.Context(), "write route response", "error", err)
	}
}

func validateListTripPositionsRequest(tripID api.TripId) error {
	if tripID == uuid.Nil {
		return errors.New("tripId must be a non-zero UUID")
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
