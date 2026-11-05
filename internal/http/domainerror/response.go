package domainerror

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	api "github.com/makkssik/template/internal/generated"
)

func ServiceError(logger *slog.Logger, w http.ResponseWriter, r *http.Request, err error) {
	status, problem := Problem(err)
	if status == http.StatusInternalServerError {
		logger.ErrorContext(r.Context(), "request failed", "error", err)
	}
	writeProblem(logger, w, r, problem)
}

func WriteProblem(logger *slog.Logger, w http.ResponseWriter, r *http.Request, status int, code, title, detail string) {
	if status < 100 || status > 599 {
		status = http.StatusInternalServerError
	}
	problem := api.Problem{
		Type:   "https://tripgo.example/problems/" + strings.ReplaceAll(code, "_", "-"),
		Title:  title,
		Status: int32(status),
		Detail: &detail,
		Code:   code,
	}
	writeProblem(logger, w, r, problem)
}

func writeProblem(logger *slog.Logger, w http.ResponseWriter, r *http.Request, problem api.Problem) {
	span := trace.SpanFromContext(r.Context())
	span.SetStatus(codes.Error, problem.Code)
	span.SetAttributes(attribute.String("error.type", problem.Code))
	instance := r.URL.Path
	problem.Instance = &instance

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(int(problem.Status))
	if err := json.NewEncoder(w).Encode(problem); err != nil {
		logger.ErrorContext(r.Context(), "write problem response", "error", err)
	}
}
