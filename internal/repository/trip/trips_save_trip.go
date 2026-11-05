package pg

import (
	"context"
	"errors"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/samber/lo"

	trips "github.com/makkssik/template/internal/domain/trip"
	"github.com/makkssik/template/internal/infra/postgres"
)

// SaveTrip persists lifecycle fields after the trip has been locked with GetTripForUpdate.
func (t *tripsRepository) SaveTrip(ctx context.Context, trip trips.Trip) (*trips.Trip, error) {
	ctx, cancel := context.WithTimeout(ctx, t.queryTimeout)
	defer cancel()

	query, args, err := sq.Update("trips").
		Set("status", string(trip.Status)).
		Set("finished_at", trip.FinishedAt).
		Set("updated_at", trip.UpdatedAt).
		Where(sq.Eq{"id": trip.TripID}).
		Suffix("RETURNING " + tripColumns).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build save trip query: %w", err)
	}

	executor := postgres.ExecutorFromContext(ctx, t.pool)
	rows, err := executor.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("save trip: %w", err)
	}

	model, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[Trip])
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("save trip: %w", trips.ErrTripNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("scan saved trip: %w", err)
	}
	return lo.ToPtr(dbToDomain(model)), nil
}
