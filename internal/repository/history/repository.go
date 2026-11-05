package pg

import (
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type statusHistoryRepository struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
}

func NewPostgresqlRepository(pool *pgxpool.Pool, queryTimeout time.Duration) *statusHistoryRepository {
	return &statusHistoryRepository{pool: pool, queryTimeout: queryTimeout}
}
