package postgres

import (
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/makkssik/template/internal/config"
)

func NewPoolConfig(cfg config.Config, tracer pgx.QueryTracer) (*pgxpool.Config, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database config: %w", &databaseConfigError{cause: err})
	}

	poolConfig.ConnConfig.Tracer = tracer
	poolConfig.MaxConns = cfg.DatabaseMaxConns
	poolConfig.MinConns = cfg.DatabaseMinConns
	poolConfig.MaxConnLifetime = cfg.DatabaseMaxConnLifetime
	poolConfig.ConnConfig.ConnectTimeout = cfg.DatabaseConnectTimeout

	return poolConfig, nil
}

// ParseConfig errors may contain credentials, including passwords in URL query
// parameters. Keep the cause for errors.Is/As without printing the connection string.
type databaseConfigError struct{ cause error }

func (*databaseConfigError) Error() string {
	return "invalid DATABASE_URL (connection string omitted)"
}

func (e *databaseConfigError) Unwrap() error { return e.cause }
