package pg

import (
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const tripColumns = "id, user_id, driver_id, start_latitude, start_longitude, end_latitude, end_longitude, price, status, started_at, finished_at, created_at, updated_at"

type tripsRepository struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
}

func NewPostgresqlRepository(pool *pgxpool.Pool, queryTimeout time.Duration) *tripsRepository {
	return &tripsRepository{pool: pool, queryTimeout: queryTimeout}
}
