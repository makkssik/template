package health

import (
	"encoding/json"
	"log/slog"
	"net/http"

	api "github.com/makkssik/template/internal/generated"
)

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(api.HealthResponse{Status: api.Ok}); err != nil {
		h.logger.ErrorContext(r.Context(), "write health response", "error", err)
	}
}

type Handler struct {
	logger *slog.Logger
}

func New(logger *slog.Logger) *Handler {
	return &Handler{logger: logger}
}
