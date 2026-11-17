package position

import (
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const positionColumns = "id, trip_id, latitude, longitude, recorded_at"

type positionsRepository struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
}

func NewPostgresqlRepository(pool *pgxpool.Pool, queryTimeout time.Duration) *positionsRepository {
	return &positionsRepository{pool: pool, queryTimeout: queryTimeout}
}
