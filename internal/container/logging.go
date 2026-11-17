package container

import (
	"context"
	"log/slog"
	"os"

	"github.com/makkssik/template/internal/infra/telemetry"
)

func LogError(ctx context.Context, err error) {
	service := os.Getenv("OTEL_SERVICE_NAME")
	if service == "" {
		service = "trip-service"
	}
	logger := telemetry.NewLogger(slog.LevelError, service)
	logger.ErrorContext(ctx, "service stopped with error", "error", err)
}
