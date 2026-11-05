package ready

import (
	"encoding/json"
	"log/slog"
	"net/http"

	api "github.com/makkssik/template/internal/generated"
)

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	statusCode := http.StatusOK
	response := api.HealthResponse{Status: api.Ok}
	if err := h.checkupService.Check(r.Context()); err != nil {
		h.logger.ErrorContext(r.Context(), "check database readiness", "error", err)
		statusCode = http.StatusServiceUnavailable
		response.Status = api.Unavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.ErrorContext(r.Context(), "write readiness response", "error", err)
	}
}

type Handler struct {
	checkupService checkupService
	logger         *slog.Logger
}

func New(checkup checkupService, logger *slog.Logger) *Handler {
	return &Handler{checkupService: checkup, logger: logger}
}
