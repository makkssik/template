package position

import (
	"context"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/samber/lo"

	positions "github.com/makkssik/template/internal/domain/position"
	"github.com/makkssik/template/internal/infra/postgres"
)

func (r *positionsRepository) SavePosition(ctx context.Context, point positions.Position) (*positions.Position, error) {
	model := domainToDB(point)

	query, args, err := sq.Insert("trip_positions").
		Columns("trip_id", "latitude", "longitude", "recorded_at").
		Values(model.TripID, model.Latitude, model.Longitude, model.RecordedAt).
		Suffix("RETURNING " + positionColumns).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build save position query: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	rows, err := postgres.ExecutorFromContext(ctx, r.pool).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("save position: %w", err)
	}

	model, err = pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[Position])
	if err != nil {
		return nil, fmt.Errorf("scan saved position: %w", err)
	}
	return lo.ToPtr(dbToDomain(model)), nil
}
