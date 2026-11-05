package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/makkssik/template/internal/http/domainerror"
)

type (
	recoveryContextKey struct{}
	recoveryContext    struct {
		ctx context.Context
	}
)

type HTTPMetrics struct {
	duration metric.Float64Histogram
	active   metric.Int64UpDownCounter
}

func NewMetrics(meter metric.Meter) (*HTTPMetrics, error) {
	d, err := meter.Float64Histogram("http.server.request.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1, 2.5, 5, 7.5, 10))
	if err != nil {
		return nil, err
	}
	a, err := meter.Int64UpDownCounter("http.server.active_requests")
	if err != nil {
		return nil, err
	}
	return &HTTPMetrics{duration: d, active: a}, nil
}

func Recover(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recovery := &recoveryContext{ctx: r.Context()}
		r = r.WithContext(context.WithValue(r.Context(), recoveryContextKey{}, recovery))
		defer func() {
			if value := recover(); value != nil {
				logger.ErrorContext(recovery.ctx, "request panic", "panic", fmt.Sprint(value), "stack", string(debug.Stack()))
				domainerror.WriteProblem(logger, w, r, http.StatusInternalServerError, "internal_error", "Internal Server Error", "Internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func Tracing(next http.Handler, tp trace.TracerProvider, route func(*http.Request) string) http.Handler {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Recover runs outside otelhttp, so keep the span context for its panic log.
		if recovery, ok := r.Context().Value(recoveryContextKey{}).(*recoveryContext); ok {
			recovery.ctx = r.Context()
		}
		trace.SpanFromContext(r.Context()).SetAttributes(attribute.String("http.route", route(r)))
		next.ServeHTTP(w, r)
	})
	return otelhttp.NewHandler(handler, "HTTP", otelhttp.WithTracerProvider(tp), otelhttp.WithMeterProvider(noop.NewMeterProvider()), otelhttp.WithPropagators(propagation.TraceContext{}), otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
		return r.Method + " " + route(r)
	}))
}

func Logging(next http.Handler, logger *slog.Logger, route func(*http.Request) string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		rec := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
		completed := false
		span := trace.SpanFromContext(r.Context())
		defer func() {
			status := responseStatus(rec, completed)
			level := slog.LevelInfo
			if status >= 500 {
				level = slog.LevelError
				span.SetStatus(codes.Error, "internal_error")
				span.SetAttributes(attribute.String("error.type", "internal_error"))
			}
			span.SetAttributes(attribute.Int("http.response.status_code", status))
			logger.Log(r.Context(), level, "request handled", "method", r.Method, "route", route(r), "status", status, "duration_ms", time.Since(started).Milliseconds())
		}()
		next.ServeHTTP(rec, r)
		completed = true
	})
}

func Metrics(next http.Handler, metrics *HTTPMetrics, route func(*http.Request) string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pattern := route(r)
		attrs := metric.WithAttributes(attribute.String("http.request.method", r.Method), attribute.String("http.route", pattern))
		started := time.Now()
		metrics.active.Add(r.Context(), 1, attrs)
		rec := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
		completed := false
		defer func() {
			status := responseStatus(rec, completed)
			metrics.active.Add(r.Context(), -1, attrs)
			metrics.duration.Record(r.Context(), time.Since(started).Seconds(), metric.WithAttributes(attribute.String("http.request.method", r.Method), attribute.String("http.route", pattern), attribute.Int("http.response.status_code", status)))
		}()
		next.ServeHTTP(rec, r)
		completed = true
	})
}

func responseStatus(rec chimw.WrapResponseWriter, completed bool) int {
	if !completed {
		return http.StatusInternalServerError
	}
	if rec.Status() == 0 {
		return http.StatusOK
	}
	return rec.Status()
}
