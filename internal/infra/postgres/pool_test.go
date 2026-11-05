package postgres_test

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/makkssik/template/internal/config"
	"github.com/makkssik/template/internal/infra/postgres"
)

func TestInvalidDatabaseConfigDoesNotExposeCredentials(t *testing.T) {
	for _, dsn := range []string{
		"postgres://audit-user@localhost/audit?password=audit-secret&sslmode=invalid",
		"postgres://audit-user:audit-secret@localhost/audit?sslmode=invalid",
		"host=localhost password=audit-secret sslmode=invalid",
		"host=localhost password='audit-secret",
	} {
		t.Run(dsn, func(t *testing.T) {
			poolConfig, err := postgres.NewPoolConfig(config.Config{DatabaseURL: dsn}, nil)
			if poolConfig != nil {
				t.Fatal("invalid config returned pool settings")
			}
			if err == nil {
				t.Fatal("invalid config accepted")
			}
			var cause *pgconn.ParseConfigError
			if !errors.As(err, &cause) {
				t.Fatal("original parse error was lost")
			}
			var output bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&output, nil))
			logger.Error("service stopped", "error", fmt.Errorf("build application: %w", err))
			if strings.Contains(output.String(), "audit-secret") || strings.Contains(output.String(), "audit-user") {
				t.Fatal("connection credentials leaked into log")
			}
			if !strings.Contains(output.String(), "DATABASE_URL") {
				t.Fatal("log does not identify the invalid setting")
			}
		})
	}
}
