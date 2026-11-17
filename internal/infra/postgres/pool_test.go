package postgres_test

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/makkssik/template/internal/config"
	"github.com/makkssik/template/internal/infra/postgres"
)

func TestInvalidDatabaseConfigDoesNotExposeCredentials(t *testing.T) {
	authURL := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword("audit-user", "audit-secret"),
		Host:     "localhost",
		Path:     "/audit",
		RawQuery: "sslmode=invalid",
	}
	tests := []struct{ name, dsn string }{
		{name: "password in query", dsn: "postgres://audit-user@localhost/audit?password=audit-secret&sslmode=invalid"},
		{name: "password in URL", dsn: authURL.String()},
		{name: "password in connection parameters", dsn: "host=localhost password=audit-secret sslmode=invalid"},
		{name: "unterminated quoted password", dsn: "host=localhost password='audit-secret"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			poolConfig, err := postgres.NewPoolConfig(config.Config{DatabaseURL: tc.dsn}, nil)
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
