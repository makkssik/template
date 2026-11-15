package http

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/trace"

	"github.com/makkssik/template/internal/config"
	api "github.com/makkssik/template/internal/generated"
	"github.com/makkssik/template/internal/http/domainerror"
	"github.com/makkssik/template/internal/http/middleware"
)

func NewServer(cfg config.Config, implementation *Implementation, logger *slog.Logger, tp trace.TracerProvider, metrics *middleware.HTTPMetrics) *http.Server {
	router := chi.NewRouter()
	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		domainerror.WriteProblem(logger, w, r, http.StatusNotFound, "not_found", "Not found", "Route was not found")
	})
	router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		for _, method := range []string{
			http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
			http.MethodPatch, http.MethodDelete, http.MethodOptions, http.MethodConnect, http.MethodTrace,
		} {
			if router.Match(chi.NewRouteContext(), method, r.URL.EscapedPath()) {
				w.Header().Add("Allow", method)
			}
		}
		domainerror.WriteProblem(logger, w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "Method is not allowed for this route")
	})

	route := func(r *http.Request) string {
		ctx := chi.NewRouteContext()
		if router.Match(ctx, r.Method, r.URL.EscapedPath()) {
			return ctx.RoutePattern()
		}
		return "unmatched"
	}
	handler := api.HandlerWithOptions(implementation, api.ChiServerOptions{
		BaseRouter:       router,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) { writeRequestError(logger, w, r, err) },
	})
	// Wrappers are applied inside out: Recover → Tracing → Logging → Metrics → router.
	handler = middleware.Metrics(handler, metrics, route)
	handler = middleware.Logging(handler, logger, route)
	handler = middleware.Tracing(handler, tp, route)
	handler = middleware.Recover(handler, logger)

	return &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadTimeout:       cfg.HTTPReadTimeout,
		ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
		WriteTimeout:      cfg.HTTPWriteTimeout,
		IdleTimeout:       cfg.HTTPIdleTimeout,
	}
}

func writeRequestError(logger *slog.Logger, w http.ResponseWriter, r *http.Request, _ error) {
	domainerror.WriteProblem(logger, w, r, http.StatusBadRequest, "invalid_request", "Invalid request", "Request parameters are invalid")
}
