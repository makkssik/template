package position

import (
	"context"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	positions "github.com/makkssik/template/internal/domain/position"
	"github.com/makkssik/template/internal/infra/postgres"
)

func (r *positionsRepository) GetPositionsByTripID(ctx context.Context, tripID uuid.UUID) ([]positions.Position, error) {
	query, args, err := sq.Select(positionColumns).
		From("trip_positions").
		Where(sq.Eq{"trip_id": tripID}).
		OrderBy("recorded_at", "id").
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build get positions query: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	rows, err := postgres.ExecutorFromContext(ctx, r.pool).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get positions: %w", err)
	}

	models, err := pgx.CollectRows(rows, pgx.RowToStructByName[Position])
	if err != nil {
		return nil, fmt.Errorf("scan positions: %w", err)
	}
	result := make([]positions.Position, len(models))
	for i, model := range models {
		result[i] = dbToDomain(model)
	}
	return result, nil
}
