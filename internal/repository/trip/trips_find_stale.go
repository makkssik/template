package trip

import (
	"context"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"

	trips "github.com/makkssik/template/internal/domain/trip"
	"github.com/makkssik/template/internal/infra/postgres"
)

func (t *tripsRepository) FindStale(ctx context.Context, cutoff time.Time, after *trips.StaleTrip, limit int) ([]trips.StaleTrip, error) {
	ctx, cancel := context.WithTimeout(ctx, t.queryTimeout)
	defer cancel()
	builder := sq.Select("id", "driver_id", "COALESCE(last_position_at, started_at)").
		From("trips").
		Where("status = 'active'").
		Where(sq.Lt{"COALESCE(last_position_at, started_at)": cutoff}).
		OrderBy("COALESCE(last_position_at, started_at)", "id")
	if limit < 1 {
		return nil, fmt.Errorf("batch size must be positive")
	}
	builder = builder.Limit(uint64(limit))
	if after != nil {
		builder = builder.Where("(COALESCE(last_position_at, started_at), id) > (?, ?)", after.LastSeen, after.TripID)
	}
	query, args, err := builder.PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build stale query: %w", err)
	}
	rows, err := postgres.ExecutorFromContext(ctx, t.pool).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("find stale trips: %w", err)
	}
	defer rows.Close()
	result := make([]trips.StaleTrip, 0)
	for rows.Next() {
		var c trips.StaleTrip
		if err := rows.Scan(&c.TripID, &c.DriverID, &c.LastSeen); err != nil {
			return nil, fmt.Errorf("scan stale trip: %w", err)
		}
		c.LastSeen = c.LastSeen.UTC()
		result = append(result, c)
	}
	return result, rows.Err()
}
