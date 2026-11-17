package trip

import (
	"context"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	trips "github.com/makkssik/template/internal/domain/trip"
	"github.com/makkssik/template/internal/infra/postgres"
)

func (t *tripsRepository) StillStale(ctx context.Context, candidate trips.StaleTrip) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, t.queryTimeout)
	defer cancel()
	query, args, err := sq.Select("id").
		From("trips").
		Where(sq.Eq{"id": candidate.TripID}).
		Where("status = 'active'").
		Where(sq.Eq{"COALESCE(last_position_at, started_at)": candidate.LastSeen}).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return false, err
	}
	var id uuid.UUID
	err = postgres.ExecutorFromContext(ctx, t.pool).QueryRow(ctx, query, args...).Scan(&id)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}
